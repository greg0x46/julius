package syncsvc_test

import (
	"context"
	"database/sql"
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

func TestVanishedPendingDoesNotReconcileIgnoredSuccessor(t *testing.T) {
	for _, origin := range []transactions.InclusionOrigin{transactions.InclusionOriginManual, transactions.InclusionOriginRule} {
		t.Run(string(origin), func(t *testing.T) {
			dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
				f := newVanishFixture(t, conn)
				pending := record("pending", "PENDING", "6772", salaryDay)
				posted := record("posted", "POSTED", "6772", salaryDay)
				f.listing(t, pending)
				commitment := f.reconcile(t, "pending")
				f.listing(t, pending, posted)
				id := f.internalID(t, "posted")
				if _, err := transactions.SetInclusion(context.Background(), conn, id, money.Ignored, origin, nil, nil, ledger.OnIgnored); err != nil {
					t.Fatal(err)
				}
				f.listing(t, posted)
				if !f.removedAt(t, "pending") {
					t.Fatal("pending was not removed")
				}
				if linked := f.linkedTo(t, commitment); linked != nil {
					t.Fatalf("ignored successor retained occurrence: %s", *linked)
				}
				var state string
				if err := conn.QueryRow(`SELECT state FROM transaction_inclusion_decisions WHERE transaction_id = ?`, id).Scan(&state); err != nil || state != "ignored" {
					t.Fatalf("successor inclusion = %s, %v", state, err)
				}
			})
		})
	}
}

func TestVanishedPendingPreservesTransferInvestmentExclusivity(t *testing.T) {
	for _, transferOnSuccessor := range []bool{false, true} {
		name := "successor_investment"
		if transferOnSuccessor {
			name = "successor_transfer"
		}
		t.Run(name, func(t *testing.T) {
			dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
				ctx := context.Background()
				f := newVanishFixture(t, conn)
				pending := record("pending", "PENDING", "-6772", salaryDay)
				posted := record("posted", "POSTED", "-6772", salaryDay)
				pending.AmountInAccountCurrency = amountP("-6772")
				posted.AmountInAccountCurrency = amountP("-6772")
				f.listing(t, pending)
				f.listing(t, pending, posted)
				removedID, successorID := f.internalID(t, "pending"), f.internalID(t, "posted")
				transferID, investmentID := removedID, successorID
				if transferOnSuccessor {
					transferID, investmentID = successorID, removedID
				}
				accountID := uuid.NewString()
				now := db.FormatTime(time.Now())
				if _, err := conn.Exec(`INSERT INTO financial_accounts (id, name, account_type, currency_code, origin, opening_balance, created_at, updated_at) VALUES (?, 'Synthetic bank', 'BANK', 'BRL', 'manual', '0', ?, ?)`, accountID, now, now); err != nil {
					t.Fatal(err)
				}
				svc := ledger.Service{DB: conn}
				counterpart, err := svc.CreateManual(ctx, transactions.ManualInput{AccountID: accountID, Description: "Synthetic counterpart", Amount: decimal.RequireFromString("6772"), OccurredAt: salaryDay}, nil)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := svc.LinkTransfer(ctx, transactions.TransferInput{SourceID: transferID, DestinationID: counterpart.ID}); err != nil {
					t.Fatal(err)
				}
				custody, err := investments.CreateAccount(ctx, conn, investments.AccountInput{Name: "Synthetic custody"})
				if err != nil {
					t.Fatal(err)
				}
				operation, err := investments.CreateOperation(ctx, conn, investments.OperationInput{AccountID: custody.ID, Kind: investments.OperationDeposit, OccurredOn: salaryDay, Amount: decimal.RequireFromString("6772")})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := investments.CreateReconciliation(ctx, conn, investments.ReconciliationInput{OperationID: operation.ID, FinancialTransactionID: &investmentID, Amount: decimal.RequireFromString("6772")}); err != nil {
					t.Fatal(err)
				}
				f.listing(t, posted)
				if !f.removedAt(t, "pending") {
					t.Fatal("pending was not removed")
				}
				for _, table := range []struct {
					name, column string
					want         int
				}{
					{"transfer_transaction_links", "transaction_id", 0},
					{"investment_reconciliations", "financial_transaction_id", 1},
				} {
					want := table.want
					if transferOnSuccessor {
						want = 1 - want
					}
					var got int
					if err := conn.QueryRow(`SELECT COUNT(*) FROM `+table.name+` WHERE `+table.column+` = ?`, successorID).Scan(&got); err != nil || got != want {
						t.Fatalf("successor %s = %d, want %d: %v", table.name, got, want, err)
					}
					if err := conn.QueryRow(`SELECT COUNT(*) FROM `+table.name+` WHERE `+table.column+` = ?`, removedID).Scan(&got); err != nil || got != 0 {
						t.Fatalf("removed %s = %d: %v", table.name, got, err)
					}
				}
			})
		})
	}
}
