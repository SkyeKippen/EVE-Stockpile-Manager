package sde

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSearch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "types.jsonl")
	lines := `{"_key": 34, "name": {"en": "Tritanium"}, "published": true}
{"_key": 4247, "name": {"en": "Helium Fuel Block"}, "published": true}
{"_key": 4314, "name": {"en": "Helium Fuel Block Blueprint"}, "published": true}
{"_key": 9999, "name": {"en": "Fuel Test"}, "published": false}
{"_key": 46166, "name": {"en": "Caesarium Cadmide Reaction Formula"}, "published": true}
`
	if err := os.WriteFile(path, []byte(lines), 0644); err != nil {
		t.Fatal(err)
	}
	loadOnce.Do(func() { loadErr = load(path) })
	if loadErr != nil {
		t.Fatal(loadErr)
	}

	got, _ := Search("fuel", false, 10)
	if len(got) != 2 || got[0].Name != "Helium Fuel Block" {
		t.Fatalf("fuel: got %+v", got)
	}
	got, _ = Search("fuel", true, 10)
	if len(got) != 1 || got[0].Id != 4314 {
		t.Fatalf("fuel blueprints: got %+v", got)
	}
	got, _ = Search("formula", true, 10)
	if len(got) != 1 || !got[0].IsBlueprint {
		t.Fatalf("formula: got %+v", got)
	}
	if _, ok := Get(9999); ok {
		t.Fatal("unpublished type was found")
	}
}
