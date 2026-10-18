package pluggy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"
)

// fakeWriter records every envelope it's asked to persist, standing in for
// a real raw_imports-backed writer.
type fakeWriter struct {
	envelopes []RawResponseEnvelope
}

func (w *fakeWriter) Write(_ context.Context, envelope RawResponseEnvelope) (string, error) {
	w.envelopes = append(w.envelopes, envelope)
	return fmt.Sprintf("raw-%d", len(w.envelopes)), nil
}

func newTestAdapter(t *testing.T, mux *http.ServeMux) (*Adapter, *fakeWriter) {
	t.Helper()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	writer := &fakeWriter{}
	cfg := DefaultConfig()
	cfg.BaseURL = server.URL
	cfg.ItemID = "item-1"
	cfg.MaxAttempts = 3
	adapter := NewAdapter(cfg, writer, server.Client())
	adapter.sleep = func(context.Context, time.Duration) error { return nil } // no real waiting in tests
	return adapter, writer
}

func authHandler(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"apiKey": "test-key"})
}

func TestAdapterGetSourceHappyPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", authHandler)
	mux.HandleFunc("/items/item-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-KEY") != "test-key" {
			t.Errorf("missing api key header")
		}
		json.NewEncoder(w).Encode(map[string]any{
			"id": "item-1", "status": "UPDATED", "executionStatus": "SUCCESS",
			"connector": map[string]string{"name": "Banco Exemplo"},
		})
	})
	adapter, writer := newTestAdapter(t, mux)

	source, rawImportID, err := adapter.GetSource(context.Background())
	if err != nil {
		t.Fatalf("GetSource: %v", err)
	}
	if source.InstitutionName == nil || *source.InstitutionName != "Banco Exemplo" {
		t.Errorf("InstitutionName = %v", source.InstitutionName)
	}
	if !source.SafeProducts["ACCOUNTS"] || !source.SafeProducts["TRANSACTIONS"] {
		t.Errorf("SafeProducts = %v", source.SafeProducts)
	}
	if rawImportID == "" {
		t.Error("expected a raw import id")
	}
	if len(writer.envelopes) != 1 {
		t.Fatalf("len(envelopes) = %d, want 1", len(writer.envelopes))
	}
}

func TestAdapterGetSourceRejectsLoginError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", authHandler)
	mux.HandleFunc("/items/item-1", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"id": "item-1", "status": "LOGIN_ERROR"})
	})
	adapter, _ := newTestAdapter(t, mux)

	_, _, err := adapter.GetSource(context.Background())
	provErr, ok := err.(*ProviderError)
	if !ok || provErr.Code != "item_invalid_credentials" {
		t.Errorf("err = %v, want item_invalid_credentials", err)
	}
}

func TestAdapterAuthRejectsInvalidCredentials(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	adapter, _ := newTestAdapter(t, mux)

	_, _, err := adapter.GetSource(context.Background())
	provErr, ok := err.(*ProviderError)
	if !ok || provErr.Code != "invalid_provider_credentials" {
		t.Errorf("err = %v, want invalid_provider_credentials", err)
	}
}

func TestAdapterRetriesOn429ThenSucceeds(t *testing.T) {
	attempts := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", authHandler)
	mux.HandleFunc("/accounts", func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{{"id": "acc-1", "name": "Conta", "currencyCode": "BRL"}},
		})
	})
	adapter, _ := newTestAdapter(t, mux)

	page, err := adapter.GetAccounts(context.Background())
	if err != nil {
		t.Fatalf("GetAccounts: %v", err)
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2", attempts)
	}
	if len(page.Accounts) != 1 || page.Accounts[0].ExternalID != "acc-1" {
		t.Errorf("Accounts = %+v", page.Accounts)
	}
}

func TestAdapterExhaustsRetriesOnPersistentRateLimit(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", authHandler)
	mux.HandleFunc("/accounts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	adapter, _ := newTestAdapter(t, mux)

	_, err := adapter.GetAccounts(context.Background())
	provErr, ok := err.(*ProviderError)
	if !ok || provErr.Code != "provider_rate_limited" {
		t.Errorf("err = %v, want provider_rate_limited", err)
	}
}

func TestAdapterIteratesTransactionPagesUntilNoCursor(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", authHandler)
	mux.HandleFunc("/v2/transactions", func(w http.ResponseWriter, r *http.Request) {
		after := r.URL.Query().Get("after")
		if after == "" {
			json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{"id": "tx-1", "accountId": "acc-1", "amount": -1.0}},
				"next":    "https://api.pluggy.ai/v2/transactions?accountId=acc-1&after=page2",
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{{"id": "tx-2", "accountId": "acc-1", "amount": -2.0}},
			"next":    nil,
		})
	})
	adapter, _ := newTestAdapter(t, mux)

	var seen []string
	err := adapter.IterTransactionPages(context.Background(), "acc-1", func(page TransactionsPage) error {
		for _, tx := range page.Transactions {
			seen = append(seen, tx.ExternalID)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("IterTransactionPages: %v", err)
	}
	if len(seen) != 2 || seen[0] != "tx-1" || seen[1] != "tx-2" {
		t.Errorf("seen = %v, want [tx-1 tx-2]", seen)
	}
}

func TestAdapterGetInvestmentsHappyPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", authHandler)
	mux.HandleFunc("/investments", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{{"id": "inv-1", "name": "Fundo XYZ", "balance": 1000.50}},
		})
	})
	adapter, writer := newTestAdapter(t, mux)

	page, err := adapter.GetInvestments(context.Background())
	if err != nil {
		t.Fatalf("GetInvestments: %v", err)
	}
	if len(page.Investments) != 1 || page.Investments[0].ExternalID != "inv-1" {
		t.Errorf("Investments = %+v", page.Investments)
	}
	if len(writer.envelopes) != 1 || writer.envelopes[0].Scope != ScopeInvestments {
		t.Errorf("envelopes = %+v", writer.envelopes)
	}
}

func TestAdapterGetInvestmentTransactionsHappyPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", authHandler)
	mux.HandleFunc("/investments/inv-1/transactions", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{{"id": "invtx-1", "investmentId": "inv-1", "type": "BUY", "amount": 500.00}},
		})
	})
	adapter, writer := newTestAdapter(t, mux)

	page, err := adapter.GetInvestmentTransactions(context.Background(), "inv-1")
	if err != nil {
		t.Fatalf("GetInvestmentTransactions: %v", err)
	}
	if len(page.Transactions) != 1 || page.Transactions[0].ExternalID != "invtx-1" {
		t.Errorf("Transactions = %+v", page.Transactions)
	}
	if len(writer.envelopes) != 1 || writer.envelopes[0].Scope != ScopeInvestmentTransactions {
		t.Errorf("envelopes = %+v", writer.envelopes)
	}
}

func TestAdapterGetBillsPaginatesByPageNumber(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", authHandler)
	mux.HandleFunc("/bills", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("accountId") != "acc-1" {
			t.Errorf("accountId = %q, want acc-1", r.URL.Query().Get("accountId"))
		}
		switch r.URL.Query().Get("page") {
		case "1":
			json.NewEncoder(w).Encode(map[string]any{
				"page": 1, "totalPages": 2,
				"results": []map[string]any{{"id": "bill-1", "dueDate": "2026-03-10T00:00:00Z"}},
			})
		case "2":
			json.NewEncoder(w).Encode(map[string]any{
				"page": 2, "totalPages": 2,
				"results": []map[string]any{{"id": "bill-2", "dueDate": "2026-04-10T00:00:00Z"}},
			})
		default:
			t.Errorf("unexpected page %q", r.URL.Query().Get("page"))
		}
	})
	adapter, writer := newTestAdapter(t, mux)

	page, err := adapter.GetBills(context.Background(), "acc-1")
	if err != nil {
		t.Fatalf("GetBills: %v", err)
	}
	if len(page.Bills) != 2 || page.Bills[0].ExternalID != "bill-1" || page.Bills[1].ExternalID != "bill-2" {
		t.Errorf("Bills = %+v", page.Bills)
	}
	if len(writer.envelopes) != 2 || writer.envelopes[0].Scope != ScopeBills {
		t.Errorf("envelopes = %+v", writer.envelopes)
	}
}

func TestAdapterRejectsRepeatedCursor(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", authHandler)
	mux.HandleFunc("/v2/transactions", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{{"id": "tx-1", "accountId": "acc-1", "amount": -1.0}},
			"next":    "https://api.pluggy.ai/v2/transactions?accountId=acc-1&after=same",
		})
	})
	adapter, _ := newTestAdapter(t, mux)

	err := adapter.IterTransactionPages(context.Background(), "acc-1", func(TransactionsPage) error { return nil })
	provErr, ok := err.(*ProviderError)
	if !ok || provErr.Code != "invalid_provider_payload" {
		t.Errorf("err = %v, want invalid_provider_payload for a repeated cursor", err)
	}
}

// Regression test: the endpoint was read as a single page on the belief that
// it was unpaginated, so a history longer than one page was truncated in
// silence — no error, no rejection, just half a history.
func TestAdapterGetInvestmentTransactionsPaginatesByPageNumber(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", authHandler)
	mux.HandleFunc("/investments/inv-1/transactions", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "1":
			json.NewEncoder(w).Encode(map[string]any{
				"page": 1, "totalPages": 2,
				"results": []map[string]any{{"id": "invtx-1", "type": "BUY", "movementType": "CREDIT", "amount": 100.0}},
			})
		case "2":
			json.NewEncoder(w).Encode(map[string]any{
				"page": 2, "totalPages": 2,
				"results": []map[string]any{{"id": "invtx-2", "type": "SELL", "movementType": "DEBIT", "amount": 120.0}},
			})
		default:
			t.Errorf("unexpected page %q", r.URL.Query().Get("page"))
		}
	})
	adapter, writer := newTestAdapter(t, mux)

	page, err := adapter.GetInvestmentTransactions(context.Background(), "inv-1")
	if err != nil {
		t.Fatalf("GetInvestmentTransactions: %v", err)
	}
	if len(page.Transactions) != 2 || page.Transactions[0].ExternalID != "invtx-1" || page.Transactions[1].ExternalID != "invtx-2" {
		t.Errorf("Transactions = %+v", page.Transactions)
	}
	if len(writer.envelopes) != 2 || writer.envelopes[0].Scope != ScopeInvestmentTransactions {
		t.Errorf("envelopes = %+v", writer.envelopes)
	}
}

// Regression test for the other half of the truncation problem: paginating
// correctly still loses history if the page budget runs out first. Reporting
// a partial history as if it were the whole one is what makes a healthy
// position read as a loss, so the cap has to fail loudly instead.
func TestAdapterGetInvestmentTransactionsRefusesATruncatedHistory(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", authHandler)
	mux.HandleFunc("/investments/inv-1/transactions", func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		// Always one more page than the caller has budget for.
		json.NewEncoder(w).Encode(map[string]any{
			"page": page, "totalPages": 9999,
			"results": []map[string]any{{
				"id": "invtx-" + strconv.Itoa(page), "type": "BUY", "movementType": "CREDIT", "amount": 1.0,
			}},
		})
	})
	adapter, _ := newTestAdapter(t, mux)
	adapter.config.MaxPagesPerAccount = 2

	_, err := adapter.GetInvestmentTransactions(context.Background(), "inv-1")
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr.Code != "provider_pagination_exhausted" {
		t.Fatalf("err = %v, want a provider_pagination_exhausted ProviderError", err)
	}
	if providerErr.ExternalAccountID == nil || *providerErr.ExternalAccountID != "inv-1" {
		t.Errorf("ExternalAccountID = %v, want the investment named so the cap can be raised knowingly", providerErr.ExternalAccountID)
	}
}

// The cursor comes out of a decoded query string and must be re-encoded when
// it is sent back, along with ids interpolated into paths and queries.
func TestAdapterEscapesCursorAndIDsInRequests(t *testing.T) {
	const cursor = "a+b&c=d"
	const accountID = "acc/1 x"
	var seenCursor, seenAccount string
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", authHandler)
	mux.HandleFunc("/v2/transactions", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("after") == "" {
			json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{},
				"next":    "https://api.pluggy.ai/v2/transactions?accountId=" + url.QueryEscape(accountID) + "&after=" + url.QueryEscape(cursor),
			})
			return
		}
		seenCursor, seenAccount = q.Get("after"), q.Get("accountId")
		if len(q) != 2 {
			t.Errorf("query = %v, want only accountId and after", q)
		}
		json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{}, "next": nil})
	})
	var itemPath string
	mux.HandleFunc("/items/", func(w http.ResponseWriter, r *http.Request) {
		itemPath = r.URL.EscapedPath()
		w.WriteHeader(http.StatusNotFound)
	})
	adapter, _ := newTestAdapter(t, mux)
	adapter.config.ItemID = "item/../x?y"

	if err := adapter.IterTransactionPages(context.Background(), accountID, func(TransactionsPage) error { return nil }); err != nil {
		t.Fatalf("IterTransactionPages: %v", err)
	}
	if seenCursor != cursor || seenAccount != accountID {
		t.Errorf("server saw after=%q accountId=%q, want %q / %q", seenCursor, seenAccount, cursor, accountID)
	}
	_, _, _ = adapter.GetSource(context.Background())
	if itemPath != "/items/item%2F..%2Fx%3Fy" {
		t.Errorf("item path = %q, want the id escaped as one segment", itemPath)
	}
}

// A rate-limit wait of up to MaxRateLimitWait must not outlive the run.
func TestAdapterRateLimitWaitStopsWhenContextIsCancelled(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", authHandler)
	mux.HandleFunc("/accounts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	adapter, _ := newTestAdapter(t, mux)
	adapter.sleep = sleepContext
	adapter.config.MaxRateLimitWait = time.Minute

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)
	start := time.Now()
	_, err := adapter.GetAccounts(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("returned after %s, want promptly after cancellation", elapsed)
	}
}

func TestAdapterBodyReadIsBounded(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", authHandler)
	mux.HandleFunc("/accounts", func(w http.ResponseWriter, r *http.Request) {
		chunk := make([]byte, 1<<20)
		for i := 0; i <= maxResponseBytes>>20; i++ {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	})
	adapter, writer := newTestAdapter(t, mux)

	if _, err := adapter.GetAccounts(context.Background()); err == nil {
		t.Fatal("GetAccounts accepted an oversized body")
	}
	for _, e := range writer.envelopes {
		if len(e.Content) > maxResponseBytes {
			t.Errorf("persisted %d bytes, want at most %d", len(e.Content), maxResponseBytes)
		}
	}
}
