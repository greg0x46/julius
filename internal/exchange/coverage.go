package exchange

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Currency describes observation bounds, not guaranteed pair/date availability.
type Currency struct {
	Code      string `json:"code"`
	StartDate string `json:"start_date,omitempty"`
	EndDate   string `json:"end_date,omitempty"`
}

type Coverage struct {
	Currencies []Currency `json:"currencies"`
	Status     string     `json:"status"`
	Source     string     `json:"source"`
}

type CoverageProvider interface {
	Currencies(context.Context) Coverage
}

type coverageCache struct {
	mu         sync.Mutex
	currencies []Currency
	fetchedAt  time.Time
	failedAt   time.Time
}

func (s *Service) Currencies(ctx context.Context) Coverage {
	if p, ok := s.provider.(CoverageProvider); ok {
		return p.Currencies(ctx)
	}
	return Coverage{Currencies: []Currency{{Code: "BRL"}}, Status: "unavailable", Source: s.provider.Name()}
}

func (f *Frankfurter) Currencies(ctx context.Context) Coverage {
	c := &f.coverage
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	status := "complete"
	if c.fetchedAt.IsZero() || now.Sub(c.fetchedAt) >= 24*time.Hour {
		if c.failedAt.IsZero() || now.Sub(c.failedAt) >= time.Minute {
			currencies, err := f.fetchCurrencies(ctx)
			if err == nil {
				c.currencies, c.fetchedAt, c.failedAt = currencies, now, time.Time{}
			} else if ctx.Err() == nil {
				c.failedAt = now
			}
		}
		if c.fetchedAt.IsZero() {
			status = "unavailable"
		} else if now.Sub(c.fetchedAt) >= 24*time.Hour {
			status = "stale"
		}
	}
	currencies := append([]Currency(nil), c.currencies...)
	found := false
	for _, currency := range currencies {
		if currency.Code == "BRL" {
			found = true
		}
	}
	if !found {
		currencies = append(currencies, Currency{Code: "BRL"})
	}
	sort.Slice(currencies, func(i, j int) bool { return currencies[i].Code < currencies[j].Code })
	return Coverage{Currencies: currencies, Status: status, Source: f.Name()}
}

func (f *Frankfurter) fetchCurrencies(ctx context.Context) ([]Currency, error) {
	body, err := f.get(ctx, f.baseURL+"/currencies?scope=all")
	if err != nil {
		return nil, err
	}
	var payload []struct {
		Code      string `json:"iso_code"`
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty currency coverage")
	}
	currencies := make([]Currency, 0, len(payload))
	seen := map[string]bool{}
	for _, row := range payload {
		start, err := time.Parse(dateLayout, row.StartDate)
		if err != nil {
			return nil, err
		}
		end, err := time.Parse(dateLayout, row.EndDate)
		if err != nil {
			return nil, err
		}
		if !validCurrency(row.Code) || seen[row.Code] || end.Before(start) {
			return nil, fmt.Errorf("invalid currency coverage")
		}
		seen[row.Code] = true
		currencies = append(currencies, Currency{Code: row.Code, StartDate: row.StartDate, EndDate: row.EndDate})
	}
	return currencies, nil
}
