package main

import (
	"Stockpile-Manager/src/assets"
	"Stockpile-Manager/src/auth"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// fetcher is shared so stations, structures and type names are only asked
// about once while the server runs.
var fetcher = assets.NewFetcher()

// handleAssets returns the signed-in character's assets as JSON, or their
// corporation's with ?owner=corp.
func handleAssets(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.CurrentSession(r)
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	token, err := auth.AccessToken(ctx, sess.CharacterID)
	if err != nil {
		log.Printf("handleAssets: %v", err)
		http.Error(w, "your EVE sign-in has expired, sign in again", http.StatusUnauthorized)
		return
	}

	var items []assets.Item
	if r.URL.Query().Get("owner") == "corp" {
		items, err = fetcher.CorporationAssets(ctx, sess.CharacterID, token)
	} else {
		items, err = fetcher.CharacterAssets(ctx, sess.CharacterID, token)
	}
	if err != nil {
		log.Printf("handleAssets: %v", err)
		http.Error(w, "could not fetch assets from ESI", http.StatusBadGateway)
		return
	}
	writeJSON(w, items)
}

// handleStructures returns the structures the signed-in character can read,
// as JSON.
func handleStructures(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.CurrentSession(r)
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	token, err := auth.AccessToken(ctx, sess.CharacterID)
	if err != nil {
		log.Printf("handleStructures: %v", err)
		http.Error(w, "your EVE sign-in has expired, sign in again", http.StatusUnauthorized)
		return
	}

	structures, err := fetcher.CharacterPlaces(ctx, sess.CharacterID, token)
	if err != nil {
		log.Printf("handleStructures: %v", err)
		http.Error(w, "could not fetch structures from ESI", http.StatusBadGateway)
		return
	}
	writeJSON(w, structures)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		log.Printf("writeJSON: %v", err)
	}
}
