package assets

import (
	"Stockpile-Manager/src/esi"
	"Stockpile-Manager/src/location"
	"context"
	"testing"
)

func TestBlueprintsTakeTheirAssetLocation(t *testing.T) {
	inContainer := location.Location{LocationId: 1035466617946, InContainer: true, ContainerName: "BPCs"}
	items := []Item{{ItemId: 501, TypeId: 4314, Location: inContainer}}
	raw := []esi.Blueprint{
		{ItemId: 501, TypeId: 4314, LocationId: 9001, LocationFlag: "Unlocked", Quantity: -2, Runs: 15},
	}

	got, err := (&Fetcher{}).blueprints(context.Background(), location.Owner{}, raw, items)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !got[0].IsCopy || got[0].Runs != 15 || got[0].Location.LocationId != inContainer.LocationId {
		t.Fatalf("got %+v", got)
	}
}

func TestOriginalsAreNotCopies(t *testing.T) {
	for quantity, isCopy := range map[int64]bool{-1: false, -2: true, 3: false} {
		if got := (esi.Blueprint{Quantity: quantity}).IsCopy(); got != isCopy {
			t.Errorf("quantity %d: IsCopy = %v, want %v", quantity, got, isCopy)
		}
	}
}
