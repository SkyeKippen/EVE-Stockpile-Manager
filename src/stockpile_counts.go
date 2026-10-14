package main

import (
	"Stockpile-Manager/src/assets"
	"Stockpile-Manager/src/auth"
	"Stockpile-Manager/src/stockpile"
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
)

// isFittedFlag is true for modules fitted to a ship, which aren't spare
// stock even when the ship is parked at the structure.
func isFittedFlag(flag string) bool {
	return strings.Contains(flag, "Slot")
}

// handleStockpileCounts returns, as JSON, how many of each stockpile item the
// signed-in character and their corporation have at the stockpile's
// structure, keyed "<stockpileId>-<typeId>". Items in containers and ship
// holds count; modules fitted to ships don't. Blueprint runs are counted the
// same way from the blueprints routes: the runs left on every copy, or BPO
// when there's an original there.
//
// The response also says which owners' assets and blueprints could be read,
// so the page can explain a missing count.
func handleStockpileCounts(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.CurrentSession(r)
	// the asset fetches have their own time limit, and carry on for the next
	// page load if this one gives up waiting
	ctx := r.Context()

	list, err := stockpiles.ListStockpiles(ctx)
	if err != nil {
		log.Printf("handleStockpileCounts: %v", err)
		http.Error(w, "could not load stockpiles", http.StatusInternalServerError)
		return
	}

	token := func(ctx context.Context) (string, error) {
		return auth.AccessToken(ctx, sess.CharacterID)
	}
	// fetched side by side; either can take a while the first time
	var characterItems, corpItems []assets.Item
	var characterBlueprints, corpBlueprints []assets.Blueprint
	var characterErr, corpErr, characterBpErr, corpBpErr error
	var fetches sync.WaitGroup
	fetches.Go(func() {
		characterItems, characterErr = fetcher.CachedCharacterAssets(ctx, sess.CharacterID, token)
	})
	fetches.Go(func() {
		corpItems, corpErr = fetcher.CachedCorporationAssets(ctx, sess.CharacterID, token)
	})
	if wantsRuns(list) {
		fetches.Go(func() {
			characterBlueprints, characterBpErr = fetcher.CachedCharacterBlueprints(ctx, sess.CharacterID, token)
		})
		fetches.Go(func() {
			corpBlueprints, corpBpErr = fetcher.CachedCorporationBlueprints(ctx, sess.CharacterID, token)
		})
	}
	fetches.Wait()
	if characterBpErr != nil {
		log.Printf("handleStockpileCounts: character blueprints: %v", characterBpErr)
	}
	if corpBpErr != nil {
		log.Printf("handleStockpileCounts: corporation blueprints: %v", corpBpErr)
	}
	if characterErr != nil {
		log.Printf("handleStockpileCounts: character assets: %v", characterErr)
	}
	if corpErr != nil {
		log.Printf("handleStockpileCounts: corporation assets: %v", corpErr)
	}

	if ctx.Err() != nil {
		return // the page was closed or reloaded
	}

	counts := countAtStructures(list, characterItems, corpItems)

	writeJSON(w, struct {
		Counts              map[string]int64       `json:"counts"`
		Runs                map[string]runsAtPlace `json:"runs"`
		CharacterAssets     bool                   `json:"characterAssets"`
		CorpAssets          bool                   `json:"corpAssets"`
		CharacterBlueprints bool                   `json:"characterBlueprints"`
		CorpBlueprints      bool                   `json:"corpBlueprints"`
		Prices              map[int64]float64      `json:"prices"` // Jita sell price of items, by type ID
	}{
		counts, runsAtStructures(list, characterBlueprints, corpBlueprints),
		characterErr == nil, corpErr == nil, characterBpErr == nil, corpBpErr == nil,
		stockpilePrices(ctx, list),
	})
}

// stockpilePrices returns the latest Jita sell price of each item the
// stockpiles want (not blueprint runs), priced the same way as net worth.
// Items with no Jita price are left out; with no database, all of them are.
func stockpilePrices(ctx context.Context, list []stockpile.Stockpile) map[int64]float64 {
	if netWorth == nil {
		return map[int64]float64{}
	}
	var typeIds []int64
	for _, s := range list {
		for _, item := range s.Items {
			if item.Kind == stockpile.KindItem {
				typeIds = append(typeIds, item.TypeId)
			}
		}
	}
	prices, err := netWorth.LatestPricesFor(ctx, typeIds)
	if err != nil {
		log.Printf("handleStockpileCounts: prices: %v", err)
		return map[int64]float64{}
	}
	return capBlueprintPrices(prices)
}

// countAtStructures counts, for each item a stockpile wants, how many of it
// the owners' assets hold at the stockpile's structure, keyed
// "<stockpileId>-<typeId>". Blueprint runs are left out.
func countAtStructures(list []stockpile.Stockpile, owners ...[]assets.Item) map[string]int64 {
	// structure ID -> type ID -> count
	atStructure := map[int64]map[int64]int64{}
	for _, items := range owners {
		for _, item := range items {
			if item.Location.LocationId == 0 || isFittedFlag(item.LocationFlag) {
				continue
			}
			byType := atStructure[item.Location.LocationId]
			if byType == nil {
				byType = map[int64]int64{}
				atStructure[item.Location.LocationId] = byType
			}
			byType[item.TypeId] += item.Quantity
		}
	}

	counts := map[string]int64{}
	for _, s := range list {
		for _, item := range s.Items {
			if item.Kind == stockpile.KindItem {
				counts[fmt.Sprintf("%d-%d", s.StockpileId, item.TypeId)] = atStructure[s.Structure.StructureId][item.TypeId]
			}
		}
	}
	return counts
}

func wantsRuns(list []stockpile.Stockpile) bool {
	for _, s := range list {
		for _, item := range s.Items {
			if item.Kind == stockpile.KindBlueprintRuns {
				return true
			}
		}
	}
	return false
}

// runsAtPlace is what a structure holds of one blueprint: the runs left on
// its copies, and whether there's an original.
type runsAtPlace struct {
	Runs int64 `json:"runs"`
	BPO  bool  `json:"bpo"`
}

// runsAtStructures totals, for each blueprint a stockpile wants runs of, the
// owners' blueprints of that type at the stockpile's structure, keyed
// "<stockpileId>-<typeId>".
func runsAtStructures(list []stockpile.Stockpile, owners ...[]assets.Blueprint) map[string]runsAtPlace {
	type placeType struct{ place, typeId int64 }
	atStructure := map[placeType]runsAtPlace{}
	for _, blueprints := range owners {
		for _, b := range blueprints {
			if b.Location.LocationId == 0 {
				continue
			}
			key := placeType{b.Location.LocationId, b.TypeId}
			total := atStructure[key]
			if b.IsCopy {
				total.Runs += b.Runs
			} else {
				total.BPO = true
			}
			atStructure[key] = total
		}
	}

	runs := map[string]runsAtPlace{}
	for _, s := range list {
		for _, item := range s.Items {
			if item.Kind == stockpile.KindBlueprintRuns {
				runs[fmt.Sprintf("%d-%d", s.StockpileId, item.TypeId)] = atStructure[placeType{s.Structure.StructureId, item.TypeId}]
			}
		}
	}
	return runs
}
