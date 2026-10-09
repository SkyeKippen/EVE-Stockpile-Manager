package location

import (
	"context"
	"sync"
)

// MemoryStore caches places in memory, so they last until the server
// restarts. Swap it for a database-backed Store once there is a table for
// places.
type MemoryStore struct {
	mu     sync.Mutex
	places map[int64]Place
}

func (s *MemoryStore) GetPlaces(ctx context.Context, locationIds []int64) (map[int64]Place, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	found := make(map[int64]Place, len(locationIds))
	for _, id := range locationIds {
		if place, ok := s.places[id]; ok {
			found[id] = place
		}
	}
	return found, nil
}

func (s *MemoryStore) SavePlace(ctx context.Context, place Place) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.places == nil {
		s.places = make(map[int64]Place)
	}
	s.places[place.LocationId] = place
	return nil
}
