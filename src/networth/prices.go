// Package networth values a character's and their corporation's assets at
// Jita prices and keeps hourly snapshots of that value and their wallets, so
// net worth can be graphed over time.
package networth

import (
	"Stockpile-Manager/src/esi"
	"cmp"
	"context"
	"fmt"
	"math"
	"slices"
)

const (
	TheForgeRegionId = 10000002
	Jita44StationId  = 60003760 // Jita IV - Moon 4 - Caldari Navy Assembly Plant
	// PLEX trades on its own global market rather than in Jita.
	PlexRegionId = 19000001

	// cheapestShare is how much of an item's sell volume its price is taken
	// from: the average price of the cheapest 5% of units listed.
	cheapestShare = 0.05
	// averageCap caps a type's sell price at this many times CCP's average
	// traded price, since a type with few sell orders can have only junk
	// listings far above what it trades at.
	averageCap = 3
)

// Price is one item type's Jita sell price for a day.
type Price struct {
	TypeId     int64
	SellPrice  float64 // volume-weighted average of the cheapest 5% of units for sale, capped by averageCap
	MinPrice   float64
	Volume     int64 // units for sale
	OrderCount int
}

// FetchJitaPrices prices every item with sell orders in Jita 4-4, plus PLEX
// from its global market.
func FetchJitaPrices(ctx context.Context, client *esi.Client) ([]Price, error) {
	forge, err := client.GetRegionSellOrders(ctx, TheForgeRegionId)
	if err != nil {
		return nil, fmt.Errorf("fetching The Forge sell orders: %w", err)
	}
	jita := slices.DeleteFunc(forge, func(o esi.RegionOrder) bool { return o.LocationId != Jita44StationId })

	plex, err := client.GetRegionSellOrders(ctx, PlexRegionId)
	if err != nil {
		return nil, fmt.Errorf("fetching PLEX sell orders: %w", err)
	}
	averages, err := client.GetAveragePrices(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching average prices: %w", err)
	}
	return CapAtAverage(SellPrices(append(jita, plex...)), averages), nil
}

// CapAtAverage caps each price at averageCap times the type's average traded
// price. Types with no average are left as they are.
func CapAtAverage(prices []Price, averages map[int64]float64) []Price {
	for i, p := range prices {
		if avg, ok := averages[p.TypeId]; ok && p.SellPrice > avg*averageCap {
			prices[i].SellPrice = math.Round(avg*averageCap*100) / 100
		}
	}
	return prices
}

// SellPrices combines each type's sell orders into one price: the
// volume-weighted average of the cheapest 5% of units for sale. The cheapest
// order always counts, so a type with a single big order gets its price.
func SellPrices(orders []esi.RegionOrder) []Price {
	byType := map[int64][]esi.RegionOrder{}
	for _, o := range orders {
		if !o.IsBuyOrder && o.VolumeRemain > 0 {
			byType[o.TypeId] = append(byType[o.TypeId], o)
		}
	}

	prices := make([]Price, 0, len(byType))
	for typeId, typeOrders := range byType {
		slices.SortFunc(typeOrders, func(a, b esi.RegionOrder) int { return cmp.Compare(a.Price, b.Price) })

		var volume int64
		for _, o := range typeOrders {
			volume += o.VolumeRemain
		}
		target := max(1, int64(math.Ceil(float64(volume)*cheapestShare)))

		var counted int64
		var total float64
		for _, o := range typeOrders {
			take := min(o.VolumeRemain, target-counted)
			total += float64(take) * o.Price
			counted += take
			if counted >= target {
				break
			}
		}

		prices = append(prices, Price{
			TypeId:     typeId,
			SellPrice:  math.Round(total/float64(counted)*100) / 100,
			MinPrice:   typeOrders[0].Price,
			Volume:     volume,
			OrderCount: len(typeOrders),
		})
	}
	slices.SortFunc(prices, func(a, b Price) int { return cmp.Compare(a.TypeId, b.TypeId) })
	return prices
}
