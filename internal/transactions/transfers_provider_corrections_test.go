package transactions_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/ledger"
	"github.com/greg0x46/julius/internal/transactions"
)

func TestTransferProviderCorrectionsKeepDetailAndUnlinkAvailable(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		f := newFixtureWithConn(t, conn)
		source := f.addAccount(account{CurrencyCode: strp("BRL"), AccountType: strp("BANK")})
		destination := f.addAccount(account{CurrencyCode: strp("USD"), AccountType: strp("BANK")})
		at := time.Now()
		out := f.addTransaction(txn{AccountID: source, Amount: strp("-540"), AmountInAccountCurrency: strp("-540"), CurrencyCode: strp("BRL"), MovementType: strp("DEBIT"), ProviderStatus: strp("POSTED"), OccurredAt: &at})
		in := f.addTransaction(txn{AccountID: destination, Amount: strp("100"), AmountInAccountCurrency: strp("100"), CurrencyCode: strp("USD"), MovementType: strp("CREDIT"), ProviderStatus: strp("POSTED"), OccurredAt: &at})
		fee := f.addTransaction(txn{AccountID: source, Amount: strp("-10"), AmountInAccountCurrency: strp("-10"), CurrencyCode: strp("BRL"), MovementType: strp("DEBIT"), ProviderStatus: strp("POSTED"), OccurredAt: &at})
		svc := ledger.Service{DB: conn}
		linked, err := svc.LinkTransfer(ctx, transactions.TransferInput{SourceID: out, DestinationID: in, FeeID: fee})
		if err != nil {
			t.Fatal(err)
		}
		for _, participant := range []struct{ role, id, amount string }{{"source", out, "-540"}, {"destination", in, "100"}, {"fee", fee, "-10"}} {
			t.Run(participant.role, func(t *testing.T) {
				if _, err := conn.Exec(`UPDATE financial_transactions SET amount_in_account_currency = NULL WHERE id = ?`, participant.id); err != nil {
					t.Fatal(err)
				}
				detail, err := transactions.GetTransferForTransaction(ctx, conn, participant.id)
				if err != nil || detail == nil {
					t.Fatalf("corrected detail: %+v %v", detail, err)
				}
				wantRate := ""
				if participant.role == "fee" {
					wantRate = "5.4"
				}
				if detail.EffectiveRate != wantRate {
					t.Fatalf("rate: %q, want %q", detail.EffectiveRate, wantRate)
				}
				data, err := json.Marshal(detail)
				if err != nil {
					t.Fatal(err)
				}
				var body map[string]json.RawMessage
				if err := json.Unmarshal(data, &body); err != nil {
					t.Fatal(err)
				}
				var movement map[string]json.RawMessage
				if err := json.Unmarshal(body[participant.role], &movement); err != nil {
					t.Fatal(err)
				}
				if string(movement["amount"]) != "null" {
					t.Fatalf("missing native amount: %s", data)
				}
				var native sql.NullString
				var original string
				if err := conn.QueryRow(`SELECT amount_in_account_currency, amount FROM financial_transactions WHERE id = ?`, participant.id).Scan(&native, &original); err != nil || native.Valid || original != participant.amount {
					t.Fatalf("provider values rewritten: %v %q %v", native, original, err)
				}
				if _, err := conn.Exec(`UPDATE financial_transactions SET amount_in_account_currency = ? WHERE id = ?`, participant.amount, participant.id); err != nil {
					t.Fatal(err)
				}
			})
		}
		if _, err := conn.Exec(`UPDATE financial_transactions SET amount_in_account_currency = NULL WHERE id = ?`, in); err != nil {
			t.Fatal(err)
		}
		if err := transactions.DeleteTransfer(ctx, conn, linked.ID); err != nil {
			t.Fatal(err)
		}
		if detail, err := transactions.GetTransferForTransaction(ctx, conn, out); err != nil || detail != nil {
			t.Fatalf("unlink: %+v %v", detail, err)
		}
		var count int
		if err := conn.QueryRow(`SELECT COUNT(*) FROM financial_transactions`).Scan(&count); err != nil || count != 3 {
			t.Fatalf("unlink removed provider rows: %d %v", count, err)
		}
	})
}
