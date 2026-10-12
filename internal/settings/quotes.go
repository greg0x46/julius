package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/greg0x46/julius/internal/db"
)

// KeyBrapiToken stores the optional brapi.dev API token the brapi provider
// in internal/marketdata sends as an Authorization: Bearer header. Encrypted like
// the Pluggy credentials it sits next to in Configurações.
const KeyBrapiToken = "quotes.brapi_token"

// GetBrapiToken reads the brapi token, decrypting it with unlockKey. ok is
// false when no token has ever been saved — internal/quotes treats that the
// same as an empty token, sending requests unauthenticated.
func GetBrapiToken(ctx context.Context, q Querier, unlockKey []byte) (token string, ok bool, err error) {
	return Get(ctx, q, KeyBrapiToken, unlockKey)
}

// SetBrapiToken stores the brapi token encrypted under unlockKey. An empty
// token is a valid value to store: it is how a previously configured token
// gets cleared.
func SetBrapiToken(ctx context.Context, q Querier, token string, unlockKey []byte) error {
	return Set(ctx, q, KeyBrapiToken, token, true, unlockKey)
}

const KeyQuoteRefresh = "quotes.refresh"

type QuoteRefreshSettings struct {
	Enabled  bool   `json:"enabled"`
	Time     string `json:"time"`
	Timezone string `json:"timezone"`
}

func DefaultQuoteRefreshSettings() QuoteRefreshSettings {
	return QuoteRefreshSettings{Enabled: true, Time: "19:00", Timezone: "America/Sao_Paulo"}
}

func (s QuoteRefreshSettings) Validate() error {
	clock, err := time.Parse("15:04", s.Time)
	if err != nil || clock.Format("15:04") != s.Time {
		return fmt.Errorf("invalid quote refresh time: %q", s.Time)
	}
	if s.Timezone == "" {
		return fmt.Errorf("quote refresh timezone is required")
	}
	if _, err := time.LoadLocation(s.Timezone); err != nil {
		return fmt.Errorf("invalid quote refresh timezone: %w", err)
	}
	return nil
}

func GetQuoteRefresh(ctx context.Context, q Querier) (QuoteRefreshSettings, bool, error) {
	value, ok, err := Get(ctx, q, KeyQuoteRefresh, nil)
	if err != nil || !ok {
		return DefaultQuoteRefreshSettings(), ok, err
	}
	var config QuoteRefreshSettings
	if err := json.Unmarshal([]byte(value), &config); err != nil {
		return config, true, err
	}
	return config, true, config.Validate()
}

func SetQuoteRefresh(ctx context.Context, q Querier, config QuoteRefreshSettings) error {
	if err := config.Validate(); err != nil {
		return err
	}
	value, err := json.Marshal(config)
	if err != nil {
		return err
	}
	return Set(ctx, q, KeyQuoteRefresh, string(value), false, nil)
}

// InitializeQuoteRefresh preserves a setting saved by another initializer.
func InitializeQuoteRefresh(ctx context.Context, q Querier, config QuoteRefreshSettings) (QuoteRefreshSettings, error) {
	if err := config.Validate(); err != nil {
		return config, err
	}
	value, err := json.Marshal(config)
	if err != nil {
		return config, err
	}
	_, err = q.ExecContext(ctx, `INSERT INTO settings (key, value, is_encrypted, updated_at) VALUES (?, ?, 0, ?) ON CONFLICT (key) DO NOTHING`, KeyQuoteRefresh, string(value), db.FormatTime(time.Now()))
	if err != nil {
		return config, err
	}
	saved, _, err := GetQuoteRefresh(ctx, q)
	return saved, err
}
