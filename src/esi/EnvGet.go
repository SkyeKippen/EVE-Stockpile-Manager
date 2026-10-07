package esi

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

const EnvFile = "config/.env"

// LoadEnv loads EnvFile into the process environment. Variables that are
// already set are left untouched, so calling it more than once is harmless.
func LoadEnv() error {
	raw, err := os.ReadFile(EnvFile)
	if err != nil {
		return fmt.Errorf("loading %s: %w", EnvFile, err)
	}
	// Windows editors often save a UTF-8 byte order mark, which godotenv
	// would otherwise read as part of the first variable's name
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))

	vars, err := godotenv.UnmarshalBytes(raw)
	if err != nil {
		return fmt.Errorf("loading %s: %w", EnvFile, err)
	}
	for key, value := range vars {
		if _, set := os.LookupEnv(key); !set {
			os.Setenv(key, value)
		}
	}
	return nil
}

// LoadConfigFromEnv reads the EVE SSO app details from the environment, which
// LoadEnv fills from EnvFile. Copy config/.env.example to start one.
func LoadConfigFromEnv() (*Config, error) {
	if err := LoadEnv(); err != nil {
		return nil, fmt.Errorf("evesso: %w", err)
	}

	clientID := os.Getenv("ESI_CLIENT_ID")
	if clientID == "" {
		return nil, fmt.Errorf("evesso: ESI_CLIENT_ID is required")
	}
	redirectURI := os.Getenv("ESI_CALLBACK_URL")
	if redirectURI == "" {
		return nil, fmt.Errorf("evesso: ESI_CALLBACK_URL is required")
	}
	// publicData is always requested, so ESI_SCOPES may be left out
	scopes := splitScopes(os.Getenv("ESI_SCOPES"))
	if len(scopes) == 0 {
		scopes = []string{"publicData"}
	}

	usePKCE := true
	if v := os.Getenv("EVE_USE_PKCE"); v != "" {
		usePKCE = strings.EqualFold(v, "true") || v == "1"
	}

	return &Config{
		ClientID:     clientID,
		ClientSecret: os.Getenv("ESI_CLIENT_SECRET"),
		RedirectURI:  redirectURI,
		Scopes:       scopes,
		UsePKCE:      usePKCE,
	}, nil
}

func splitScopes(raw string) []string {
	raw = strings.ReplaceAll(raw, ",", " ")
	fields := strings.Fields(raw)
	return fields
}

func esiStatusError(url, status string, body []byte) error {
	return fmt.Errorf("esi: %s returned %s: %s", url, status, string(body))
}
