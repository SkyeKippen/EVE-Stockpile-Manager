package networth

import (
	"slices"
	"time"
)

// Point is one time on the net worth graph, totalled across owners.
type Point struct {
	Time       time.Time `json:"time"`
	Wallet     float64   `json:"wallet"`
	AssetValue float64   `json:"assetValue"`
	NetWorth   float64   `json:"netWorth"`
}

// Series totals readings (oldest first) into one point per time. When an
// owner's wallet or assets couldn't be read at some time, their last known
// value is used, so a failed fetch doesn't show as a drop in net worth.
func Series(readings []Reading) []Point {
	lastWallet := map[Owner]float64{}
	lastAssets := map[Owner]float64{}
	var points []Point
	for i := 0; i < len(readings); {
		takenAt := readings[i].TakenAt
		for ; i < len(readings) && readings[i].TakenAt.Equal(takenAt); i++ {
			if r := readings[i]; r.Wallet != nil {
				lastWallet[r.Owner] = *r.Wallet
			}
			if r := readings[i]; r.AssetValue != nil {
				lastAssets[r.Owner] = *r.AssetValue
			}
		}
		p := Point{Time: takenAt}
		for _, v := range lastWallet {
			p.Wallet += v
		}
		for _, v := range lastAssets {
			p.AssetValue += v
		}
		p.NetWorth = p.Wallet + p.AssetValue
		points = append(points, p)
	}
	slices.SortFunc(points, func(a, b Point) int { return a.Time.Compare(b.Time) })
	return points
}
