package exchange

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/shopspring/decimal"
)

// Frankfurter uses the ECB-backed v1 API: https://frankfurter.dev/v1/.
// It needs no API key and returns the previous business day on holidays/weekends.
type Frankfurter struct {
	baseURL string
	client  *http.Client
}

const frankfurterMaxBytes = 64 << 10

// NewFrankfurter builds a provider against the public Frankfurter API.
func NewFrankfurter() *Frankfurter {
	return &Frankfurter{
		baseURL: "https://api.frankfurter.dev/v1",
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (f *Frankfurter) Name() string { return "frankfurter" }

func (f *Frankfurter) Rate(ctx context.Context, base, quote string, day time.Time) (ProviderRate, error) {
	endpoint := f.baseURL + "/" + formatDate(day) + "?" + url.Values{"base": {base}, "symbols": {quote}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ProviderRate{}, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return ProviderRate{}, fmt.Errorf("frankfurter: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, frankfurterMaxBytes+1))
	if err != nil {
		return ProviderRate{}, fmt.Errorf("frankfurter: read response: %w", err)
	}
	if len(body) > frankfurterMaxBytes {
		return ProviderRate{}, fmt.Errorf("frankfurter: response larger than %d bytes", frankfurterMaxBytes)
	}
	if resp.StatusCode != http.StatusOK {
		return ProviderRate{}, fmt.Errorf("frankfurter: status %d: %s", resp.StatusCode, body)
	}

	var payload struct {
		Base  string                 `json:"base"`
		Date  string                 `json:"date"`
		Rates map[string]json.Number `json:"rates"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ProviderRate{}, fmt.Errorf("frankfurter: decode response: %w", err)
	}
	raw, ok := payload.Rates[quote]
	if payload.Base != base || !ok {
		return ProviderRate{}, fmt.Errorf("frankfurter: no %s/%s rate in response", base, quote)
	}
	// Parsed from the JSON text, never through float64.
	value, err := decimal.NewFromString(raw.String())
	if err != nil {
		return ProviderRate{}, fmt.Errorf("frankfurter: rate %q: %w", raw, err)
	}
	effective, err := time.Parse(dateLayout, payload.Date)
	if err != nil {
		return ProviderRate{}, fmt.Errorf("frankfurter: date %q: %w", payload.Date, err)
	}
	return ProviderRate{Value: value, EffectiveOn: effective}, nil
}
