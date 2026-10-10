package auth

import (
	"Stockpile-Manager/src/esi"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"
)

// Tokens are kept in memory, and their refresh tokens also in the database
// when there is one (see token_db.go), so ESI can be called in the
// background after a restart.
type tokenStore struct {
	mu     sync.Mutex
	tokens map[int64]esi.CharacterToken // characterID -> token
}

var tokens = &tokenStore{tokens: make(map[int64]esi.CharacterToken)}

// refreshMargin refreshes an access token a little before it expires, so it
// doesn't run out partway through an ESI request.
const refreshMargin = time.Minute

// ErrNoToken means the character hasn't signed in since the server started.
var ErrNoToken = errors.New("evesso: no token for this character, sign in again")

func (s *tokenStore) save(token esi.CharacterToken) {
	s.mu.Lock()
	s.tokens[token.CharacterID] = token
	s.mu.Unlock()
}

func (s *tokenStore) delete(characterID int64) {
	s.mu.Lock()
	delete(s.tokens, characterID)
	s.mu.Unlock()
}

// HasScope reports whether the character's current sign-in granted scope.
// It's false when the character hasn't signed in since the server started.
func HasScope(characterID int64, scope string) bool {
	tokens.mu.Lock()
	defer tokens.mu.Unlock()
	token, ok := tokens.tokens[characterID]
	return ok && slices.Contains(token.Scopes, scope)
}

// AccessToken returns a usable ESI access token for the character, refreshing
// it with EVE SSO first if it has expired or is about to.
func AccessToken(ctx context.Context, characterID int64) (string, error) {
	if cfg == nil {
		return "", ErrNotConfigured
	}

	// held across the refresh so two requests don't both spend the refresh token
	tokens.mu.Lock()
	defer tokens.mu.Unlock()

	token, ok := tokens.tokens[characterID]
	if !ok {
		saved, err := loadToken(ctx, characterID)
		if err != nil {
			return "", err
		}
		token = saved
	}
	if time.Until(token.ExpiresAt) > refreshMargin {
		return token.AccessToken, nil
	}

	refreshed, err := cfg.RefreshAccessToken(ctx, token.RefreshToken)
	if err != nil {
		return "", fmt.Errorf("evesso: refreshing token for character %d: %w", characterID, err)
	}
	token.AccessToken = refreshed.AccessToken
	token.ExpiresAt = refreshed.ExpiresAt
	// EVE SSO may rotate the refresh token
	if refreshed.RefreshToken != "" && refreshed.RefreshToken != token.RefreshToken {
		token.RefreshToken = refreshed.RefreshToken
		persistToken(ctx, token)
	}
	tokens.tokens[characterID] = token
	return token.AccessToken, nil
}
