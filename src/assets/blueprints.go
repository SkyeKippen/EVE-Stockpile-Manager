package assets

import (
	"Stockpile-Manager/src/esi"
	"Stockpile-Manager/src/location"
	"context"
	"fmt"
	"log"
)

// Blueprint is one blueprint original or copy, or a stack of unused
// originals, and where it is.
type Blueprint struct {
	ItemId   int64
	TypeId   int64
	IsCopy   bool
	Runs     int64 // runs left on a copy; -1 for an original
	Location location.Location
}

// CharacterBlueprints needs the esi-characters.read_blueprints.v1 scope.
// Blueprints are also assets, so their locations come from the character's
// assets, which follows them into containers and ships.
func (f *Fetcher) CharacterBlueprints(ctx context.Context, characterId int64, accessToken string, token TokenFunc) ([]Blueprint, error) {
	raw, err := f.ESI.GetCharacterBlueprints(ctx, characterId, accessToken)
	if err != nil {
		return nil, fmt.Errorf("fetching blueprints of character %d: %w", characterId, err)
	}
	items, err := f.CachedCharacterAssets(ctx, characterId, token)
	if err != nil {
		log.Printf("blueprints: locating character %d's blueprints without assets: %v", characterId, err)
	}
	owner := location.Owner{CharacterId: characterId, AccessToken: accessToken, ReadCharacterAssets: true}
	return f.blueprints(ctx, owner, raw, items)
}

// CorporationBlueprints returns the blueprints of the character's
// corporation. It needs the esi-corporations.read_blueprints.v1 scope and the
// Director role in game.
func (f *Fetcher) CorporationBlueprints(ctx context.Context, characterId int64, accessToken string, token TokenFunc) ([]Blueprint, error) {
	corporationId, err := f.ESI.GetCharacterCorporation(ctx, characterId)
	if err != nil {
		return nil, fmt.Errorf("looking up corporation of character %d: %w", characterId, err)
	}
	raw, err := f.ESI.GetCorporationBlueprints(ctx, corporationId, accessToken)
	if err != nil {
		return nil, fmt.Errorf("fetching blueprints of corporation %d: %w", corporationId, err)
	}
	items, err := f.CachedCorporationAssets(ctx, characterId, token)
	if err != nil {
		log.Printf("blueprints: locating corporation %d's blueprints without assets: %v", corporationId, err)
	}
	owner := location.Owner{CharacterId: characterId, CorporationId: corporationId, AccessToken: accessToken}
	return f.blueprints(ctx, owner, raw, items)
}

// blueprints places each blueprint where its asset entry says it is. The
// blueprint and asset lists are cached separately by ESI, so a blueprint
// missing from the assets is looked up on its own.
func (f *Fetcher) blueprints(ctx context.Context, owner location.Owner, raw []esi.Blueprint, items []Item) ([]Blueprint, error) {
	byItemId := make(map[int64]location.Location, len(items))
	for _, item := range items {
		byItemId[item.ItemId] = item.Location
	}

	var missing []location.Ref
	for _, b := range raw {
		if _, ok := byItemId[b.ItemId]; !ok {
			missing = append(missing, location.Ref{LocationId: b.LocationId, LocationFlag: b.LocationFlag})
		}
	}
	var resolved map[location.Ref]location.Location
	if len(missing) > 0 {
		var err error
		resolved, err = f.Resolver.Resolve(ctx, owner, missing)
		if err != nil {
			return nil, fmt.Errorf("resolving blueprint locations: %w", err)
		}
	}

	blueprints := make([]Blueprint, 0, len(raw))
	for _, b := range raw {
		loc, ok := byItemId[b.ItemId]
		if !ok {
			loc = resolved[location.Ref{LocationId: b.LocationId, LocationFlag: b.LocationFlag}]
		}
		blueprints = append(blueprints, Blueprint{
			ItemId:   b.ItemId,
			TypeId:   b.TypeId,
			IsCopy:   b.IsCopy(),
			Runs:     b.Runs,
			Location: loc,
		})
	}
	return blueprints, nil
}
