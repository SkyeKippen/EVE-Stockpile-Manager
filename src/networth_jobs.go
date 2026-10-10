package main

import (
	"Stockpile-Manager/src/assets"
	"Stockpile-Manager/src/auth"
	"Stockpile-Manager/src/esi"
	"Stockpile-Manager/src/networth"
	"Stockpile-Manager/src/sde"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// netWorth keeps prices and snapshots; nil when there's no database with the
// networth tables, which turns the Net Worth page and its jobs off.
var netWorth *networth.PgStore

const (
	// Jita prices are fetched once a day, after downtime (11:00 to about
	// 11:15 UTC), when the market has settled.
	priceFetchHourUTC   = 11
	priceFetchMinuteUTC = 30
	// snapshots are taken a few minutes past each hour; taken_at is the
	// hour itself, so every owner's readings line up on the graph
	snapshotMinute = 5

	priceFetchTimeout = 20 * time.Minute
	snapshotTimeout   = 30 * time.Minute
)

// snapshotMu stops a manual snapshot and the hourly one from fetching the
// same assets at once.
var snapshotMu sync.Mutex

// initNetWorth connects to DATABASE_URL, saves sign-in tokens there and
// starts the price and snapshot jobs.
func initNetWorth() {
	databaseUrl := os.Getenv("DATABASE_URL")
	if databaseUrl == "" {
		log.Printf("net worth: DATABASE_URL isn't set, so net worth isn't tracked")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseUrl)
	if err == nil {
		err = pool.Ping(ctx)
	}
	if err != nil {
		log.Printf("net worth: connecting to DATABASE_URL: %v; net worth isn't tracked", err)
		return
	}
	var ready bool
	err = pool.QueryRow(ctx, `
		SELECT to_regclass('networth.asset_values') IS NOT NULL
		   AND to_regclass('auth.character_tokens') IS NOT NULL`).Scan(&ready)
	if err != nil || !ready {
		log.Printf("net worth: tables missing (err: %v); run tools/db/generate_networth_tables.sql, then restart", err)
		pool.Close()
		return
	}

	auth.UseTokenDatabase(pool)
	netWorth = &networth.PgStore{Pool: pool}
	go runNetWorthJobs()
}

func runNetWorthJobs() {
	// catch up first: today's prices if they're missing, then a snapshot for
	// this hour, so the first snapshot after a long break has prices to use
	if err := fetchPricesIfMissing(); err != nil {
		log.Printf("net worth: fetching Jita prices: %v", err)
	}
	if last, err := netWorth.LastSnapshot(context.Background()); err != nil {
		log.Printf("net worth: reading last snapshot: %v", err)
	} else if last.Before(snapshotHour(time.Now())) {
		takeSnapshots(nil)
	}

	go every(nextDaily, func() {
		if err := fetchPrices(); err != nil {
			log.Printf("net worth: fetching Jita prices: %v", err)
		}
	})
	every(nextHourly, func() { takeSnapshots(nil) })
}

// every runs job each time next says, forever.
func every(next func(now time.Time) time.Time, job func()) {
	for {
		now := time.Now()
		time.Sleep(next(now).Sub(now))
		job()
	}
}

func nextDaily(now time.Time) time.Time {
	now = now.UTC()
	t := time.Date(now.Year(), now.Month(), now.Day(), priceFetchHourUTC, priceFetchMinuteUTC, 0, 0, time.UTC)
	if !t.After(now) {
		t = t.AddDate(0, 0, 1)
	}
	return t
}

func nextHourly(now time.Time) time.Time {
	t := now.Truncate(time.Hour).Add(snapshotMinute * time.Minute)
	if !t.After(now) {
		t = t.Add(time.Hour)
	}
	return t
}

// snapshotHour is the hour a snapshot taken at t is recorded under.
func snapshotHour(t time.Time) time.Time {
	return t.UTC().Truncate(time.Hour)
}

// priceDay is the UTC day prices fetched at t are saved under. Before the
// daily fetch time they belong to the day before, so a restart in the
// morning doesn't fetch again before downtime.
func priceDay(t time.Time) time.Time {
	t = t.UTC()
	if t.Hour() < priceFetchHourUTC || t.Hour() == priceFetchHourUTC && t.Minute() < priceFetchMinuteUTC {
		t = t.AddDate(0, 0, -1)
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func fetchPricesIfMissing() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	have, err := netWorth.HasPrices(ctx, priceDay(time.Now()))
	if err != nil || have {
		return err
	}
	return fetchPrices()
}

func fetchPrices() error {
	ctx, cancel := context.WithTimeout(context.Background(), priceFetchTimeout)
	defer cancel()
	start := time.Now()
	day := priceDay(start)
	prices, err := networth.FetchJitaPrices(ctx, fetcher.ESI)
	if err != nil {
		return err
	}
	if err := netWorth.SavePrices(ctx, day, prices); err != nil {
		return fmt.Errorf("saving prices: %w", err)
	}
	log.Printf("net worth: saved Jita prices of %d types for %s in %s", len(prices), day.Format(time.DateOnly), time.Since(start).Round(time.Second))
	return nil
}

// takeSnapshots records the asset value and wallet balance of each
// character, and of their corporations where their roles allow, for the
// current hour. With characters nil, every character with a saved sign-in
// is snapshotted.
func takeSnapshots(characters []auth.Character) {
	snapshotMu.Lock()
	defer snapshotMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), snapshotTimeout)
	defer cancel()

	if characters == nil {
		var err error
		if characters, err = auth.SavedCharacters(ctx); err != nil {
			log.Printf("net worth: listing characters: %v", err)
			return
		}
	}
	prices, err := netWorth.LatestPrices(ctx)
	if err != nil {
		log.Printf("net worth: reading prices: %v", err)
		return
	}
	if len(prices) == 0 {
		log.Printf("net worth: no Jita prices yet, so assets aren't valued this hour")
	}
	if err := sde.Load(); err != nil {
		log.Printf("net worth: blueprint prices aren't capped at their NPC price: %v", err)
	} else {
		prices = networth.CapBlueprintPrices(prices, func(typeId int64) (bool, float64) {
			t, ok := sde.Get(typeId)
			return ok && t.IsBlueprint, t.BasePrice
		})
	}

	s := snapshot{ctx: ctx, takenAt: snapshotHour(time.Now()), prices: prices, done: map[string]bool{}}
	for _, c := range characters {
		s.character(c)
	}
}

// snapshot is one hour's run over the characters.
type snapshot struct {
	ctx     context.Context
	takenAt time.Time
	prices  map[int64]float64
	// done holds what's been recorded this run, e.g. "assets:corporation:98000001",
	// so two directors of one corporation don't both fetch it
	done map[string]bool
}

func (s *snapshot) character(c auth.Character) {
	token := func(ctx context.Context) (string, error) { return auth.AccessToken(ctx, c.Id) }
	if _, err := token(s.ctx); err != nil {
		log.Printf("net worth: skipping %s (%d): %v", c.Name, c.Id, err)
		return
	}

	owner := networth.Owner{Kind: networth.KindCharacter, Id: c.Id}
	if len(s.prices) > 0 {
		s.assets(owner, c.Name,
			func() ([]assets.Item, error) { return fetcher.CachedCharacterAssets(s.ctx, c.Id, token) },
			func() ([]assets.Blueprint, error) { return fetcher.CachedCharacterBlueprints(s.ctx, c.Id, token) })
	}
	s.wallet(owner, c.Name, func() (float64, error) {
		accessToken, err := token(s.ctx)
		if err != nil {
			return 0, err
		}
		return fetcher.ESI.GetCharacterWallet(s.ctx, c.Id, accessToken)
	})

	corporationId, err := fetcher.ESI.GetCharacterCorporation(s.ctx, c.Id)
	if err != nil {
		log.Printf("net worth: looking up corporation of %s (%d): %v", c.Name, c.Id, err)
		return
	}
	if isNpcCorporation(corporationId) {
		return
	}
	corp := networth.Owner{Kind: networth.KindCorporation, Id: corporationId}
	corpName := fmt.Sprintf("corporation %d (via %s)", corporationId, c.Name)
	if len(s.prices) > 0 {
		s.assets(corp, corpName,
			func() ([]assets.Item, error) { return fetcher.CachedCorporationAssets(s.ctx, c.Id, token) },
			func() ([]assets.Blueprint, error) { return fetcher.CachedCorporationBlueprints(s.ctx, c.Id, token) })
	}
	s.wallet(corp, corpName, func() (float64, error) {
		accessToken, err := token(s.ctx)
		if err != nil {
			return 0, err
		}
		wallets, err := fetcher.ESI.GetCorporationWallets(s.ctx, corporationId, accessToken)
		var total float64
		for _, w := range wallets {
			total += w.Balance
		}
		return total, err
	})
}

func (s *snapshot) assets(owner networth.Owner, name string, items func() ([]assets.Item, error), blueprints func() ([]assets.Blueprint, error)) {
	key := fmt.Sprintf("assets:%s:%d", owner.Kind, owner.Id)
	if s.done[key] {
		return
	}
	list, err := items()
	if err != nil {
		logSnapshotError("assets", name, err)
		return
	}
	var copies map[int64]bool
	if bps, err := blueprints(); err != nil {
		log.Printf("net worth: blueprints of %s unreadable, so all its blueprints are valued at 0: %v", name, err)
	} else {
		copies = networth.CopyIds(bps)
	}

	v := networth.ValueAssets(list, copies, s.prices)
	if err := netWorth.SaveAssetValue(s.ctx, owner, s.takenAt, v); err != nil {
		log.Printf("net worth: saving asset value of %s: %v", name, err)
		return
	}
	s.done[key] = true
	log.Printf("net worth: %s assets worth %.2f ISK (%d types unpriced)", name, v.Value, v.UnpricedTypes)
}

func (s *snapshot) wallet(owner networth.Owner, name string, balance func() (float64, error)) {
	key := fmt.Sprintf("wallet:%s:%d", owner.Kind, owner.Id)
	if s.done[key] {
		return
	}
	b, err := balance()
	if err != nil {
		logSnapshotError("wallet", name, err)
		return
	}
	if err := netWorth.SaveWalletBalance(s.ctx, owner, s.takenAt, b); err != nil {
		log.Printf("net worth: saving wallet of %s: %v", name, err)
		return
	}
	s.done[key] = true
	log.Printf("net worth: %s wallet holds %.2f ISK", name, b)
}

func logSnapshotError(what, name string, err error) {
	if errors.Is(err, esi.ErrForbidden) {
		log.Printf("net worth: no access to the %s of %s (missing scope or role), skipped", what, name)
		return
	}
	log.Printf("net worth: fetching %s of %s: %v", what, name, err)
}

// isNpcCorporation is true for the NPC corporations characters start in,
// which have no assets or wallets to read.
func isNpcCorporation(id int64) bool {
	return id >= 1_000_000 && id < 2_000_000
}
