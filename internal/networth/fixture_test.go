package networth_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/categories"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/exchange"
	"github.com/greg0x46/julius/internal/networth"
	"github.com/greg0x46/julius/internal/transactions"
)

type fixture struct {
	t           *testing.T
	conn        *sql.DB
	sourceID    string
	rawImportID string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	f := &fixture{t: t, conn: conn, sourceID: uuid.NewString(), rawImportID: uuid.NewString()}
	now := db.FormatTime(time.Now())
	f.exec(`INSERT INTO data_sources (id, provider, external_item_id, created_at, updated_at)
		VALUES (?, 'pluggy', 'item-1', ?, ?)`, f.sourceID, now, now)
	syncRunID := uuid.NewString()
	f.exec(`INSERT INTO sync_runs (id, source_id, status, started_at, finished_at)
		VALUES (?, ?, 'completed', ?, ?)`, syncRunID, f.sourceID, now, now)
	f.exec(`INSERT INTO raw_imports (
			id, sync_run_id, source_id, scope, page_sequence, request_attempt,
			request_method, request_path, http_status, response_headers, payload,
			payload_sha256, received_at
		) VALUES (?, ?, ?, 'accounts', 1, 1, 'GET', '/x', 200, '{}', x'00', 'sha', ?)`,
		f.rawImportID, syncRunID, f.sourceID, now)
	return f
}

func (f *fixture) exec(query string, args ...any) {
	f.t.Helper()
	if _, err := f.conn.Exec(query, args...); err != nil {
		f.t.Fatalf("exec %q: %v", query, err)
	}
}

// addAccount inserts a financial_accounts row. accountType "" leaves the
// column NULL, matching a real non-credit account synced without a type.
func (f *fixture) addAccount(accountType, balance string) string {
	f.t.Helper()
	id := uuid.NewString()
	now := db.FormatTime(time.Now())
	var accountTypeArg any
	if accountType != "" {
		accountTypeArg = accountType
	}
	f.exec(`INSERT INTO financial_accounts (
			id, source_id, external_id, account_type, balance, currency_code,
			current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, 'BRL', ?, 'hash', ?, ?)`,
		id, f.sourceID, id, accountTypeArg, balance, f.rawImportID, now, now)
	return id
}

func (f *fixture) addInvestment(balance string) string {
	f.t.Helper()
	id := uuid.NewString()
	now := db.FormatTime(time.Now())
	f.exec(`INSERT INTO financial_investments (
			id, source_id, external_id, balance, currency_code,
			current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, ?, 'BRL', ?, 'hash', ?, ?)`,
		id, f.sourceID, id, balance, f.rawImportID, now, now)
	return id
}

// addTransaction inserts a synced financial_transactions row (POSTED) on
// accountID. movementType is "DEBIT" or "CREDIT".
func (f *fixture) addTransaction(accountID string, occurredAt time.Time, amount, movementType string) string {
	f.t.Helper()
	id := uuid.NewString()
	now := db.FormatTime(time.Now())
	f.exec(`INSERT INTO financial_transactions (
			id, source_id, account_id, external_id, description, amount, amount_in_account_currency,
			currency_code, occurred_at, provider_status, movement_type, current_raw_import_id,
			normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, ?, 'Movimento', ?, ?, 'BRL', ?, 'POSTED', ?, ?, 'hash', ?, ?)`,
		id, f.sourceID, accountID, id, amount, amount, db.FormatTime(occurredAt), movementType, f.rawImportID, now, now)
	return id
}

func (f *fixture) setIgnored(transactionID string) {
	f.t.Helper()
	if _, err := transactions.SetInclusion(context.Background(), f.conn, transactionID, "ignored", transactions.InclusionOriginManual, nil, nil, nil); err != nil {
		f.t.Fatalf("SetInclusion: %v", err)
	}
}

// categoryTransferencia is the seeded "Transferência entre Contas Próprias"
// category, the one kind that gates totals.
const categoryTransferencia = "533d9187-99b6-542b-a2f3-6eb9cbb299ce"

func (f *fixture) setCategory(transactionID, categoryID string) {
	f.t.Helper()
	if _, err := categories.AssignManual(context.Background(), f.conn, transactionID, categoryID); err != nil {
		f.t.Fatalf("AssignManual: %v", err)
	}
}

func mustDecimal(t *testing.T, s string) decimal.Decimal {
	t.Helper()
	d, err := decimal.NewFromString(s)
	if err != nil {
		t.Fatalf("parse decimal %q: %v", s, err)
	}
	return d
}

func assertDecimalEqual(t *testing.T, label string, got decimal.Decimal, want string) {
	t.Helper()
	wantDec := mustDecimal(t, want)
	if !got.Equal(wantDec) {
		t.Errorf("%s = %s, want %s", label, got.String(), wantDec.String())
	}
}

// stubRates is a provider with fixed answers: "USD/BRL" applies to every day,
// "USD/BRL@2024-06-12" only to that one. Anything else is a provider failure.
type stubRates map[string]string

func (stubRates) Name() string { return "stub" }

func (s stubRates) Rate(_ context.Context, base, quote string, day time.Time) (exchange.ProviderRate, error) {
	pair := base + "/" + quote
	value, ok := s[pair+"@"+day.Format("2006-01-02")]
	if !ok {
		value, ok = s[pair]
	}
	if !ok {
		return exchange.ProviderRate{}, errors.New("no rate for " + pair)
	}
	return exchange.ProviderRate{Value: decimal.RequireFromString(value), EffectiveOn: day}, nil
}

func (f *fixture) fx(rates map[string]string) *exchange.Service {
	return exchange.New(f.conn, stubRates(rates))
}

func (f *fixture) setCurrency(table, id, currency string) {
	f.t.Helper()
	f.exec(fmt.Sprintf(`UPDATE %s SET currency_code = ? WHERE id = ?`, table), currency, id)
}

// series runs networth.Series as of today with the given rates.
func (f *fixture) series(today time.Time, rates map[string]string) []networth.Point {
	f.t.Helper()
	points, err := networth.SeriesWith(context.Background(), f.conn, today, f.fx(rates))
	if err != nil {
		f.t.Fatalf("SeriesWith: %v", err)
	}
	return points
}

// pointOn finds the point for a calendar day.
func pointOn(t *testing.T, points []networth.Point, day time.Time) (networth.Point, bool) {
	t.Helper()
	for _, p := range points {
		if p.Day.Format("2006-01-02") == day.Format("2006-01-02") {
			return p, true
		}
	}
	return networth.Point{}, false
}
