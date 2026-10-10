package networth

import (
	"Stockpile-Manager/src/assets"
	"strings"
)

// Valuation is what a list of assets is worth at Jita prices.
type Valuation struct {
	Value float64
	// UnpricedTypes counts the item types with no Jita sell orders, which
	// are valued at 0.
	UnpricedTypes int
}

// ValueAssets adds up items at prices (ISK per unit, by type). Blueprint
// copies can't be sold on the market, so they're valued at 0 rather than at
// their original's price. copies holds the item IDs of the copies; when it's
// nil the blueprint list couldn't be read, and every blueprint is valued at
// 0 rather than risk pricing copies as originals.
func ValueAssets(items []assets.Item, copies map[int64]bool, prices map[int64]float64) Valuation {
	var v Valuation
	unpriced := map[int64]bool{}
	for _, item := range items {
		if copies == nil && isBlueprintName(item.Name) || copies[item.ItemId] {
			continue
		}
		price, ok := prices[item.TypeId]
		if !ok {
			unpriced[item.TypeId] = true
			continue
		}
		v.Value += price * float64(item.Quantity)
	}
	v.UnpricedTypes = len(unpriced)
	return v
}

// isBlueprintName matches the rule sde.Type.IsBlueprint uses: every
// published blueprint or reaction formula's name ends in one of these.
func isBlueprintName(name string) bool {
	return strings.HasSuffix(name, " Blueprint") || strings.HasSuffix(name, " Formula")
}

// CopyIds returns the item IDs of the blueprint copies in blueprints.
func CopyIds(blueprints []assets.Blueprint) map[int64]bool {
	copies := map[int64]bool{}
	for _, b := range blueprints {
		if b.IsCopy {
			copies[b.ItemId] = true
		}
	}
	return copies
}

// CapBlueprintPrices returns prices with each blueprint's price capped at
// its NPC base price, since a blueprint original's cheapest Jita listings
// are often far above what NPCs sell it for. blueprint reports whether a
// type is a blueprint and its base price (0 when unknown, which leaves the
// price as it is).
func CapBlueprintPrices(prices map[int64]float64, blueprint func(typeId int64) (isBlueprint bool, basePrice float64)) map[int64]float64 {
	capped := make(map[int64]float64, len(prices))
	for typeId, price := range prices {
		if isBlueprint, base := blueprint(typeId); isBlueprint && base > 0 && price > base {
			price = base
		}
		capped[typeId] = price
	}
	return capped
}
