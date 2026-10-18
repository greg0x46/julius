package projections_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/scenarios"
)

type standaloneTx struct {
	amount, movement, currency string
	amountInAccount            any
}

func insertStandaloneTx(t *testing.T, conn *sql.DB, spec standaloneTx) string {
	t.Helper()
	now := db.FormatTime(time.Now().UTC())
	sourceID, runID, importID, accountID, txID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := conn.Exec(query, args...); err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
	}
	exec(`INSERT INTO data_sources (id, provider, external_item_id, created_at, updated_at) VALUES (?, 'pluggy', ?, ?, ?)`, sourceID, sourceID, now, now)
	exec(`INSERT INTO sync_runs (id, source_id, status, started_at, finished_at) VALUES (?, ?, 'completed', ?, ?)`, runID, sourceID, now, now)
	exec(`INSERT INTO raw_imports (id, sync_run_id, source_id, scope, page_sequence, request_attempt, request_method, request_path, http_status, response_headers, payload, payload_sha256, received_at) VALUES (?, ?, ?, 'transactions', 1, 1, 'GET', '/x', 200, '{}', x'00', 'hash', ?)`, importID, runID, sourceID, now)
	exec(`INSERT INTO financial_accounts (id, source_id, external_id, currency_code, current_raw_import_id, normalized_hash, created_at, updated_at) VALUES (?, ?, ?, 'BRL', ?, 'hash', ?, ?)`, accountID, sourceID, accountID, importID, now, now)
	exec(`INSERT INTO financial_transactions (id, source_id, account_id, external_id, description, amount, amount_in_account_currency, currency_code, occurred_at, provider_status, movement_type, current_raw_import_id, normalized_hash, created_at, updated_at) VALUES (?, ?, ?, ?, 'Hotel', ?, ?, ?, '2026-10-20T00:00:00.000000000Z', 'POSTED', ?, ?, 'hash', ?, ?)`,
		txID, sourceID, accountID, txID, spec.amount, spec.amountInAccount, spec.currency, spec.movement, importID, now, now)
	return txID
}

func standaloneEvent(t *testing.T, conn *sql.DB, amount string) (scenarios.Scenario, scenarios.ScenarioTransaction, string) {
	t.Helper()
	ctx := context.Background()
	s, err := scenarios.CreateScenario(ctx, conn, scenarios.KindStandalone, "Viagem", nil)
	if err != nil {
		t.Fatal(err)
	}
	st, err := scenarios.CreateScenarioTransaction(ctx, conn, s.ID, "Hotel", projectionDecimal(t, amount), projectionDate(t, "2026-10-20"), nil)
	if err != nil {
		t.Fatal(err)
	}
	return s, st, "scenario:" + s.ID + ":transaction:" + st.ID
}

// A standalone expense is authored negative while its allocation is a
// positive magnitude: fully realized means paga with no deviation.
func TestStandaloneExpenseFullyRealizedIsPaid(t *testing.T) {
	conn := projectionDB(t)
	ctx := context.Background()
	s, _, key := standaloneEvent(t, conn, "-100.00")
	txID := insertStandaloneTx(t, conn, standaloneTx{amount: "-100.00", movement: "DEBIT", currency: "BRL", amountInAccount: "-100.00"})
	if _, err := scenarios.RealizeEvent(ctx, conn, s.ID, key, scenarios.RealizationWrite{State: scenarios.RealizationStateLinked, TransactionID: &txID}); err != nil {
		t.Fatalf("RealizeEvent: %v", err)
	}

	summary, err := scenarios.Summarize(ctx, conn, s.ID, projectionDate(t, "2026-10-25"))
	if err != nil {
		t.Fatal(err)
	}
	if got := summary.Transactions[0].Status; got != scenarios.StatusPaga {
		t.Errorf("status = %s, want paga", got)
	}
	if !summary.AccumulatedDeviation.IsZero() {
		t.Errorf("deviation = %s, want 0", summary.AccumulatedDeviation)
	}
}

func TestStandaloneExpenseStatusUsesMagnitude(t *testing.T) {
	st := scenarios.ScenarioTransaction{Amount: projectionDecimal(t, "-100"), ProjectedAt: projectionDate(t, "2026-10-20")}
	today := projectionDate(t, "2026-10-25")
	for realized, want := range map[string]scenarios.Status{
		"60": scenarios.StatusPagaParcialmente, "100": scenarios.StatusPaga, "130": scenarios.StatusPagaAMais,
	} {
		if got := st.Status(today, projectionDecimal(t, realized)); got != want {
			t.Errorf("Status(%s) = %s, want %s", realized, got, want)
		}
	}
}
