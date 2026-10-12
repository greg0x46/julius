package ledger_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/ledger"
	"github.com/greg0x46/julius/internal/transactions"
)

func TestManualCreateIdempotencyAcrossBackends(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		now := db.FormatTime(time.Now())
		source, run, raw, account := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
		for _, statement := range []struct {
			query string
			args  []any
		}{
			{`INSERT INTO data_sources(id,provider,external_item_id,created_at,updated_at) VALUES(?, 'pluggy', ?, ?, ?)`, []any{source, source, now, now}},
			{`INSERT INTO sync_runs(id,source_id,status,started_at,finished_at) VALUES(?, ?, 'completed', ?, ?)`, []any{run, source, now, now}},
			{`INSERT INTO raw_imports(id,sync_run_id,source_id,scope,page_sequence,request_attempt,request_method,request_path,http_status,response_headers,payload,payload_sha256,received_at) VALUES(?, ?, ?, 'transactions', 1, 1, 'GET', '/x', 200, '{}', ?, 'sha', ?)`, []any{raw, run, source, []byte{}, now}},
			{`INSERT INTO financial_accounts(id,source_id,external_id,currency_code,current_raw_import_id,normalized_hash,created_at,updated_at) VALUES(?, ?, ?, 'BRL', ?, 'hash', ?, ?)`, []any{account, source, account, raw, now, now}},
		} {
			if _, err := conn.ExecContext(ctx, statement.query, statement.args...); err != nil {
				t.Fatal(err)
			}
		}
		svc := ledger.Service{DB: conn}
		input := transactions.ManualInput{AccountID: account, Description: "Táxi", Amount: decimal.RequireFromString("-23.50"), OccurredAt: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)}
		key := uuid.NewString()
		first, err := svc.CreateManualIdempotent(ctx, input, nil, key, 1)
		if err != nil {
			t.Fatal(err)
		}
		input.Amount = decimal.RequireFromString("-23.500")
		retry, err := svc.CreateManualIdempotent(ctx, input, nil, key, 1)
		if err != nil {
			t.Fatal(err)
		}
		if retry.ID != first.ID {
			t.Fatalf("retry ID = %s, want %s", retry.ID, first.ID)
		}
		input.Description = "Outro táxi"
		if _, err := svc.CreateManualIdempotent(ctx, input, nil, key, 1); !errors.Is(err, ledger.ErrIdempotencyConflict) {
			t.Fatalf("different request error = %v", err)
		}
		secondOwner, err := svc.CreateManualIdempotent(ctx, input, nil, key, 2)
		if err != nil {
			t.Fatal(err)
		}
		if secondOwner.ID == first.ID {
			t.Fatal("key reused across owners returned first transaction")
		}
		var count int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM financial_transactions WHERE origin = 'manual'`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 2 {
			t.Fatalf("manual rows = %d, want 2", count)
		}
		failedKey := uuid.NewString()
		missingCategory := uuid.NewString()
		if _, err := svc.CreateManualIdempotent(ctx, input, &missingCategory, failedKey, 1); err == nil {
			t.Fatal("invalid category unexpectedly accepted")
		}
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM manual_transaction_idempotency WHERE idempotency_key = ?`, failedKey).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatal("failed creation consumed idempotency key")
		}
		if _, err := svc.CreateManualIdempotent(ctx, input, nil, failedKey, 1); err != nil {
			t.Fatalf("retry after rollback: %v", err)
		}

		concurrentKey := uuid.NewString()
		var wg sync.WaitGroup
		ids := make([]string, 2)
		errs := make([]error, 2)
		for i := range ids {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				item, err := svc.CreateManualIdempotent(ctx, input, nil, concurrentKey, 1)
				ids[i], errs[i] = item.ID, err
			}(i)
		}
		wg.Wait()
		for _, err := range errs {
			if err != nil {
				t.Fatalf("concurrent retry: %v", err)
			}
		}
		if ids[0] == "" || ids[0] != ids[1] {
			t.Fatalf("concurrent IDs = %v", ids)
		}
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM financial_transactions WHERE id = ?`, ids[0]).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("concurrent transactions = %d, want one", count)
		}
	})
}
