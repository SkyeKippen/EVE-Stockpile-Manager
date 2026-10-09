// Package assets fetches a character's (or their corporation's) assets from
// ESI, with each item's type name and where it is, and the structures the
// character can see.
package assets

import (
	"Stockpile-Manager/src/esi"
	"Stockpile-Manager/src/location"
	"cmp"
	"context"
	"fmt"
	"log"
	"net/http"
	"slices"
	"time"
)

// Item is one stack of items, or one ship or container.
type Item struct {
	ItemId       int64
	TypeId       int64
	Name         string // the type's name, e.g. "Tritanium"
	Quantity     int64
	LocationFlag string // e.g. Hangar, Cargo, CorpSAG3
	Location     location.Location
}

// Fetcher holds what the lookups share, so stations, structures and type
// names are only asked about once while the server runs.
type Fetcher struct {
	ESI       *esi.Client
	Resolver  *location.Resolver
	TypeNames *esi.TypeNames

	cache assetCache
}

func NewFetcher() *Fetcher {
	// a stalled ESI request shouldn't hold up everything queued behind it
	client := &esi.Client{HTTPClient: &http.Client{Timeout: 30 * time.Second}}
	typeNames := &esi.TypeNames{Client: client}
	return &Fetcher{
		ESI:       client,
		TypeNames: typeNames,
		Resolver: &location.Resolver{
			ESI:      client,
			Store:    &location.MemoryStore{},
			TypeName: typeNames.Name,
		},
	}
}

// CharacterAssets needs the esi-assets.read_assets.v1 scope, and
// esi-universe.read_structures.v1 to name the structures items are in.
func (f *Fetcher) CharacterAssets(ctx context.Context, characterId int64, accessToken string) ([]Item, error) {
	raw, err := f.ESI.GetCharacterAssets(ctx, characterId, accessToken)
	if err != nil {
		return nil, fmt.Errorf("fetching assets of character %d: %w", characterId, err)
	}
	owner := location.Owner{CharacterId: characterId, AccessToken: accessToken, ReadCharacterAssets: true, Assets: raw}
	return f.items(ctx, owner, raw)
}

// CorporationAssets returns the assets of the character's corporation. It
// needs the esi-assets.read_corporation_assets.v1 scope and the Director role
// in game.
func (f *Fetcher) CorporationAssets(ctx context.Context, characterId int64, accessToken string) ([]Item, error) {
	corporationId, err := f.ESI.GetCharacterCorporation(ctx, characterId)
	if err != nil {
		return nil, fmt.Errorf("looking up corporation of character %d: %w", characterId, err)
	}
	raw, err := f.ESI.GetCorporationAssets(ctx, corporationId, accessToken)
	if err != nil {
		return nil, fmt.Errorf("fetching assets of corporation %d: %w", corporationId, err)
	}
	owner := location.Owner{CharacterId: characterId, CorporationId: corporationId, AccessToken: accessToken, Assets: raw}
	return f.items(ctx, owner, raw)
}

func (f *Fetcher) items(ctx context.Context, owner location.Owner, raw []esi.Asset) ([]Item, error) {
	typeIds := make([]int64, 0, len(raw))
	refs := make([]location.Ref, 0, len(raw))
	for _, asset := range raw {
		typeIds = append(typeIds, asset.TypeId)
		refs = append(refs, location.Ref{LocationId: asset.LocationId, LocationFlag: asset.LocationFlag})
	}

	names, err := f.TypeNames.Names(ctx, typeIds)
	if err != nil {
		return nil, fmt.Errorf("naming asset types: %w", err)
	}
	locations, err := f.Resolver.Resolve(ctx, owner, refs)
	if err != nil {
		return nil, fmt.Errorf("resolving asset locations: %w", err)
	}

	items := make([]Item, 0, len(raw))
	for i, asset := range raw {
		items = append(items, Item{
			ItemId:       asset.ItemId,
			TypeId:       asset.TypeId,
			Name:         names[asset.TypeId],
			Quantity:     asset.Quantity,
			LocationFlag: asset.LocationFlag,
			Location:     locations[refs[i]],
		})
	}
	return items, nil
}

// CharacterPlaces returns the NPC stations and player structures a
// stockpile can be tied to for this character, sorted by name: wherever the
// character or their corporation has assets, plus where the character's
// industry jobs run and market orders are listed. Player structures are only
// included when the character can read them (esi-universe.read_structures.v1),
// since otherwise their name is unknown.
//
// ESI has no list of every structure a character may dock at, and asking
// about structures the character can't see counts toward ESI's error limit,
// so only places the character has a reason to know about are checked. Each
// source needs its own scope (corporation assets also need the Director
// role); one that fails is logged and skipped.
func (f *Fetcher) CharacterPlaces(ctx context.Context, characterId int64, accessToken string) ([]location.Location, error) {
	var candidates []int64

	token := func(context.Context) (string, error) { return accessToken, nil }
	assetPlaces := func(items []Item) {
		for _, item := range items {
			if item.Location.Kind == location.KindStation || item.Location.Kind == location.KindStructure {
				candidates = append(candidates, item.Location.LocationId)
			}
		}
	}
	if items, err := f.CachedCharacterAssets(ctx, characterId, token); err != nil {
		log.Printf("places: character assets: %v", err)
	} else {
		assetPlaces(items)
	}
	if items, err := f.CachedCorporationAssets(ctx, characterId, token); err != nil {
		log.Printf("places: corporation assets: %v", err)
	} else {
		assetPlaces(items)
	}

	if jobs, err := f.ESI.GetCharacterIndustryJobs(ctx, characterId, accessToken); err != nil {
		log.Printf("places: fetching industry jobs of character %d: %v", characterId, err)
	} else {
		for _, job := range jobs {
			// not output_location_id, which may be a container
			candidates = append(candidates, job.FacilityId, job.StationId)
		}
	}

	if orders, err := f.ESI.GetCharacterOrders(ctx, characterId, accessToken); err != nil {
		log.Printf("places: fetching market orders of character %d: %v", characterId, err)
	} else {
		for _, order := range orders {
			candidates = append(candidates, order.LocationId)
		}
	}

	// a Hangar ref goes straight to the place, without walking containers
	refs := make([]location.Ref, 0, len(candidates))
	seen := map[int64]bool{}
	for _, id := range candidates {
		if !(isStationId(id) || isStructureId(id)) || seen[id] {
			continue
		}
		seen[id] = true
		refs = append(refs, location.Ref{LocationId: id, LocationFlag: "Hangar"})
	}

	owner := location.Owner{CharacterId: characterId, AccessToken: accessToken}
	locations, err := f.Resolver.Resolve(ctx, owner, refs)
	if err != nil {
		return nil, fmt.Errorf("resolving places: %w", err)
	}

	places := make([]location.Location, 0, len(locations))
	for _, loc := range locations {
		readableStructure := loc.Kind == location.KindStructure && loc.Accessible
		if readableStructure || (loc.Kind == location.KindStation && loc.LocationId != 0) {
			places = append(places, loc)
		}
	}
	slices.SortFunc(places, func(a, b location.Location) int {
		return cmp.Compare(a.Name, b.Name)
	})
	return places, nil
}

// isStationId is true for NPC station IDs.
func isStationId(id int64) bool {
	return id >= 60_000_000 && id < 64_000_000
}

// isStructureId is true for IDs in the range player structures use, which
// rules out NPC stations and solar systems. Item IDs share the range, so
// only pass IDs that ESI says are a facility or listing location.
func isStructureId(id int64) bool {
	return id >= 1_000_000_000_000
}
