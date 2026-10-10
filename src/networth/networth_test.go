package networth

import (
	"Stockpile-Manager/src/assets"
	"Stockpile-Manager/src/esi"
	"testing"
	"time"
)

func TestSellPricesAveragesCheapestFivePercent(t *testing.T) {
	orders := []esi.RegionOrder{
		{OrderId: 1, TypeId: 34, Price: 6, VolumeRemain: 900},
		{OrderId: 2, TypeId: 34, Price: 4, VolumeRemain: 30},
		{OrderId: 3, TypeId: 34, Price: 5, VolumeRemain: 70},
		{OrderId: 4, TypeId: 34, Price: 1, VolumeRemain: 500, IsBuyOrder: true}, // ignored
		{OrderId: 5, TypeId: 35, Price: 100, VolumeRemain: 1},
	}
	prices := SellPrices(orders)
	if len(prices) != 2 {
		t.Fatalf("got %d prices, want 2", len(prices))
	}
	// 1000 units for sale, so the cheapest 50: 30 at 4 and 20 at 5
	if got, want := prices[0].SellPrice, (30*4.0+20*5.0)/50; got != want {
		t.Errorf("type 34 price = %v, want %v", got, want)
	}
	if prices[0].MinPrice != 4 || prices[0].Volume != 1000 || prices[0].OrderCount != 3 {
		t.Errorf("type 34 = %+v", prices[0])
	}
	if prices[1].SellPrice != 100 {
		t.Errorf("type 35 price = %v, want 100", prices[1].SellPrice)
	}
}

func TestCapAtAverage(t *testing.T) {
	prices := CapAtAverage([]Price{
		{TypeId: 21594, SellPrice: 64_000_000}, // junk listings only
		{TypeId: 34, SellPrice: 3.89},
		{TypeId: 99, SellPrice: 50}, // no average
	}, map[int64]float64{21594: 9.36, 34: 3.73})
	if prices[0].SellPrice != 28.08 || prices[1].SellPrice != 3.89 || prices[2].SellPrice != 50 {
		t.Errorf("prices = %+v", prices)
	}
}

func TestValueAssets(t *testing.T) {
	items := []assets.Item{
		{ItemId: 1, TypeId: 34, Name: "Tritanium", Quantity: 1000},
		{ItemId: 2, TypeId: 688, Name: "Raven Blueprint", Quantity: 1}, // copy
		{ItemId: 3, TypeId: 688, Name: "Raven Blueprint", Quantity: 1}, // original
		{ItemId: 4, TypeId: 99999, Name: "Unsold Thing", Quantity: 5},
	}
	prices := map[int64]float64{34: 4.5, 688: 1_000_000}

	v := ValueAssets(items, map[int64]bool{2: true}, prices)
	if v.Value != 4500+1_000_000 || v.UnpricedTypes != 1 {
		t.Errorf("with copies known: %+v", v)
	}
	v = ValueAssets(items, nil, prices)
	if v.Value != 4500 {
		t.Errorf("with copies unknown, blueprints should count as 0: %+v", v)
	}
}

func TestCapBlueprintPrices(t *testing.T) {
	prices := map[int64]float64{34: 4.5, 688: 2_000_000_000, 691: 1_000_000}
	capped := CapBlueprintPrices(prices, func(typeId int64) (bool, float64) {
		switch typeId {
		case 688:
			return true, 1_135_000_000
		case 691:
			return true, 2_800_000
		}
		return false, 0
	})
	if capped[34] != 4.5 || capped[688] != 1_135_000_000 || capped[691] != 1_000_000 {
		t.Errorf("capped = %v", capped)
	}
	if prices[688] != 2_000_000_000 {
		t.Errorf("the original map was changed")
	}
}

func TestSeriesCarriesForwardMissingReadings(t *testing.T) {
	h0 := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
	h1 := h0.Add(time.Hour)
	f := func(v float64) *float64 { return &v }
	char := Owner{Kind: KindCharacter, Id: 1}
	corp := Owner{Kind: KindCorporation, Id: 2}

	points := Series([]Reading{
		{Owner: char, TakenAt: h0, AssetValue: f(100), Wallet: f(10)},
		{Owner: corp, TakenAt: h0, AssetValue: f(1000), Wallet: f(50)},
		{Owner: char, TakenAt: h1, AssetValue: f(120), Wallet: f(20)},
		{Owner: corp, TakenAt: h1, Wallet: f(60)}, // corp assets unreadable this hour
	})
	want := []Point{
		{Time: h0, Wallet: 60, AssetValue: 1100, NetWorth: 1160},
		{Time: h1, Wallet: 80, AssetValue: 1120, NetWorth: 1200},
	}
	if len(points) != len(want) {
		t.Fatalf("got %d points, want %d", len(points), len(want))
	}
	for i := range want {
		if points[i] != want[i] {
			t.Errorf("point %d = %+v, want %+v", i, points[i], want[i])
		}
	}
}
