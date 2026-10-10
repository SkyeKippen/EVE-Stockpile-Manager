package auth

import (
	"Stockpile-Manager/src/esi"
	"cmp"
	"context"
	"errors"
	"log"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// tokenDB, when set, keeps each character's refresh token in
// auth.character_tokens (see tools/db/generate_networth_tables.sql), so ESI
// can be called in the background for characters who signed in before the
// server restarted. Logging out leaves the saved token, so the hourly net
// worth snapshots keep running.
var tokenDB *pgxpool.Pool

// UseTokenDatabase saves refresh tokens in pool from now on.
func UseTokenDatabase(pool *pgxpool.Pool) {
	tokenDB = pool
}

// Character is a character with a saved token.
type Character struct {
	Id   int64
	Name string
}

// SavedCharacters returns every character whose token is kept, by name.
func SavedCharacters(ctx context.Context) ([]Character, error) {
	byId := map[int64]string{}
	tokens.mu.Lock()
	for id, token := range tokens.tokens {
		byId[id] = token.CharacterName
	}
	tokens.mu.Unlock()

	if tokenDB != nil {
		rows, err := tokenDB.Query(ctx, `SELECT character_id, character_name FROM auth.character_tokens`)
		if err != nil {
			return nil, err
		}
		var c Character
		_, err = pgx.ForEachRow(rows, []any{&c.Id, &c.Name}, func() error {
			byId[c.Id] = c.Name
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	characters := make([]Character, 0, len(byId))
	for id, name := range byId {
		characters = append(characters, Character{Id: id, Name: name})
	}
	slices.SortFunc(characters, func(a, b Character) int { return cmp.Compare(a.Name, b.Name) })
	return characters, nil
}

// persistToken saves token's refresh token, if there's a database.
func persistToken(ctx context.Context, token esi.CharacterToken) {
	if tokenDB == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, err := tokenDB.Exec(ctx, `
		INSERT INTO auth.character_tokens (character_id, character_name, refresh_token, scopes, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (character_id) DO UPDATE SET
			character_name = excluded.character_name, refresh_token = excluded.refresh_token,
			scopes = excluded.scopes, updated_at = now()`,
		token.CharacterID, token.CharacterName, token.RefreshToken, token.Scopes)
	if err != nil {
		log.Printf("evesso: saving token for character %d: %v", token.CharacterID, err)
	}
}

// loadToken reads a saved token, with no access token yet, so the caller
// refreshes it before use. It returns ErrNoToken when none is saved.
func loadToken(ctx context.Context, characterID int64) (esi.CharacterToken, error) {
	if tokenDB == nil {
		return esi.CharacterToken{}, ErrNoToken
	}
	token := esi.CharacterToken{CharacterID: characterID}
	err := tokenDB.QueryRow(ctx, `
		SELECT character_name, refresh_token, scopes FROM auth.character_tokens WHERE character_id = $1`,
		characterID).Scan(&token.CharacterName, &token.RefreshToken, &token.Scopes)
	if errors.Is(err, pgx.ErrNoRows) {
		return esi.CharacterToken{}, ErrNoToken
	}
	return token, err
}
