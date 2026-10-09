package stockpile

import (
	"context"
	"slices"
	"strings"
	"sync"
)

// MemoryStore keeps everything in memory, so it's lost when the server
// restarts. It's used when config/.env has no DATABASE_URL.
type MemoryStore struct {
	mu         sync.Mutex
	nextId     int64
	stockpiles []Stockpile
	structures map[int64]Structure
}

func (m *MemoryStore) id() int64 {
	m.nextId++
	return m.nextId
}

func (m *MemoryStore) ListStockpiles(ctx context.Context) ([]Stockpile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Stockpile, 0, len(m.stockpiles))
	for _, s := range m.stockpiles {
		s.Structure = m.structures[s.Structure.StructureId]
		s.Items = slices.Clone(s.Items)
		slices.SortFunc(s.Items, func(a, b Item) int { return compare(a.Name, b.Name) })
		out = append(out, s)
	}
	slices.SortFunc(out, func(a, b Stockpile) int { return compare(a.Name, b.Name) })
	return out, nil
}

func (m *MemoryStore) CreateStockpile(ctx context.Context, name string, structureId int64, createdById int64, createdByName string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.structures[structureId]; !ok {
		return 0, ErrNotFound
	}
	id := m.id()
	m.stockpiles = append(m.stockpiles, Stockpile{
		StockpileId:   id,
		Name:          name,
		Structure:     Structure{StructureId: structureId},
		CreatedByName: createdByName,
	})
	return id, nil
}

func (m *MemoryStore) DeleteStockpile(ctx context.Context, stockpileId int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stockpiles = slices.DeleteFunc(m.stockpiles, func(s Stockpile) bool { return s.StockpileId == stockpileId })
	return nil
}

func (m *MemoryStore) SetItem(ctx context.Context, stockpileId int64, item Item) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	i := slices.IndexFunc(m.stockpiles, func(s Stockpile) bool { return s.StockpileId == stockpileId })
	if i < 0 {
		return ErrNotFound
	}
	s := &m.stockpiles[i]
	for j := range s.Items {
		if s.Items[j].TypeId == item.TypeId && s.Items[j].Kind == item.Kind {
			s.Items[j].Quantity = item.Quantity
			return nil
		}
	}
	item.StockpileItemId = m.id()
	s.Items = append(s.Items, item)
	return nil
}

func (m *MemoryStore) RemoveItem(ctx context.Context, stockpileId, stockpileItemId int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.stockpiles {
		if m.stockpiles[i].StockpileId == stockpileId {
			m.stockpiles[i].Items = slices.DeleteFunc(m.stockpiles[i].Items, func(it Item) bool {
				return it.StockpileItemId == stockpileItemId
			})
		}
	}
	return nil
}

func (m *MemoryStore) SaveStructures(ctx context.Context, structures []Structure) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.structures == nil {
		m.structures = make(map[int64]Structure)
	}
	for _, s := range structures {
		m.structures[s.StructureId] = s
	}
	return nil
}

func (m *MemoryStore) SearchStructures(ctx context.Context, query string, limit int) ([]Structure, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]Structure, 0)
	for _, s := range m.structures {
		if strings.Contains(strings.ToLower(s.Name), q) {
			out = append(out, s)
		}
	}
	slices.SortFunc(out, func(a, b Structure) int { return compare(a.Name, b.Name) })
	return out[:min(limit, len(out))], nil
}

func (m *MemoryStore) GetStructure(ctx context.Context, structureId int64) (Structure, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.structures[structureId]
	if !ok {
		return Structure{}, ErrNotFound
	}
	return s, nil
}

// compare turns Less into a sort function.
func compare(a, b string) int {
	switch {
	case Less(a, b):
		return -1
	case Less(b, a):
		return 1
	}
	return 0
}
