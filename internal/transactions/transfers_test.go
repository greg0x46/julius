package transactions_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/ledger"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/transactions"
	"github.com/shopspring/decimal"
)

func manualTransferAccount(t *testing.T, conn *sql.DB, currency string) string {
	t.Helper()
	id := uuid.NewString()
	now := db.FormatTime(time.Now())
	_, err := conn.Exec(`INSERT INTO financial_accounts (id, name, account_type, currency_code, origin, opening_balance, created_at, updated_at) VALUES (?, 'Synthetic account', 'BANK', ?, 'manual', '1000', ?, ?)`, id, currency, now, now)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestLinkedTransfersPreserveNativeCashAndReporting(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		source, destination := manualTransferAccount(t, conn, "BRL"), manualTransferAccount(t, conn, "USD")
		svc := ledger.Service{DB: conn}
		at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
		input := func(account, amount string) transactions.ManualInput {
			return transactions.ManualInput{AccountID: account, Description: "Synthetic transfer", Amount: decimal.RequireFromString(amount), OccurredAt: at}
		}
		fee := input(source, "-10")
		linked, err := svc.CreateManualTransfer(ctx, input(source, "-540"), input(destination, "100"), &fee)
		if err != nil {
			t.Fatal(err)
		}
		if linked.Source.Amount == nil || *linked.Source.Amount != "-540" || linked.Destination.Amount == nil || *linked.Destination.Amount != "100" || linked.EffectiveRate != "5.4" || linked.Fee == nil {
			t.Fatalf("transfer: %+v", linked)
		}
		for _, id := range []string{linked.Source.TransactionID, linked.Destination.TransactionID} {
			item, found, err := transactions.GetItem(ctx, conn, id)
			if err != nil || !found || item.TotalsEligibility.Included || !item.TotalsEligibility.MovesCash() || *item.ReportableAmount != "0" {
				t.Fatalf("principal: %+v, %v", item, err)
			}
			if err := transactions.UpdateManual(ctx, conn, id, input(source, "-1")); !errors.Is(err, transactions.ErrTransferLinked) {
				t.Fatalf("linked edit: %v", err)
			}
		}
		sourceBalance, err := transactions.AccountBalance(ctx, conn, source)
		if err != nil || *sourceBalance != "450" {
			t.Fatalf("source cash: %v %v", sourceBalance, err)
		}
		destinationBalance, err := transactions.AccountBalance(ctx, conn, destination)
		if err != nil || *destinationBalance != "1100" {
			t.Fatalf("destination cash: %v %v", destinationBalance, err)
		}
		report, err := transactions.Query(ctx, conn, transactions.QueryRequest{Timezone: "UTC", GroupBy: money.GroupNone, Page: 1, PageSize: 50})
		if err != nil {
			t.Fatal(err)
		}
		for _, total := range report.Totals {
			if total.Inflow != "0" || (total.CurrencyCode == "BRL" && total.Outflow != "10") || (total.CurrencyCode == "USD" && total.Outflow != "0") {
				t.Fatalf("totals: %+v", report.Totals)
			}
		}
		if _, err := svc.LinkTransfer(ctx, transactions.TransferInput{SourceID: linked.Source.TransactionID, DestinationID: linked.Destination.TransactionID}); !errors.Is(err, transactions.ErrTransferLinked) {
			t.Fatalf("duplicate: %v", err)
		}
		if err := transactions.DeleteTransfer(ctx, conn, linked.ID); err != nil {
			t.Fatal(err)
		}
		for _, id := range []string{linked.Source.TransactionID, linked.Destination.TransactionID, linked.Fee.TransactionID} {
			item, _, err := transactions.GetItem(ctx, conn, id)
			if err != nil || !item.TotalsEligibility.Included {
				t.Fatalf("restored: %+v %v", item, err)
			}
		}
		linked, err = svc.LinkTransfer(ctx, transactions.TransferInput{SourceID: linked.Source.TransactionID, DestinationID: linked.Destination.TransactionID, FeeID: linked.Fee.TransactionID})
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.DeleteManual(ctx, linked.Source.TransactionID); err != nil {
			t.Fatal(err)
		}
		if found, err := transactions.GetTransferForTransaction(ctx, conn, linked.Destination.TransactionID); err != nil || found != nil {
			t.Fatalf("delete unlink: %+v %v", found, err)
		}
		item, _, err := transactions.GetItem(ctx, conn, linked.Destination.TransactionID)
		if err != nil || !item.TotalsEligibility.Included {
			t.Fatalf("delete restored destination: %+v %v", item, err)
		}
	})
}

func TestSyncedTransferLinkingAndUnlinkDecisions(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		f := newFixtureWithConn(t, conn)
		source := f.addAccount(account{CurrencyCode: strp("BRL"), AccountType: strp("BANK"), Balance: strp("460")})
		destination := f.addAccount(account{CurrencyCode: strp("USD"), AccountType: strp("BANK"), Balance: strp("100")})
		at := time.Now()
		out := f.addTransaction(txn{AccountID: source, Amount: strp("-540"), AmountInAccountCurrency: strp("-540"), CurrencyCode: strp("BRL"), MovementType: strp("DEBIT"), ProviderStatus: strp("POSTED"), OccurredAt: &at})
		in := f.addTransaction(txn{AccountID: destination, Amount: strp("100"), AmountInAccountCurrency: strp("100"), CurrencyCode: strp("USD"), MovementType: strp("CREDIT"), ProviderStatus: strp("POSTED"), OccurredAt: &at})
		f.setCategory(out, categorySupermercado)
		svc := ledger.Service{DB: conn}
		linked, err := svc.LinkTransfer(ctx, transactions.TransferInput{SourceID: out, DestinationID: in})
		if err != nil {
			t.Fatal(err)
		}
		var count int
		if err := conn.QueryRow(`SELECT COUNT(*) FROM financial_transactions`).Scan(&count); err != nil || count != 2 {
			t.Fatalf("duplicate rows: %d %v", count, err)
		}
		var hash, rawID, amount string
		if err := conn.QueryRow(`SELECT normalized_hash, current_raw_import_id, amount FROM financial_transactions WHERE id = ?`, out).Scan(&hash, &rawID, &amount); err != nil || hash != "hash" || rawID != f.rawImportID || amount != "-540" {
			t.Fatalf("provider changed: %s %s %s %v", hash, rawID, amount, err)
		}
		if err := transactions.DeleteTransfer(ctx, conn, linked.ID); err != nil {
			t.Fatal(err)
		}
		item, _, err := transactions.GetItem(ctx, conn, out)
		if err != nil || !item.TotalsEligibility.Included || item.InternalCategory.ID != categorySupermercado {
			t.Fatalf("category restored: %+v %v", item, err)
		}
		// Mixed manual/synced pairing uses the same transactions, with no provider writes.
		manual, err := svc.CreateManual(ctx, transactions.ManualInput{AccountID: destination, Description: "Synthetic", Amount: decimal.NewFromInt(100), OccurredAt: at}, nil)
		if err != nil {
			t.Fatal(err)
		}
		linked, err = svc.LinkTransfer(ctx, transactions.TransferInput{SourceID: out, DestinationID: manual.ID})
		if err != nil {
			t.Fatal(err)
		}
		_, err = transactions.SetInclusion(ctx, conn, out, money.Ignored, transactions.InclusionOriginManual, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if transfer, err := transactions.GetTransferForTransaction(ctx, conn, manual.ID); err != nil || transfer != nil {
			t.Fatalf("ignore unlink: %+v %v", transfer, err)
		}
	})
}

func TestTransferValidationAndAtomicManualPairs(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		a, b, usd := manualTransferAccount(t, conn, "BRL"), manualTransferAccount(t, conn, "BRL"), manualTransferAccount(t, conn, "USD")
		svc := ledger.Service{DB: conn}
		at := time.Now()
		input := func(account, amount string) transactions.ManualInput {
			return transactions.ManualInput{AccountID: account, Description: "Synthetic", Amount: decimal.RequireFromString(amount), OccurredAt: at}
		}
		for _, tc := range []struct {
			source, dest transactions.ManualInput
			fee          *transactions.ManualInput
		}{
			{input(a, "-10"), input(a, "10"), nil}, {input(a, "10"), input(b, "10"), nil}, {input(a, "-10"), input(b, "9"), nil}, {input(a, "-10"), input(usd, "0"), nil},
		} {
			if _, err := svc.CreateManualTransfer(ctx, tc.source, tc.dest, tc.fee); !errors.Is(err, transactions.ErrInvalidTransfer) {
				t.Fatalf("invalid pair: %v", err)
			}
		}
		var n int
		conn.QueryRow(`SELECT COUNT(*) FROM financial_transactions`).Scan(&n)
		if n != 0 {
			t.Fatalf("failed manual pairs leaked %d rows", n)
		}
		pair, err := svc.CreateManualTransfer(ctx, input(a, "-10"), input(b, "10"), nil)
		if err != nil || pair.EffectiveRate != "1" || pair.Fee != nil {
			t.Fatalf("same currency: %+v %v", pair, err)
		}
		if err := transactions.DeleteTransfer(ctx, conn, pair.ID); err != nil {
			t.Fatal(err)
		}
		for _, in := range []transactions.TransferInput{
			{SourceID: pair.Source.TransactionID, DestinationID: pair.Source.TransactionID},
			{SourceID: pair.Destination.TransactionID, DestinationID: pair.Source.TransactionID},
			{SourceID: pair.Source.TransactionID, DestinationID: pair.Destination.TransactionID, FeeID: pair.Source.TransactionID},
		} {
			if _, err := svc.LinkTransfer(ctx, in); !errors.Is(err, transactions.ErrInvalidTransfer) {
				t.Fatalf("invalid existing pair: %v", err)
			}
		}
		drop := dbtest.FailInserts(t, conn, "transfer_transaction_links", "NEW.role = 'destination'")
		if _, err := svc.CreateManualTransfer(ctx, input(a, "-540"), input(usd, "100"), nil); err == nil {
			t.Fatal("wanted injected failure")
		}
		drop()
		conn.QueryRow(`SELECT COUNT(*) FROM financial_transactions`).Scan(&n)
		if n != 2 {
			t.Fatalf("atomic creation leaked rows: %d", n)
		}
		conn.QueryRow(`SELECT COUNT(*) FROM transfer_links`).Scan(&n)
		if n != 0 {
			t.Fatalf("atomic creation leaked link: %d", n)
		}
	})
}

func TestTransferRejectsInvalidFeesAndKeepsPreviousTransferCategory(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		a, b, other := manualTransferAccount(t, conn, "BRL"), manualTransferAccount(t, conn, "USD"), manualTransferAccount(t, conn, "BRL")
		svc := ledger.Service{DB: conn}
		create := func(account, amount string, category *string) string {
			item, err := svc.CreateManual(ctx, transactions.ManualInput{AccountID: account, Description: "Synthetic", Amount: decimal.RequireFromString(amount), OccurredAt: time.Now()}, category)
			if err != nil {
				t.Fatal(err)
			}
			return item.ID
		}
		out, in := create(a, "-540", strp(categoryTransferencia)), create(b, "100", nil)
		fees := []string{create(a, "10", nil), create(other, "-10", nil), create(a, "-10", strp(categoryTransferencia))}
		ignoredFee := create(a, "-10", nil)
		if _, err := transactions.SetInclusion(ctx, conn, ignoredFee, money.Ignored, transactions.InclusionOriginManual, nil, nil, nil); err != nil {
			t.Fatal(err)
		}
		fees = append(fees, ignoredFee)
		for _, fee := range fees {
			if _, err := svc.LinkTransfer(ctx, transactions.TransferInput{SourceID: out, DestinationID: in, FeeID: fee}); !errors.Is(err, transactions.ErrInvalidTransfer) {
				t.Fatalf("invalid fee: %v", err)
			}
		}
		linked, err := svc.LinkTransfer(ctx, transactions.TransferInput{SourceID: out, DestinationID: in})
		if err != nil {
			t.Fatal(err)
		}
		if linked.Fee != nil {
			t.Fatal("inferred fee from exchange spread")
		}
		if err := transactions.DeleteTransfer(ctx, conn, linked.ID); err != nil {
			t.Fatal(err)
		}
		item, _, err := transactions.GetItem(ctx, conn, out)
		if err != nil || item.TotalsEligibility.Included || *item.TotalsEligibility.Reason != money.ReasonTransferCategory {
			t.Fatalf("previous transfer category: %+v %v", item, err)
		}
	})
}

func TestTransferParticipantsCannotReconcileInvestments(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		svc := ledger.Service{DB: conn}
		source, destination := manualTransferAccount(t, conn, "BRL"), manualTransferAccount(t, conn, "BRL")
		at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
		input := func(account, amount string) transactions.ManualInput {
			return transactions.ManualInput{AccountID: account, Description: "Synthetic transfer", Amount: decimal.RequireFromString(amount), OccurredAt: at}
		}
		fee := input(source, "-10")
		transfer, err := svc.CreateManualTransfer(ctx, input(source, "-540"), input(destination, "540"), &fee)
		if err != nil {
			t.Fatal(err)
		}
		account, err := investments.CreateAccount(ctx, conn, investments.AccountInput{Name: "Synthetic custody"})
		if err != nil {
			t.Fatal(err)
		}
		for _, participant := range []struct {
			name, id string
			kind     investments.OperationKind
			amount   string
		}{
			{"source", transfer.Source.TransactionID, investments.OperationDeposit, "540"},
			{"destination", transfer.Destination.TransactionID, investments.OperationIncome, "540"},
			{"fee", transfer.Fee.TransactionID, investments.OperationDeposit, "10"},
		} {
			t.Run(participant.name, func(t *testing.T) {
				op, err := investments.CreateOperation(ctx, conn, investments.OperationInput{AccountID: account.ID, Kind: participant.kind, OccurredOn: at, Amount: decimal.RequireFromString(participant.amount)})
				if err != nil {
					t.Fatal(err)
				}
				in := investments.ReconciliationInput{OperationID: op.ID, FinancialTransactionID: &participant.id, Amount: decimal.RequireFromString(participant.amount)}
				if _, err := investments.CreateReconciliation(ctx, conn, in); !errors.Is(err, investments.ErrInvalidReconciliationLink) {
					t.Fatalf("linked participant reconciliation: %v", err)
				}
				links, err := investments.ListReconciliations(ctx, conn, investments.ReconciliationFilter{FinancialTransactionID: &participant.id})
				if err != nil || len(links) != 0 {
					t.Fatalf("rejected reconciliation persisted: %+v %v", links, err)
				}
				feeItem, found, err := transactions.GetItem(ctx, conn, transfer.Fee.TransactionID)
				if err != nil || !found || !feeItem.TotalsEligibility.Included || *feeItem.ReportableAmount != "10" {
					t.Fatalf("fee reporting changed: %+v %v", feeItem, err)
				}
				if err := transactions.DeleteTransfer(ctx, conn, transfer.ID); err != nil {
					t.Fatal(err)
				}
				reconciliation, err := investments.CreateReconciliation(ctx, conn, in)
				if err != nil {
					t.Fatalf("unlinked participant reconciliation: %v", err)
				}
				pair := transactions.TransferInput{SourceID: transfer.Source.TransactionID, DestinationID: transfer.Destination.TransactionID, FeeID: transfer.Fee.TransactionID}
				if _, err := svc.LinkTransfer(ctx, pair); !errors.Is(err, transactions.ErrInvestmentLinked) {
					t.Fatalf("investment-linked participant transfer: %v", err)
				}
				if err := investments.DeleteReconciliation(ctx, conn, reconciliation.ID); err != nil {
					t.Fatal(err)
				}
				transfer, err = svc.LinkTransfer(ctx, pair)
				if err != nil {
					t.Fatalf("relink after reconciliation removal: %v", err)
				}
			})
		}
	})
}

func TestTransferWaitsForConcurrentParticipantDeletion(t *testing.T) {
	conn := dbtest.OpenPostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	svc := ledger.Service{DB: conn}
	source, destination := manualTransferAccount(t, conn, "BRL"), manualTransferAccount(t, conn, "BRL")
	at := time.Now()
	input := func(account, amount string) transactions.ManualInput {
		return transactions.ManualInput{AccountID: account, Description: "Synthetic transfer", Amount: decimal.RequireFromString(amount), OccurredAt: at}
	}
	out, err := svc.CreateManual(ctx, input(source, "-540"), nil)
	if err != nil {
		t.Fatal(err)
	}
	in, err := svc.CreateManual(ctx, input(destination, "540"), nil)
	if err != nil {
		t.Fatal(err)
	}
	deleting, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer deleting.Rollback()
	if _, err := deleting.ExecContext(ctx, `UPDATE financial_transactions SET id = id WHERE id = ?`, out.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := svc.LinkTransfer(ctx, transactions.TransferInput{SourceID: out.ID, DestinationID: in.ID})
		done <- err
	}()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	waiting := false
	for !waiting {
		select {
		case err := <-done:
			t.Fatalf("pairing did not wait for the participant lock: %v", err)
		case <-ctx.Done():
			t.Fatal("pairing did not acquire the participant lock")
		case <-ticker.C:
			err := conn.QueryRowContext(ctx, `SELECT EXISTS (
    SELECT 1 FROM pg_stat_activity WHERE datname = current_database()
     AND wait_event_type = 'Lock' AND query LIKE 'UPDATE financial_transactions SET id = id WHERE id = %'
   )`).Scan(&waiting)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := deleting.ExecContext(ctx, `UPDATE financial_transactions SET deleted_at = ? WHERE id = ?`, db.FormatTime(at), out.ID); err != nil {
		t.Fatal(err)
	}
	if err := deleting.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, transactions.ErrTransactionNotFound) {
		t.Fatalf("pairing deleted participant: %v", err)
	}
	var count int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM transfer_links`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("stale transfer persisted: %d %v", count, err)
	}
}
