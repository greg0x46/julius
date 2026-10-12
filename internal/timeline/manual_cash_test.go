package timeline_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/timeline"
	"github.com/greg0x46/julius/internal/transactions"
)

func TestBuildSeriesManualAdjustmentsReconstructBalance(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		created := db.FormatTime(date(t, "2026-10-01"))
		for _, account := range []struct{ id, currency, opening string }{{"brl", "BRL", "100"}, {"other", "BRL", "50"}, {"usd", "USD", "1000"}} {
			_, err := conn.Exec(`INSERT INTO financial_accounts(id,origin,name,account_type,currency_code,opening_balance,created_at,updated_at) VALUES(?,'manual',?,'BANK',?,?,?,?)`, account.id, account.id, account.currency, account.opening, created, created)
			if err != nil {
				t.Fatal(err)
			}
		}
		for _, adjustment := range []struct{ id, account, amount, day string }{{"before", "brl", "3", "2026-10-02"}, {"first", "brl", "5", "2026-10-09"}, {"second", "brl", "-2", "2026-10-10"}, {"other", "other", "20", "2026-10-09"}, {"foreign", "usd", "500", "2026-10-09"}} {
			_, err := conn.Exec(`INSERT INTO account_balance_adjustments(id,account_id,amount,description,created_at) VALUES(?,?,?,'Synthetic correction',?)`, adjustment.id, adjustment.account, adjustment.amount, db.FormatTime(date(t, adjustment.day)))
			if err != nil {
				t.Fatal(err)
			}
		}
		for _, tc := range []struct {
			name, to string
			accounts []string
			balances []string
		}{{"scoped", "2026-10-10", []string{"brl"}, []string{"103", "108", "106"}}, {"all", "2026-10-10", nil, []string{"153", "178", "176"}}, {"historical window", "2026-10-08", []string{"brl"}, []string{"103"}}, {"foreign only", "2026-10-10", []string{"usd"}, []string{"0", "0", "0"}}} {
			t.Run(tc.name, func(t *testing.T) {
				series, err := timeline.BuildSeries(ctx, conn, timeline.BuildParams{From: date(t, "2026-10-08"), To: date(t, tc.to), ReferenceDate: date(t, "2026-10-10"), AccountIDs: tc.accounts})
				if err != nil {
					t.Fatal(err)
				}
				for i, expected := range tc.balances {
					if series.Points[i].Balance.String() != expected {
						t.Errorf("balance on %s = %s, want %s", series.Points[i].Date, series.Points[i].Balance, expected)
					}
				}
				if len(series.Entries) != 0 || !timeline.TotalsForPeriod(series).Result.IsZero() {
					t.Fatal("adjustments became reporting entries")
				}
			})
		}
	})
}

func TestBuildSeriesFutureManualMovementAlreadyInAnchor(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		created := db.FormatTime(date(t, "2026-10-01"))
		if _, err := conn.Exec(`INSERT INTO financial_accounts(id,origin,name,account_type,currency_code,opening_balance,created_at,updated_at) VALUES('bank','manual','Bank','BANK','BRL','100',?,?)`, created, created); err != nil {
			t.Fatal(err)
		}
		if _, err := transactions.CreateManual(ctx, conn, transactions.ManualInput{AccountID: "bank", Description: "Future deposit", Amount: decimal.RequireFromString("10"), OccurredAt: date(t, "2026-10-12")}); err != nil {
			t.Fatal(err)
		}
		for _, to := range []string{"2026-10-10", "2026-10-12"} {
			t.Run(to, func(t *testing.T) {
				series, err := timeline.BuildSeries(ctx, conn, timeline.BuildParams{From: date(t, "2026-10-10"), To: date(t, to), ReferenceDate: date(t, "2026-10-10")})
				if err != nil {
					t.Fatal(err)
				}
				if series.StartingBalance.String() != "110" || series.Points[0].Balance.String() != "100" {
					t.Fatalf("anchor = %s, reference point = %s; want 110 and 100", series.StartingBalance, series.Points[0].Balance)
				}
				if to == "2026-10-12" {
					if series.Points[2].Balance.String() != "110" || timeline.TotalsForPeriod(series).Income.String() != "10" {
						t.Fatalf("future balance = %s, income = %s; want 110 and 10", series.Points[2].Balance, timeline.TotalsForPeriod(series).Income)
					}
				}
			})
		}
	})
}
