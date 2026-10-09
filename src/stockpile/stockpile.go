// Package stockpile stores stockpiles: named groups of items, and blueprint
// runs, wanted at one structure. Stockpiles are shared by everyone who signs
// in.
package stockpile

import (
	"context"
	"errors"
	"strings"
	"unicode"
)

// Item kinds, stored in stockpile_items.kind.
const (
	KindItem          = "item"           // Quantity is how many of the item
	KindBlueprintRuns = "blueprint_runs" // Quantity is runs of the blueprint
)

// ErrNotFound means the stockpile or structure doesn't exist.
var ErrNotFound = errors.New("stockpile: not found")

// Structure is a player structure a signed-in character could read.
type Structure struct {
	StructureId int64  `json:"id"`
	Name        string `json:"name"`
	System      string `json:"system"`
	Region      string `json:"region"`
	Type        string `json:"type"`
	Owner       string `json:"owner"`
}

type Stockpile struct {
	StockpileId   int64
	Name          string
	Structure     Structure
	CreatedByName string
	Items         []Item // sorted by name
}

type Item struct {
	StockpileItemId int64
	TypeId          int64
	Name            string
	Kind            string // KindItem or KindBlueprintRuns
	Quantity        int64
}

// Store keeps stockpiles and the structures they can be tied to. PgStore is
// the Postgres one; MemoryStore stands in when there's no DATABASE_URL.
type Store interface {
	// ListStockpiles returns every stockpile with its items, sorted by name
	// (see Less).
	ListStockpiles(ctx context.Context) ([]Stockpile, error)
	CreateStockpile(ctx context.Context, name string, structureId int64, createdById int64, createdByName string) (int64, error)
	DeleteStockpile(ctx context.Context, stockpileId int64) error
	// SetItem adds the item to the stockpile, or changes the wanted
	// quantity if the stockpile already has it as the same kind.
	SetItem(ctx context.Context, stockpileId int64, item Item) error
	RemoveItem(ctx context.Context, stockpileId, stockpileItemId int64) error

	// SaveStructures adds or updates structures a character could read.
	SaveStructures(ctx context.Context, structures []Structure) error
	// SearchStructures returns up to limit known structures whose name
	// contains query, ignoring case.
	SearchStructures(ctx context.Context, query string, limit int) ([]Structure, error)
	GetStructure(ctx context.Context, structureId int64) (Structure, error)
}

// Less orders names alphanumerically, ignoring case and comparing runs of
// digits by value, so "Stockpile 2" comes before "Stockpile 10".
func Less(a, b string) bool {
	ar, br := []rune(strings.ToLower(a)), []rune(strings.ToLower(b))
	i, j := 0, 0
	for i < len(ar) && j < len(br) {
		if unicode.IsDigit(ar[i]) && unicode.IsDigit(br[j]) {
			si, sj := i, j
			for i < len(ar) && unicode.IsDigit(ar[i]) {
				i++
			}
			for j < len(br) && unicode.IsDigit(br[j]) {
				j++
			}
			na := strings.TrimLeft(string(ar[si:i]), "0")
			nb := strings.TrimLeft(string(br[sj:j]), "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			continue
		}
		if ar[i] != br[j] {
			return ar[i] < br[j]
		}
		i++
		j++
	}
	if len(ar)-i != len(br)-j {
		return len(ar)-i < len(br)-j
	}
	return a < b
}
