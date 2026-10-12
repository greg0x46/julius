package syncsvc_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/greg0x46/julius/internal/db/dbtest"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/pluggy"
	"github.com/greg0x46/julius/internal/syncsvc"
)

func TestNativeCalendarDatesPreserveProviderDay(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		sourceID, runID := newSyncRun(t, conn)
		rawID := uuid.NewString()
		insertRawImport(t, conn, rawID, runID, sourceID)
		observation := time.Date(2026, 10, 11, 2, 0, 0, 0, time.UTC)
		trade := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
		provider := &fakeProvider{
			source: investmentSafeSource(), sourceRawImportID: rawID,
			investmentsPage: pluggy.InvestmentsPage{RawImportID: rawID, Investments: []pluggy.InvestmentSnapshot{{
				ExternalID: "investment", Name: strp("Synthetic CDB"), InvestmentType: strp("FIXED_INCOME"), Subtype: strp("CDB"), CurrencyCode: strp("BRL"),
				IssueDate: &observation, PurchaseDate: &observation, DueDate: &observation, AsOfDate: &observation,
				Balance: amountP("1"), Quantity: amountP("1"), AmountOriginal: amountP("1"), Rate: amountP("100"), RateType: strp("CDI"), IssuerCNPJ: strp("12345678000190"),
			}}},
			investmentTransactions: map[string]pluggy.InvestmentTransactionsPage{"investment": {RawImportID: rawID, Transactions: []pluggy.InvestmentTransactionSnapshot{{ExternalID: "trade", ExternalInvestmentID: "investment", TradeDate: &trade}}}},
		}
		if err := (&syncsvc.Service{DB: conn, Provider: provider, SourceID: sourceID, SyncRunID: runID}).Execute(context.Background()); err != nil {
			t.Fatal(err)
		}
		var issue, purchase, due, asOf string
		if err := conn.QueryRow(`SELECT issue_date,purchase_date,due_date,as_of_date FROM financial_investments WHERE external_id='investment'`).Scan(dbtypes.Date(&issue), dbtypes.Date(&purchase), dbtypes.Date(&due), dbtypes.Time(&asOf)); err != nil {
			t.Fatal(err)
		}
		if issue != "2026-10-10" || purchase != issue || due != issue {
			t.Fatalf("provider calendar days: %s %s %s", issue, purchase, due)
		}
		positions, err := investments.ListPositions(context.Background(), conn, investments.PositionFilter{})
		if err != nil {
			t.Fatal(err)
		}
		if len(positions) != 1 || positions[0].Ticker == nil || *positions[0].Ticker != "CDB-12345678-100CDI-20261010-20261010" {
			t.Fatalf("holding lost its provider day: %+v", positions)
		}

		parsedAsOf, err := dbtypes.ParseTime(asOf)
		if err != nil || !parsedAsOf.Equal(observation) {
			t.Fatalf("observation time changed: %s, %v", asOf, err)
		}
		var storedTrade string
		if err := conn.QueryRow(`SELECT trade_date FROM financial_investment_transactions WHERE external_id='trade'`).Scan(dbtypes.Date(&storedTrade)); err != nil {
			t.Fatal(err)
		}
		if storedTrade != "2026-10-11" {
			t.Fatalf("midnight UTC trade shifted to %s", storedTrade)
		}
	})
}
