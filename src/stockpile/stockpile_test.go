package stockpile

import (
	"slices"
	"testing"
)

func TestLessSortsAlphanumerically(t *testing.T) {
	names := []string{"stockpile 10", "Fuel", "Stockpile 2", "ammo", "Stockpile 1"}
	slices.SortFunc(names, func(a, b string) int { return compare(a, b) })
	want := []string{"ammo", "Fuel", "Stockpile 1", "Stockpile 2", "stockpile 10"}
	if !slices.Equal(names, want) {
		t.Fatalf("got %v, want %v", names, want)
	}
}

func TestParseQuantity(t *testing.T) {
	good := map[string]int64{"1200": 1200, "1,200": 1200, " 1.2k ": 1200, "3M": 3_000_000, "1b": 1_000_000_000}
	for in, want := range good {
		if got, ok := ParseQuantity(in); !ok || got != want {
			t.Errorf("ParseQuantity(%q) = %d, %v; want %d", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "0", "-5", "1.5", "abc", "2000b"} {
		if got, ok := ParseQuantity(in); ok {
			t.Errorf("ParseQuantity(%q) = %d, want rejected", in, got)
		}
	}
}
