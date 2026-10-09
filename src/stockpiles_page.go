package main

import (
	"Stockpile-Manager/src/auth"
	"Stockpile-Manager/src/location"
	"Stockpile-Manager/src/sde"
	"Stockpile-Manager/src/stockpile"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// stockpiles is where stockpiles are kept; see initStockpiles.
var (
	stockpiles      stockpile.Store
	stockpilesSaved bool // false when they only last until the server restarts
)

const (
	suggestionMinChars = 3
	suggestionLimit    = 10
	maxNameLength      = 100
)

// initStockpiles connects to DATABASE_URL, or keeps stockpiles in memory
// when it isn't set or can't be reached.
func initStockpiles() {
	databaseUrl := os.Getenv("DATABASE_URL")
	if databaseUrl == "" {
		log.Printf("stockpiles: DATABASE_URL isn't set, so stockpiles only last until the server restarts")
		stockpiles = &stockpile.MemoryStore{}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	store, err := stockpile.NewPgStore(ctx, databaseUrl)
	if err != nil {
		log.Printf("stockpiles: connecting to DATABASE_URL: %v; stockpiles only last until the server restarts", err)
		stockpiles = &stockpile.MemoryStore{}
		return
	}
	stockpiles = store
	stockpilesSaved = true
}

// refreshStructures saves the stations and structures the character can
// tie a stockpile to (see assets.Fetcher.CharacterPlaces), so they
// can be picked when creating a stockpile.
func refreshStructures(ctx context.Context, characterId int64) (int, error) {
	token, err := auth.AccessToken(ctx, characterId)
	if err != nil {
		return 0, err
	}
	found, err := fetcher.CharacterPlaces(ctx, characterId, token)
	if err != nil {
		return 0, err
	}
	structures := make([]stockpile.Structure, 0, len(found))
	for _, s := range found {
		structures = append(structures, structureFromLocation(s))
	}
	if err := stockpiles.SaveStructures(ctx, structures); err != nil {
		return 0, err
	}
	return len(structures), nil
}

func structureFromLocation(l location.Location) stockpile.Structure {
	return stockpile.Structure{
		StructureId: l.LocationId,
		Name:        l.Name,
		System:      l.System,
		Region:      l.Region,
		Type:        l.Type,
		Owner:       l.Owner,
	}
}

// refreshStructuresOnSignIn is auth.OnSignIn.
func refreshStructuresOnSignIn(characterId int64, characterName string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	n, err := refreshStructures(ctx, characterId)
	if err != nil {
		log.Printf("stockpiles: refreshing structures for %s (%d): %v", characterName, characterId, err)
		return
	}
	log.Printf("stockpiles: %s (%d) has %d stations and structures to pick from", characterName, characterId, n)
}

func renderStockpiles(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.CurrentSession(r)

	list, err := stockpiles.ListStockpiles(r.Context())
	if err != nil {
		log.Printf("renderStockpiles: %v", err)
		http.Error(w, "could not load stockpiles", http.StatusInternalServerError)
		return
	}
	open, _ := strconv.ParseInt(r.URL.Query().Get("open"), 10, 64)

	data := struct {
		CharacterName string
		Saved         bool
		Stockpiles    []stockpile.Stockpile
		Open          int64 // the stockpile to show expanded
		Message       string
		Error         string
		KindItem      string
		KindRuns      string
	}{
		CharacterName: sess.CharacterName,
		Saved:         stockpilesSaved,
		Stockpiles:    list,
		Open:          open,
		Message:       r.URL.Query().Get("message"),
		Error:         r.URL.Query().Get("error"),
		KindItem:      stockpile.KindItem,
		KindRuns:      stockpile.KindBlueprintRuns,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "stockpiles.html", data); err != nil {
		log.Printf("renderStockpiles: %v", err)
	}
}

// backToStockpiles redirects to the Stockpiles page, showing a message or an
// error, with stockpile open expanded (0 for none).
func backToStockpiles(w http.ResponseWriter, r *http.Request, open int64, message, errMessage string) {
	q := url.Values{}
	if open != 0 {
		q.Set("open", strconv.FormatInt(open, 10))
	}
	if message != "" {
		q.Set("message", message)
	}
	if errMessage != "" {
		q.Set("error", errMessage)
	}
	target := "/stockpiles"
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	if open != 0 {
		target += fmt.Sprintf("#stockpile-%d", open)
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func handleCreateStockpile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sess, _ := auth.CurrentSession(r)

	name := strings.TrimSpace(r.FormValue("name"))
	structureId, _ := strconv.ParseInt(r.FormValue("structure_id"), 10, 64)
	switch {
	case name == "":
		backToStockpiles(w, r, 0, "", "Give the stockpile a name.")
		return
	case len([]rune(name)) > maxNameLength:
		backToStockpiles(w, r, 0, "", fmt.Sprintf("Stockpile names can be up to %d characters.", maxNameLength))
		return
	case structureId == 0:
		backToStockpiles(w, r, 0, "", "Pick a structure from the suggestions.")
		return
	}

	id, err := stockpiles.CreateStockpile(r.Context(), name, structureId, sess.CharacterID, sess.CharacterName)
	if errors.Is(err, stockpile.ErrNotFound) {
		backToStockpiles(w, r, 0, "", "That structure isn't one we know about. Pick one from the suggestions.")
		return
	}
	if err != nil {
		log.Printf("handleCreateStockpile: %v", err)
		http.Error(w, "could not create the stockpile", http.StatusInternalServerError)
		return
	}
	backToStockpiles(w, r, id, "Created "+name+".", "")
}

func handleDeleteStockpile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, _ := strconv.ParseInt(r.FormValue("stockpile_id"), 10, 64)
	if err := stockpiles.DeleteStockpile(r.Context(), id); err != nil {
		log.Printf("handleDeleteStockpile: %v", err)
		http.Error(w, "could not delete the stockpile", http.StatusInternalServerError)
		return
	}
	backToStockpiles(w, r, 0, "Deleted the stockpile.", "")
}

func handleAddStockpileItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	stockpileId, _ := strconv.ParseInt(r.FormValue("stockpile_id"), 10, 64)
	typeId, _ := strconv.ParseInt(r.FormValue("type_id"), 10, 64)
	kind := r.FormValue("kind")
	if kind != stockpile.KindBlueprintRuns {
		kind = stockpile.KindItem
	}

	t, ok := sde.Get(typeId)
	if !ok {
		backToStockpiles(w, r, stockpileId, "", "Pick an item from the suggestions.")
		return
	}
	if kind == stockpile.KindBlueprintRuns && !t.IsBlueprint {
		backToStockpiles(w, r, stockpileId, "", t.Name+" isn't a blueprint, so it can't be added as blueprint runs.")
		return
	}
	quantity, ok := stockpile.ParseQuantity(r.FormValue("quantity"))
	if !ok {
		backToStockpiles(w, r, stockpileId, "", "Enter a whole number above 0, like 1200, 1,200 or 1.2k.")
		return
	}

	err := stockpiles.SetItem(r.Context(), stockpileId, stockpile.Item{TypeId: t.Id, Name: t.Name, Kind: kind, Quantity: quantity})
	if errors.Is(err, stockpile.ErrNotFound) {
		backToStockpiles(w, r, 0, "", "That stockpile no longer exists.")
		return
	}
	if err != nil {
		log.Printf("handleAddStockpileItem: %v", err)
		http.Error(w, "could not add the item", http.StatusInternalServerError)
		return
	}
	backToStockpiles(w, r, stockpileId, "", "")
}

func handleRemoveStockpileItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	stockpileId, _ := strconv.ParseInt(r.FormValue("stockpile_id"), 10, 64)
	itemId, _ := strconv.ParseInt(r.FormValue("stockpile_item_id"), 10, 64)
	if err := stockpiles.RemoveItem(r.Context(), stockpileId, itemId); err != nil {
		log.Printf("handleRemoveStockpileItem: %v", err)
		http.Error(w, "could not remove the item", http.StatusInternalServerError)
		return
	}
	backToStockpiles(w, r, stockpileId, "", "")
}

func handleRefreshStructures(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sess, _ := auth.CurrentSession(r)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	n, err := refreshStructures(ctx, sess.CharacterID)
	if errors.Is(err, auth.ErrNoToken) {
		backToStockpiles(w, r, 0, "", "Your EVE sign-in has expired. Sign in again to refresh structures.")
		return
	}
	if err != nil {
		log.Printf("handleRefreshStructures: %v", err)
		backToStockpiles(w, r, 0, "", "Could not fetch your structures from ESI. Try again in a minute.")
		return
	}
	backToStockpiles(w, r, 0, fmt.Sprintf("Found %d stations and structures for %s.", n, sess.CharacterName), "")
}

// handleItemSuggestions returns item types matching ?q= as JSON, for the Add
// Item dialog. ?kind=blueprint_runs suggests only blueprints.
func handleItemSuggestions(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	types := []sde.Type{}
	if len([]rune(q)) >= suggestionMinChars {
		var err error
		types, err = sde.Search(q, r.URL.Query().Get("kind") == stockpile.KindBlueprintRuns, suggestionLimit)
		if err != nil {
			log.Printf("handleItemSuggestions: %v", err)
			http.Error(w, "item lookup failed", http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, types)
}

// handleStructureSuggestions returns known structures matching ?q= as JSON,
// for the New Stockpile dialog.
func handleStructureSuggestions(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	structures := []stockpile.Structure{}
	if len([]rune(q)) >= suggestionMinChars {
		var err error
		structures, err = stockpiles.SearchStructures(r.Context(), q, suggestionLimit)
		if err != nil {
			log.Printf("handleStructureSuggestions: %v", err)
			http.Error(w, "structure lookup failed", http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, structures)
}
