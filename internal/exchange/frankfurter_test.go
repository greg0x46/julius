package exchange

import (
	"context"
	"database/sql"
	"errors"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func frankfurterAt(t *testing.T, handler http.HandlerFunc) *Frankfurter {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/currencies" {
			w.Write([]byte(`[{"iso_code":"BRL","start_date":"1994-07-01","end_date":"2026-10-14"},{"iso_code":"USD","start_date":"1971-01-01","end_date":"2026-10-14"},{"iso_code":"ARS","start_date":"1998-07-07","end_date":"2026-10-14"}]`))
			return
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	f := NewFrankfurter()
	f.baseURL = server.URL
	f.client = server.Client()
	return f
}

func TestFrankfurterParsesRateExactly(t *testing.T) {
	f := frankfurterAt(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rate/USD/BRL" || r.URL.Query().Get("date") != "2026-10-10" {
			t.Errorf("request %s", r.URL)
		}
		w.Write([]byte(`{"base":"USD","quote":"BRL","date":"2026-10-09","rate":5.12345678901234567}`))
	})
	got, err := f.Rate(context.Background(), "USD", "BRL", sat)
	if err != nil {
		t.Fatal(err)
	}
	if got.Value.String() != "5.12345678901234567" || !got.EffectiveOn.Equal(fri) {
		t.Fatalf("got %+v", got)
	}
}

func TestFrankfurterErrors(t *testing.T) {
	for name, handler := range map[string]http.HandlerFunc{
		"not found": func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		},
		"rate limited": func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "try later", http.StatusTooManyRequests)
		},
		"invalid date": func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"base":"USD","quote":"BRL","date":"invalid","rate":5}`))
		},
		"missing quote": func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"base":"USD","date":"2026-10-09"}`))
		},
		"other base": func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"base":"EUR","quote":"BRL","date":"2026-10-09","rate":5.6}`))
		},
		"oversized": func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(strings.Repeat(" ", frankfurterMaxBytes+1)))
		},
		"malformed": func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"rates":`))
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := frankfurterAt(t, handler).Rate(context.Background(), "USD", "BRL", sat); err == nil {
				t.Fatal("want error")
			}
		})
	}
}

func TestFrankfurterCoverageFailureKinds(t *testing.T) {
	calls := 0
	f := frankfurterAt(t, func(w http.ResponseWriter, r *http.Request) { calls++; http.NotFound(w, r) })
	for _, tc := range []struct {
		base string
		day  time.Time
		want error
	}{
		{"XTS", sat, ErrUnsupported}, {"ARS", day(1950, 1, 1), ErrHistorical}, {"ARS", sat, ErrHistorical},
	} {
		if _, err := f.Rate(context.Background(), tc.base, "BRL", tc.day); !errors.Is(err, tc.want) {
			t.Fatalf("%s: %v", tc.base, err)
		}
	}
	if calls != 1 {
		t.Fatalf("rate calls = %d", calls)
	}
	f = frankfurterAt(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) })
	if _, err := f.Rate(context.Background(), "ARS", "BRL", sat); !errors.Is(err, ErrProviderFailure) {
		t.Fatal(err)
	}
}

func TestFrankfurterV2ConversionAndLegacyCache(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		f := frankfurterAt(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/rate/ARS/BRL" {
				t.Errorf("request: %s", r.URL)
			}
			w.Write([]byte(`{"base":"ARS","quote":"BRL","date":"2026-10-09","rate":0.005789123456789012}`))
		})
		s, _ := newService(conn, f, sun)
		ctx := context.Background()
		got, err := s.On(ctx, "ARS", "BRL", sat)
		if err != nil || got.Source != "frankfurter-v2-blended" || got.Convert(dec("1000")).String() != "5.789123456789012" {
			t.Fatalf("%+v %v", got, err)
		}
		inverse, err := s.On(ctx, "BRL", "ARS", sat)
		if err != nil || !inverse.Inverted || inverse.Source != got.Source {
			t.Fatalf("%+v %v", inverse, err)
		}
		legacy := Rate{Base: "USD", Quote: "BRL", Value: dec("5"), AsOf: sat, EffectiveOn: fri, Source: "frankfurter", FetchedAt: sun}
		if err := s.save(ctx, legacy); err != nil {
			t.Fatal(err)
		}
		got, err = s.On(ctx, "USD", "BRL", sat)
		if err != nil || got.Source != "frankfurter" || got.Convert(dec("10")).String() != "50" {
			t.Fatalf("%+v %v", got, err)
		}
	})
}

func TestCurrencyCoverageCacheAndOutage(t *testing.T) {
	calls := 0
	offline := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if offline {
			w.WriteHeader(503)
			return
		}
		w.Write([]byte(`[{"iso_code":"ARS","start_date":"1998-07-07","end_date":"2026-10-14"}]`))
	}))
	defer server.Close()
	f := NewFrankfurter()
	f.baseURL = server.URL
	f.client = server.Client()
	ctx := context.Background()
	for range 2 {
		if got := f.Currencies(ctx); got.Status != "complete" || len(got.Currencies) != 2 {
			t.Fatalf("%+v", got)
		}
	}
	if calls != 1 {
		t.Fatal(calls)
	}
	f.coverage.fetchedAt = time.Now().Add(-25 * time.Hour)
	offline = true
	for range 2 {
		if got := f.Currencies(ctx); got.Status != "stale" || len(got.Currencies) != 2 {
			t.Fatalf("%+v", got)
		}
	}
	if calls != 2 {
		t.Fatal(calls)
	}
	f = NewFrankfurter()
	f.baseURL = server.URL
	f.client = server.Client()
	for range 2 {
		if got := f.Currencies(ctx); got.Status != "unavailable" || len(got.Currencies) != 1 || got.Currencies[0].Code != "BRL" {
			t.Fatalf("%+v", got)
		}
	}
	if calls != 3 {
		t.Fatal(calls)
	}
}

func TestFrankfurterObservationDates(t *testing.T) {
	for _, tc := range []struct{ requested, effective string }{
		{"2024-05-18", "2024-05-18"},
		{"2024-05-19", "2024-05-17"},
		{"2024-12-25", "2024-12-24"},
	} {
		t.Run(tc.requested, func(t *testing.T) {
			f := frankfurterAt(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("date") != tc.requested {
					t.Errorf("date %s", r.URL)
				}
				w.Write([]byte(`{"base":"ARS","quote":"BRL","date":"` + tc.effective + `","rate":0.00578}`))
			})
			requested, _ := time.Parse(dateLayout, tc.requested)
			got, err := f.Rate(context.Background(), "ARS", "BRL", requested)
			if err != nil || formatDate(got.EffectiveOn) != tc.effective {
				t.Fatalf("%+v %v", got, err)
			}
		})
	}
}
