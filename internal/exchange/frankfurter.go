package exchange

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/shopspring/decimal"
)

// Frankfurter uses v2 blended daily observations from official sources.
type Frankfurter struct {
	baseURL  string
	client   *http.Client
	coverage coverageCache
}

const frankfurterMaxBytes = 64 << 10

// NewFrankfurter builds a provider against the public Frankfurter API.
func NewFrankfurter() *Frankfurter {
	return &Frankfurter{
		baseURL: "https://api.frankfurter.dev/v2",
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (f *Frankfurter) Name() string { return "frankfurter-v2-blended" }

func (f *Frankfurter) Rate(ctx context.Context, base, quote string, day time.Time) (ProviderRate, error) {
	coverage := f.Currencies(ctx)
	knownPair := coverage.Status != "unavailable"
	if coverage.Status != "unavailable" {
		for _, code := range []string{base, quote} {
			found := false
			for _, c := range coverage.Currencies {
				if c.Code == code {
					found = true
					if c.StartDate != "" && formatDate(day) < c.StartDate {
						return ProviderRate{}, ErrHistorical
					}
					break
				}
			}
			if !found {
				knownPair = false
			}
			if !found && coverage.Status == "complete" {
				return ProviderRate{}, fmt.Errorf("%w: %s", ErrUnsupported, code)
			}
		}
	}
	endpoint := f.baseURL + "/rate/" + base + "/" + quote + "?" + url.Values{"date": {formatDate(day)}}.Encode()
	body, err := f.get(ctx, endpoint)
	if err != nil {
		if errors.Is(err, ErrHistorical) && !knownPair {
			return ProviderRate{}, ErrProviderFailure
		}
		return ProviderRate{}, err
	}

	var payload struct {
		Base  string      `json:"base"`
		Quote string      `json:"quote"`
		Date  string      `json:"date"`
		Rate  json.Number `json:"rate"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ProviderRate{}, fmt.Errorf("frankfurter: decode response: %w", err)
	}
	raw := payload.Rate
	if payload.Base != base || payload.Quote != quote || raw == "" {
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

func (f *Frankfurter) get(ctx context.Context, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProviderFailure, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, frankfurterMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read response: %v", ErrProviderFailure, err)
	}
	if len(body) > frankfurterMaxBytes {
		return nil, fmt.Errorf("%w: oversized response", ErrProviderFailure)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrHistorical
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrProviderFailure, resp.StatusCode)
	}
	return body, nil
}
