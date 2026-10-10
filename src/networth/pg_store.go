package networth

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	KindCharacter   = "character"
	KindCorporation = "corporation"
)

// Owner is whose assets or wallet a snapshot is of.
type Owner struct {
	Kind string // KindCharacter or KindCorporation
	Id   int64
}

// PgStore keeps prices and snapshots in the networth schema; create its
// tables with tools/db/generate_networth_tables.sql.
type PgStore struct {
	Pool *pgxpool.Pool
}

// HasPrices reports whether prices were saved for day (a UTC date).
func (p *PgStore) HasPrices(ctx context.Context, day time.Time) (bool, error) {
	var exists bool
	err := p.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM networth.jita_prices WHERE price_date = $1)`, day).Scan(&exists)
	return exists, err
}

// SavePrices saves day's prices, replacing any already saved for that day.
func (p *PgStore) SavePrices(ctx context.Context, day time.Time, prices []Price) error {
	return pgx.BeginFunc(ctx, p.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM networth.jita_prices WHERE price_date = $1`, day); err != nil {
			return err
		}
		_, err := tx.CopyFrom(ctx, pgx.Identifier{"networth", "jita_prices"},
			[]string{"price_date", "type_id", "sell_price", "min_price", "volume", "order_count"},
			pgx.CopyFromSlice(len(prices), func(i int) ([]any, error) {
				pr := prices[i]
				return []any{day, pr.TypeId, pr.SellPrice, pr.MinPrice, pr.Volume, pr.OrderCount}, nil
			}))
		return err
	})
}

// LatestPriceDate returns the newest day prices were saved for, or the zero
// time when there are none.
func (p *PgStore) LatestPriceDate(ctx context.Context) (time.Time, error) {
	var day *time.Time
	err := p.Pool.QueryRow(ctx, `SELECT max(price_date) FROM networth.jita_prices`).Scan(&day)
	if err != nil || day == nil {
		return time.Time{}, err
	}
	return *day, nil
}

// LatestPrices returns each type's most recently saved sell price.
func (p *PgStore) LatestPrices(ctx context.Context) (map[int64]float64, error) {
	rows, err := p.Pool.Query(ctx, `
		SELECT DISTINCT ON (type_id) type_id, sell_price::float8
		FROM networth.jita_prices
		ORDER BY type_id, price_date DESC`)
	if err != nil {
		return nil, err
	}
	prices := map[int64]float64{}
	var typeId int64
	var price float64
	_, err = pgx.ForEachRow(rows, []any{&typeId, &price}, func() error {
		prices[typeId] = price
		return nil
	})
	return prices, err
}

// SaveAssetValue records what owner's assets were worth at takenAt,
// replacing a value already recorded for that time.
func (p *PgStore) SaveAssetValue(ctx context.Context, owner Owner, takenAt time.Time, v Valuation) error {
	_, err := p.Pool.Exec(ctx, `
		INSERT INTO networth.asset_values (owner_kind, owner_id, taken_at, value, unpriced_types)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (owner_kind, owner_id, taken_at) DO UPDATE
		SET value = excluded.value, unpriced_types = excluded.unpriced_types`,
		owner.Kind, owner.Id, takenAt, v.Value, v.UnpricedTypes)
	return err
}

// SaveWalletBalance records owner's total wallet balance at takenAt,
// replacing a balance already recorded for that time.
func (p *PgStore) SaveWalletBalance(ctx context.Context, owner Owner, takenAt time.Time, balance float64) error {
	_, err := p.Pool.Exec(ctx, `
		INSERT INTO networth.wallet_balances (owner_kind, owner_id, taken_at, balance)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (owner_kind, owner_id, taken_at) DO UPDATE SET balance = excluded.balance`,
		owner.Kind, owner.Id, takenAt, balance)
	return err
}

// LastSnapshot returns when the newest asset value or wallet balance was
// taken, or the zero time when there are none.
func (p *PgStore) LastSnapshot(ctx context.Context) (time.Time, error) {
	var last *time.Time
	err := p.Pool.QueryRow(ctx, `
		SELECT max(taken_at) FROM (
			SELECT max(taken_at) AS taken_at FROM networth.asset_values
			UNION ALL
			SELECT max(taken_at) FROM networth.wallet_balances
		) latest`).Scan(&last)
	if err != nil || last == nil {
		return time.Time{}, err
	}
	return *last, nil
}

// Reading is one owner's asset value or wallet balance at one time; the
// other is nil when it wasn't recorded.
type Reading struct {
	Owner      Owner
	TakenAt    time.Time
	AssetValue *float64
	Wallet     *float64
}

// History returns every reading for owners, oldest first.
func (p *PgStore) History(ctx context.Context, owners []Owner) ([]Reading, error) {
	kinds := make([]string, len(owners))
	ids := make([]int64, len(owners))
	for i, o := range owners {
		kinds[i], ids[i] = o.Kind, o.Id
	}
	rows, err := p.Pool.Query(ctx, `
		WITH owners AS (SELECT * FROM unnest($1::text[], $2::bigint[]) AS o(owner_kind, owner_id))
		SELECT owner_kind, owner_id, taken_at, a.value::float8, w.balance::float8
		FROM (SELECT * FROM networth.asset_values a JOIN owners USING (owner_kind, owner_id)) a
		FULL JOIN (SELECT * FROM networth.wallet_balances w JOIN owners USING (owner_kind, owner_id)) w
			USING (owner_kind, owner_id, taken_at)
		ORDER BY taken_at`, kinds, ids)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Reading, error) {
		var r Reading
		err := row.Scan(&r.Owner.Kind, &r.Owner.Id, &r.TakenAt, &r.AssetValue, &r.Wallet)
		return r, err
	})
}
