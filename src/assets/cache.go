package assets

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"Stockpile-Manager/src/esi"
)

// ESI only refreshes asset lists hourly, so there's no point asking more
// often than assetCacheTTL. A refusal (no Director role, missing scope) is
// remembered for refusalBackoff, since ESI counts each one toward its error
// limit. A fetch runs for up to fetchTimeout whoever is waiting for it, so a
// reloaded page picks up the fetch already running instead of starting over.
const (
	assetCacheTTL  = 5 * time.Minute
	refusalBackoff = time.Hour
	fetchTimeout   = 5 * time.Minute
)

// TokenFunc returns a usable access token, refreshing it if needed.
type TokenFunc func(ctx context.Context) (string, error)

type assetFetch struct {
	done      chan struct{}
	value     any // the []Item or []Blueprint fetched
	count     int
	err       error
	fetchedAt time.Time
}

type assetCache struct {
	mu      sync.Mutex
	fetches map[string]*assetFetch // e.g. "character:<id>", "corp-blueprints:<characterId>"
}

// cachedGet returns the cached list for key, or waits for the fetch already
// running for it, or starts one.
func cachedGet[T any](ctx context.Context, c *assetCache, key string, fetch func(ctx context.Context) ([]T, error)) ([]T, error) {
	c.mu.Lock()
	if c.fetches == nil {
		c.fetches = map[string]*assetFetch{}
	}
	f := c.fetches[key]
	if f == nil || isExpired(f) {
		f = &assetFetch{done: make(chan struct{})}
		c.fetches[key] = f
		go func() {
			fetchCtx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
			defer cancel()
			start := time.Now()
			list, err := fetch(fetchCtx)
			f.value, f.count, f.err = list, len(list), err
			f.fetchedAt = time.Now()
			log.Printf("assets: %s: %d items in %s (err: %v)", key, f.count, time.Since(start).Round(time.Millisecond), f.err)
			close(f.done)
		}()
	}
	c.mu.Unlock()

	select {
	case <-f.done:
		list, _ := f.value.([]T)
		return list, f.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// isExpired is true once a finished fetch should be redone. Only refusals
// are kept for long; any other failure (a timeout, an ESI outage, an expired
// sign-in) is retried on the next request.
func isExpired(f *assetFetch) bool {
	select {
	case <-f.done:
	default:
		return false // still running
	}
	switch {
	case f.err == nil:
		return time.Since(f.fetchedAt) >= assetCacheTTL
	case errors.Is(f.err, esi.ErrForbidden):
		return time.Since(f.fetchedAt) >= refusalBackoff
	default:
		return true
	}
}

// CachedCharacterAssets is CharacterAssets, reusing a recent or running fetch.
func (f *Fetcher) CachedCharacterAssets(ctx context.Context, characterId int64, token TokenFunc) ([]Item, error) {
	return cachedGet(ctx, &f.cache, fmt.Sprintf("character:%d", characterId), func(ctx context.Context) ([]Item, error) {
		accessToken, err := token(ctx)
		if err != nil {
			return nil, err
		}
		return f.CharacterAssets(ctx, characterId, accessToken)
	})
}

// CachedCorporationAssets is CorporationAssets, reusing a recent or running
// fetch.
func (f *Fetcher) CachedCorporationAssets(ctx context.Context, characterId int64, token TokenFunc) ([]Item, error) {
	return cachedGet(ctx, &f.cache, fmt.Sprintf("corp:%d", characterId), func(ctx context.Context) ([]Item, error) {
		accessToken, err := token(ctx)
		if err != nil {
			return nil, err
		}
		return f.CorporationAssets(ctx, characterId, accessToken)
	})
}

// CachedCharacterBlueprints is CharacterBlueprints, reusing a recent or
// running fetch.
func (f *Fetcher) CachedCharacterBlueprints(ctx context.Context, characterId int64, token TokenFunc) ([]Blueprint, error) {
	return cachedGet(ctx, &f.cache, fmt.Sprintf("character-blueprints:%d", characterId), func(ctx context.Context) ([]Blueprint, error) {
		accessToken, err := token(ctx)
		if err != nil {
			return nil, err
		}
		return f.CharacterBlueprints(ctx, characterId, accessToken, token)
	})
}

// CachedCorporationBlueprints is CorporationBlueprints, reusing a recent or
// running fetch.
func (f *Fetcher) CachedCorporationBlueprints(ctx context.Context, characterId int64, token TokenFunc) ([]Blueprint, error) {
	return cachedGet(ctx, &f.cache, fmt.Sprintf("corp-blueprints:%d", characterId), func(ctx context.Context) ([]Blueprint, error) {
		accessToken, err := token(ctx)
		if err != nil {
			return nil, err
		}
		return f.CorporationBlueprints(ctx, characterId, accessToken, token)
	})
}
