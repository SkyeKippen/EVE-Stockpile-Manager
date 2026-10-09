package esi

import (
	"context"
	"log"
	"sync"
	"time"
)

// TypeNames names item types through ESI's /universe/names route and keeps
// every answer in memory, since type names don't change. It stands in for
// the SDE until the app has one.
type TypeNames struct {
	Client *Client

	mu    sync.Mutex
	names map[int64]string
}

// Names returns the name of each type ID, asking ESI only about ones it
// hasn't seen. IDs ESI doesn't know are left out.
func (t *TypeNames) Names(ctx context.Context, typeIds []int64) (map[int64]string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.names == nil {
		t.names = make(map[int64]string)
	}

	missing := make([]int64, 0)
	for _, id := range uniqueIds(typeIds) {
		if _, ok := t.names[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		fetched, err := t.Client.GetNames(ctx, missing)
		if err != nil {
			return nil, err
		}
		for id, name := range fetched {
			t.names[id] = name
		}
	}

	names := make(map[int64]string, len(typeIds))
	for _, id := range typeIds {
		if name, ok := t.names[id]; ok {
			names[id] = name
		}
	}
	return names, nil
}

// Name names one type, or returns "" when it can't. It fits
// location.Resolver's TypeName.
func (t *TypeNames) Name(typeId int64) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	names, err := t.Names(ctx, []int64{typeId})
	if err != nil {
		log.Printf("esi: naming type %d: %v", typeId, err)
		return ""
	}
	return names[typeId]
}
