package demo_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/demo"
	"github.com/greg0x46/julius/internal/httpapi"
	"github.com/greg0x46/julius/internal/settings"
)

const origin = "https://demo.example"

var master = []byte("0123456789abcdef0123456789abcdef")

func seed(t *testing.T, conn *sql.DB, now time.Time) demo.Summary {
	t.Helper()
	summary, err := demo.Seed(context.Background(), conn, demo.Options{Master: master, PublicURL: origin, Now: now})
	if err != nil {
		t.Fatalf("Seed(%s): %v", now.Format("2006-01-02"), err)
	}
	return summary
}

func server(conn *sql.DB) http.Handler {
	return httpapi.NewServer(conn, fstest.MapFS{"index.html": {Data: []byte("spa")}}, settings.NewSecrets(master), auth.Config{PublicURL: origin, Demo: true}, nil)
}

func get(t *testing.T, h http.Handler, path string) []byte {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: %d %s", path, rec.Code, rec.Body.String())
	}
	return rec.Body.Bytes()
}

func count(t *testing.T, h http.Handler, path string) int {
	t.Helper()
	body := get(t, h, path)
	var items []json.RawMessage
	if err := json.Unmarshal(body, &items); err == nil {
		return len(items)
	}
	var wrapped struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil || wrapped.Items == nil {
		t.Fatalf("GET %s: neither a list nor {items}: %s", path, body)
	}
	return len(wrapped.Items)
}

func TestSeedBuildsAnOpenDemo(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		// Seed relative to now: the portfolio is valued as of today and treats old quotes as stale.
		summary := seed(t, conn, time.Now().UTC())
		if summary.Transactions < 100 || summary.Transfers != 6 || summary.Positions != 4 {
			t.Fatalf("summary = %+v", summary)
		}

		h := server(conn)
		var got map[string]any
		if err := json.Unmarshal(get(t, h, "/api/auth/session"), &got); err != nil {
			t.Fatal(err)
		}
		if got["authentication_enabled"] != false || got["demo"] != true {
			t.Fatalf("session = %v, want open demo", got)
		}
		if enabled, err := settings.AuthenticationEnabled(context.Background(), conn); err != nil || enabled {
			t.Fatalf("stored authentication = %v, %v; want disabled", enabled, err)
		}

		for path, want := range map[string]int{
			"/api/accounts": 3, "/api/goals": 2, "/api/payables": 2,
			"/api/recurring-scenarios": 6, "/api/automation-rules": 1,
			"/api/investment-accounts": 1, "/api/investment-positions": 4,
		} {
			if n := count(t, h, path); n != want {
				t.Errorf("GET %s: %d items, want %d", path, n, want)
			}
		}
		get(t, h, "/api/net-worth")

		// What a visitor sees must look like a healthy household.
		var accounts []struct{ Name, Balance string }
		if err := json.Unmarshal(get(t, h, "/api/accounts"), &accounts); err != nil {
			t.Fatal(err)
		}
		for _, a := range accounts {
			if decimal.RequireFromString(a.Balance).IsNegative() {
				t.Errorf("account %q has a negative balance %s", a.Name, a.Balance)
			}
		}
		var positions struct {
			Items []struct {
				Name         string
				CurrentValue string `json:"current_value"`
			}
		}
		if err := json.Unmarshal(get(t, h, "/api/investment-positions"), &positions); err != nil {
			t.Fatal(err)
		}
		var portfolio struct {
			Totals struct {
				Unavailable int  `json:"positions_unavailable"`
				Partial     bool `json:"partial"`
			}
		}
		if err := json.Unmarshal(get(t, h, "/api/investment-portfolio"), &portfolio); err != nil {
			t.Fatal(err)
		}
		if portfolio.Totals.Unavailable != 0 || portfolio.Totals.Partial {
			t.Errorf("portfolio totals = %+v, want every position valued", portfolio.Totals)
		}
		for _, p := range positions.Items {
			if !decimal.RequireFromString(p.CurrentValue).IsPositive() {
				t.Errorf("position %q is worth %q: listed assets need quotes", p.Name, p.CurrentValue)
			}
		}
	})
}

func TestSeedFillsOnlyAFreshDatabase(t *testing.T) {
	conn := dbtest.OpenSQLite(t)
	seed(t, conn, time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC))
	_, err := demo.Seed(context.Background(), conn, demo.Options{Master: master, PublicURL: origin, Now: time.Now()})
	if !errors.Is(err, demo.ErrNotEmpty) {
		t.Fatalf("second Seed error = %v, want ErrNotEmpty", err)
	}
}

// Month-end and leap days are where date arithmetic breaks, and the demo is
// re-seeded every day.
func TestSeedWorksOnAnyDayOfTheYear(t *testing.T) {
	for _, now := range []time.Time{
		time.Date(2026, 1, 31, 23, 30, 0, 0, time.UTC),
		time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2028, 2, 29, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 12, 31, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC),
	} {
		t.Run(now.Format("2006-01-02"), func(t *testing.T) {
			seed(t, dbtest.OpenSQLite(t), now)
		})
	}
}
