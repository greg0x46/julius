package exchange

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func frankfurterAt(t *testing.T, handler http.HandlerFunc) *Frankfurter {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	f := NewFrankfurter()
	f.baseURL = server.URL
	f.client = server.Client()
	return f
}

func TestFrankfurterParsesRateExactly(t *testing.T) {
	f := frankfurterAt(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/2026-10-10" || r.URL.Query().Get("base") != "USD" || r.URL.Query().Get("symbols") != "BRL" {
			t.Errorf("request %s", r.URL)
		}
		w.Write([]byte(`{"amount":1.0,"base":"USD","date":"2026-10-09","rates":{"BRL":5.12345678901234567}}`))
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
		"missing quote": func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"amount":1.0,"base":"USD","date":"2026-10-09","rates":{}}`))
		},
		"other base": func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"amount":1.0,"base":"EUR","date":"2026-10-09","rates":{"BRL":5.6}}`))
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
