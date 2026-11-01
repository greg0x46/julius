package recurrences_test

import (
	"context"
	"database/sql"
	"github.com/google/uuid"
	"github.com/greg0x46/julius/internal/db"
	"reflect"
	"testing"

	"github.com/greg0x46/julius/internal/dates"
	"github.com/greg0x46/julius/internal/db/dbtest"

	"github.com/greg0x46/julius/internal/recurrences"
	"github.com/greg0x46/julius/internal/transactions"
)

func TestDiscoveryReadOnlyBothDialects(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		account, err := transactions.WriteManualAccount(ctx, conn, "", transactions.ManualAccountInput{Name: "Synthetic", CurrencyCode: "BRL"}, true)
		if err != nil {
			t.Fatal(err)
		}
		ids := []string{}
		for month := 0; month < 6; month++ {
			id, err := transactions.CreateManual(ctx, conn, transactions.ManualInput{AccountID: account, Description: "Internet", Amount: dec(t, "-99.1234"), OccurredAt: dates.AddMonths(date(t, "2026-01-05"), month)})
			if err != nil {
				t.Fatal(err)
			}
			ids = append(ids, id)
		}
		// A deleted movement and an ignored duplicate never add evidence.
		deleted, err := transactions.CreateManual(ctx, conn, transactions.ManualInput{AccountID: account, Description: "Internet", Amount: dec(t, "-50"), OccurredAt: date(t, "2026-06-10")})
		if err != nil {
			t.Fatal(err)
		}
		if err = transactions.DeleteManual(ctx, conn, deleted, nil); err != nil {
			t.Fatal(err)
		}
		ignored, err := transactions.CreateManual(ctx, conn, transactions.ManualInput{AccountID: account, Description: "Internet", Amount: dec(t, "-50"), OccurredAt: date(t, "2026-06-11")})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = conn.Exec(`INSERT INTO transaction_inclusion_decisions (transaction_id,state,revision,origin,changed_at) VALUES (?, 'ignored', 1, 'manual', ?)`, ignored, "2026-06-11T12:00:00Z"); err != nil {
			t.Fatal(err)
		}
		before, err := transactions.History(ctx, conn)
		if err != nil {
			t.Fatal(err)
		}
		got, err := recurrences.Discover(ctx, conn, date(t, "2026-07-01"))
		if err != nil || len(got) != 1 {
			t.Fatalf("Discover: %+v, %v", got, err)
		}
		if !reflect.DeepEqual(got[0].TransactionIDs, ids) || !got[0].Draft.Amount.Equal(dec(t, "99.1234")) {
			t.Fatalf("evidence: %+v", got)
		}
		tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			t.Fatal(err)
		}
		again, err := recurrences.Discover(ctx, tx, date(t, "2026-07-01"))
		tx.Rollback()
		if err != nil || !reflect.DeepEqual(got, again) {
			t.Fatalf("repeat: %+v, %v", again, err)
		}
		after, err := transactions.History(ctx, conn)
		if err != nil {
			t.Fatal(err)
		}
		// History has no ORDER BY: compare the ledger by ID.
		index := func(items []transactions.Item) map[string]transactions.Item {
			result := map[string]transactions.Item{}
			for _, i := range items {
				result[i.ID] = i
			}
			return result
		}
		if !reflect.DeepEqual(index(before), index(after)) {
			t.Fatal("ledger changed")
		}
		for _, table := range []string{"scenarios", "scenario_realizations", "transaction_category_decisions", "transaction_inclusion_events"} {
			var n int
			if err := conn.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
				t.Fatal(err)
			}
			if n != 0 {
				t.Fatalf("discovery wrote %s", table)
			}
		}
		category := newCategory(t, conn)
		c := got[0].Draft
		_, err = recurrences.Create(ctx, conn, recurrences.Write{Name: c.Name, Kind: c.Kind, Amount: c.Amount, CategoryID: category.ID, AccountID: c.AccountID, Cadence: c.Cadence, DayOfMonth: c.DayOfMonth, StartDate: c.StartDate, IsActive: true})
		if err != nil {
			t.Fatal(err)
		}
		suppressed, err := recurrences.Discover(ctx, conn, date(t, "2026-07-01"))
		if err != nil || len(suppressed) != 0 {
			t.Fatalf("existing: %+v, %v", suppressed, err)
		}
		// Linked transfers retain cash eligibility but cannot seed recurring drafts.
		other, err := transactions.WriteManualAccount(ctx, conn, "", transactions.ManualAccountInput{Name: "Other", CurrencyCode: "BRL"}, true)
		if err != nil {
			t.Fatal(err)
		}
		for month, id := range ids {
			destination, err := transactions.CreateManual(ctx, conn, transactions.ManualInput{AccountID: other, Description: "Transfer", Amount: dec(t, "99.1234"), OccurredAt: dates.AddMonths(date(t, "2026-01-05"), month)})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = transactions.CreateTransfer(ctx, conn, transactions.TransferInput{SourceID: id, DestinationID: destination}); err != nil {
				t.Fatal(err)
			}
		}
		// Remove the covering scenario, so only ledger eligibility excludes these rows.
		schedules, err := recurrences.List(ctx, conn)
		if err != nil {
			t.Fatal(err)
		}
		if err = recurrences.Delete(ctx, conn, schedules[0].ID); err != nil {
			t.Fatal(err)
		}
		transfers, err := recurrences.Discover(ctx, conn, date(t, "2026-07-01"))
		if err != nil || len(transfers) != 0 {
			t.Fatalf("transfers: %+v, %v", transfers, err)
		}
	})
}

func TestDiscoverySyncedAndCSVHistoryBothDialects(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		exec := func(query string, args ...any) {
			t.Helper()
			if _, err := conn.ExecContext(ctx, query, args...); err != nil {
				t.Fatal(err)
			}
		}
		now := "2026-06-30T12:00:00Z"
		for _, provider := range []string{"pluggy", "file"} {
			source, run, raw, account := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
			exec(`INSERT INTO data_sources (id,provider,external_item_id,created_at,updated_at) VALUES (?,?,?,?,?)`, source, provider, source, now, now)
			exec(`INSERT INTO sync_runs (id,source_id,status,started_at,finished_at) VALUES (?,?,'completed',?,?)`, run, source, now, now)
			exec(`INSERT INTO raw_imports (id,sync_run_id,source_id,scope,page_sequence,request_attempt,request_method,request_path,http_status,response_headers,payload,payload_sha256,received_at) VALUES (?,?,?,'transactions',1,1,'GET','/synthetic',200,'{}',?,'synthetic',?)`, raw, run, source, []byte("{}"), now)
			exec(`INSERT INTO financial_accounts (id,source_id,external_id,name,currency_code,current_raw_import_id,normalized_hash,created_at,updated_at) VALUES (?,?,?,?,'BRL',?,'synthetic',?,?)`, account, source, account, provider, raw, now, now)
			for month := 0; month < 6; month++ {
				id := uuid.NewString()
				exec(`INSERT INTO financial_transactions (id,source_id,account_id,external_id,description,amount,amount_in_account_currency,currency_code,occurred_at,provider_status,movement_type,current_raw_import_id,normalized_hash,created_at,updated_at) VALUES (?,?,?,?,?,'-100','-100','BRL',?,'POSTED','DEBIT',?,'synthetic',?,?)`, id, source, account, id, "Subscription "+provider, db.FormatTime(dates.AddMonths(date(t, "2026-01-05"), month)), raw, now, now)
			}
		}
		suggestions, err := recurrences.Discover(ctx, conn, date(t, "2026-07-01"))
		if err != nil || len(suggestions) != 2 {
			t.Fatalf("synced/CSV history: %+v, %v", suggestions, err)
		}
		for _, s := range suggestions {
			if len(s.TransactionIDs) != 6 || s.Draft.AccountID == nil {
				t.Fatalf("evidence: %+v", s)
			}
		}
	})
}
