package main

import (
	"Stockpile-Manager/src/auth"
	"Stockpile-Manager/src/networth"
	"context"
	"log"
	"net/http"
	"net/url"
	"time"
)

const (
	characterWalletScope   = "esi-wallet.read_character_wallet.v1"
	corporationWalletScope = "esi-wallet.read_corporation_wallets.v1"
)

// renderNetWorth graphs the signed-in character's net worth over time, with
// their corporation's added in when it's been readable.
func renderNetWorth(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.CurrentSession(r)

	data := struct {
		CharacterName string
		Tracked       bool
		Points        []networth.Point
		PriceDate     string
		MissingScopes []string
		Message       string
	}{
		CharacterName: sess.CharacterName,
		Tracked:       netWorth != nil,
		Points:        []networth.Point{},
		Message:       r.URL.Query().Get("message"),
	}

	if netWorth != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		owners := []networth.Owner{{Kind: networth.KindCharacter, Id: sess.CharacterID}}
		if corporationId, err := fetcher.ESI.GetCharacterCorporation(ctx, sess.CharacterID); err != nil {
			log.Printf("renderNetWorth: looking up corporation: %v", err)
		} else if !isNpcCorporation(corporationId) {
			owners = append(owners, networth.Owner{Kind: networth.KindCorporation, Id: corporationId})
		}

		readings, err := netWorth.History(ctx, owners)
		if err != nil {
			log.Printf("renderNetWorth: %v", err)
			http.Error(w, "could not load net worth history", http.StatusInternalServerError)
			return
		}
		data.Points = networth.Series(readings)

		if day, err := netWorth.LatestPriceDate(ctx); err != nil {
			log.Printf("renderNetWorth: %v", err)
		} else if !day.IsZero() {
			data.PriceDate = day.Format("2 Jan 2006")
		}

		for _, scope := range []string{characterWalletScope, corporationWalletScope} {
			if !auth.HasScope(sess.CharacterID, scope) {
				data.MissingScopes = append(data.MissingScopes, scope)
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "networth.html", data); err != nil {
		log.Printf("renderNetWorth: %v", err)
	}
}

// handleNetWorthSnapshot takes a snapshot for the signed-in character now,
// in the background, since fetching assets can take a minute or two.
func handleNetWorthSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if netWorth == nil {
		http.Redirect(w, r, "/networth", http.StatusSeeOther)
		return
	}
	sess, _ := auth.CurrentSession(r)
	go takeSnapshots([]auth.Character{{Id: sess.CharacterID, Name: sess.CharacterName}})
	q := url.Values{"message": {"Taking a snapshot of your wallets and assets. Reload in a minute or two to see it."}}
	http.Redirect(w, r, "/networth?"+q.Encode(), http.StatusSeeOther)
}
