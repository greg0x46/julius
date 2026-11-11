package investments_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/investments"
)

// providerHolding stores a synced share the way the sync would, with a quote
// from day(20), and returns its id.
func providerHolding(t *testing.T, conn *sql.DB, holding investments.SyncedHolding) string {
	t.Helper()
	ctx := context.Background()
	now := db.FormatTime(time.Now())
	sourceID, syncRunID, rawImportID, id := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO data_sources (id, provider, external_item_id, created_at, updated_at) VALUES (?, 'pluggy', 'item-1', ?, ?)`, []any{sourceID, now, now}},
		{`INSERT INTO sync_runs (id, source_id, status, started_at, finished_at) VALUES (?, ?, 'completed', ?, ?)`, []any{syncRunID, sourceID, now, now}},
		{`INSERT INTO raw_imports (id, sync_run_id, source_id, scope, page_sequence, request_attempt, request_method, request_path, http_status, response_headers, payload, payload_sha256, received_at)
			VALUES (?, ?, ?, 'investments', 1, 1, 'GET', '/x', 200, '{}', ?, 'sha', ?)`, []any{rawImportID, syncRunID, sourceID, []byte{0}, now}},
		{`INSERT INTO financial_investments (id, source_id, external_id, name, code, investment_type, currency_code, quantity, balance, current_raw_import_id, normalized_hash, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, 'EQUITY', 'BRL', '7', '280', ?, 'hash', ?, ?)`, []any{id, sourceID, holding.ExternalID, holding.Name, holding.Code, rawImportID, now, now}},
	} {
		if _, err := conn.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("%.40s…: %v", statement.query, err)
		}
	}
	priced := holding
	priced.Quantity, priced.AsOfDate, priced.Amount = decRef("7"), instant("2026-09-20T12:00:00Z"), decRef("280")
	if err := investments.RecordSyncedQuotes(ctx, conn, priced, nil); err != nil {
		t.Fatalf("RecordSyncedQuotes: %v", err)
	}
	return id
}

// The combined movement source, the history it feeds and the backfill's
// held-from day behave the same on SQLite and Postgres.
func TestManualMovementsFeedHistoryAndBackfillOnEveryDialect(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		holding := equity("7")
		id := providerHolding(t, conn, holding)
		asset, err := investments.ResolveSyncedAsset(ctx, conn, holding)
		if err != nil {
			t.Fatal(err)
		}
		if from, err := investments.AssetHeldFrom(ctx, conn, asset.ID); err != nil || from != nil {
			t.Fatalf("held from = %v, %v before any purchase, want nil", from, err)
		}

		purchase, err := investments.CreateManualMovement(ctx, conn, investments.ManualMovementInput{
			InvestmentID: id, Kind: investments.ManualMovementBuy, TradeDate: day(10), Quantity: decRef("7"), Amount: dec("259"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := investments.CreateManualMovement(ctx, conn, investments.ManualMovementInput{
			InvestmentID: id, Kind: investments.ManualMovementInterest, TradeDate: day(3), Amount: dec("2"),
		}); err != nil {
			t.Fatal(err)
		}

		from, err := investments.AssetHeldFrom(ctx, conn, asset.ID)
		if err != nil || from == nil || !from.Equal(day(10)) {
			t.Fatalf("held from = %v, %v, want day 10 (a provento is not a purchase)", from, err)
		}
		values, err := investments.ValuesOn(ctx, conn, []time.Time{day(5), day(12), day(20)})
		if err != nil {
			t.Fatal(err)
		}
		wantBRL(t, "before the purchase", values[0], "0")
		if values[1].Complete {
			t.Error("the days between the purchase and the first price must stay incomplete")
		}
		wantBRL(t, "priced day", values[2], "280")

		updated, err := investments.UpdateManualMovement(ctx, conn, purchase.ID, investments.ManualMovementInput{
			InvestmentID: id, Kind: investments.ManualMovementBuy, TradeDate: day(8), Quantity: decRef("7"), Amount: dec("259"),
		})
		if err != nil || !updated.TradeDate.Equal(day(8)) {
			t.Fatalf("updated = %+v, %v; want day 8", updated, err)
		}
		if from, _ := investments.AssetHeldFrom(ctx, conn, asset.ID); from == nil || !from.Equal(day(8)) {
			t.Fatalf("held from = %v after correcting the date, want day 8", from)
		}
		if err := investments.DeleteManualMovement(ctx, conn, purchase.ID); err != nil {
			t.Fatal(err)
		}
		if from, _ := investments.AssetHeldFrom(ctx, conn, asset.ID); from != nil {
			t.Fatalf("held from = %v after deleting the purchase, want nil", from)
		}
	})
}
