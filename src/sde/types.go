// Package sde reads item types from the EVE Static Data Export, so any item
// in the game can be named and searched for without asking ESI.
package sde

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

// TypesFile is the SDE's types table, copied from QS-Indy. It's gitignored
// because of its size.
const TypesFile = "data/sde/types.jsonl"

// Type is one published item type.
type Type struct {
	Id   int64  `json:"id"`
	Name string `json:"name"`
	// IsBlueprint is true for blueprints and reaction formulas. The types
	// table has no category, so this goes by name: every published one ends
	// in "Blueprint" or "Formula".
	IsBlueprint bool `json:"isBlueprint"`
	// BasePrice is the price NPCs sell the type for, where they do (e.g.
	// blueprint originals); 0 when the SDE gives none.
	BasePrice float64 `json:"-"`

	lower string
}

var (
	loadOnce sync.Once
	loadErr  error
	byId     map[int64]*Type
	sorted   []*Type // by lowercase name
)

// Load reads TypesFile the first time it's called; later calls return at
// once. Call it at startup in the background, since the file is large.
func Load() error {
	loadOnce.Do(func() {
		loadErr = load(TypesFile)
	})
	return loadErr
}

func load(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("sde: %w", err)
	}
	defer file.Close()

	byId = make(map[int64]*Type)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024) // some types have long descriptions
	for scanner.Scan() {
		var raw struct {
			Key       int64             `json:"_key"`
			Name      map[string]string `json:"name"`
			Published bool              `json:"published"`
			BasePrice float64           `json:"basePrice"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &raw); err != nil {
			return fmt.Errorf("sde: parsing %s: %w", path, err)
		}
		name := raw.Name["en"]
		if !raw.Published || name == "" {
			continue
		}
		t := &Type{
			Id:          raw.Key,
			Name:        name,
			IsBlueprint: strings.HasSuffix(name, " Blueprint") || strings.HasSuffix(name, " Formula"),
			BasePrice:   raw.BasePrice,
			lower:       strings.ToLower(name),
		}
		byId[t.Id] = t
		sorted = append(sorted, t)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("sde: reading %s: %w", path, err)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].lower < sorted[j].lower })
	return nil
}

// Get returns the published type with that ID.
func Get(id int64) (Type, bool) {
	if Load() != nil {
		return Type{}, false
	}
	t, ok := byId[id]
	if !ok {
		return Type{}, false
	}
	return *t, true
}

// Search returns up to limit published types whose name contains query,
// ignoring case. Names starting with query come first, then names with a
// word starting with it, then the rest; shorter names first within each.
// blueprintsOnly limits it to blueprints and reaction formulas.
func Search(query string, blueprintsOnly bool, limit int) ([]Type, error) {
	if err := Load(); err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" || limit <= 0 {
		return []Type{}, nil
	}

	type match struct {
		t    *Type
		rank int
	}
	var matches []match
	for _, t := range sorted {
		if blueprintsOnly && !t.IsBlueprint {
			continue
		}
		i := strings.Index(t.lower, q)
		if i < 0 {
			continue
		}
		rank := 2
		if i == 0 {
			rank = 0
		} else if strings.Contains(t.lower, " "+q) {
			rank = 1
		}
		matches = append(matches, match{t, rank})
	}
	// sorted is already alphabetical, so a stable sort keeps ties in order
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].rank != matches[j].rank {
			return matches[i].rank < matches[j].rank
		}
		return len(matches[i].t.Name) < len(matches[j].t.Name)
	})

	out := make([]Type, 0, min(limit, len(matches)))
	for _, m := range matches[:min(limit, len(matches))] {
		out = append(out, *m.t)
	}
	return out, nil
}
