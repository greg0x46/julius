package payables_test

import (
	"context"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/payables"
	"github.com/greg0x46/julius/internal/projections"
	"github.com/greg0x46/julius/internal/scenarios"
	"github.com/greg0x46/julius/internal/transactions"
)

// Ignoring a transaction frees every event it realized, standalone included;
// otherwise the event stays Realized and drops out of the projection while the
// ignored transaction no longer counts.
func TestIgnoringTransactionFreesStandaloneEvent(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	acc := f.addAccount("BRL")
	now := time.Now()
	txID := f.addTransaction(txSpec{AccountID: acc, Description: "Hotel", Amount: "-100", CurrencyCode: "BRL", MovementType: "DEBIT", ProviderStatus: "POSTED", OccurredAt: &now})
	s, err := scenarios.CreateScenario(ctx, f.conn, scenarios.KindStandalone, "Viagem", nil)
	if err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	st, err := scenarios.CreateScenarioTransaction(ctx, f.conn, s.ID, "Hotel", dec(t, "-100"), day, nil)
	if err != nil {
		t.Fatal(err)
	}
	key := "scenario:" + s.ID + ":transaction:" + st.ID
	if _, err := scenarios.RealizeEvent(ctx, f.conn, s.ID, key, scenarios.RealizationWrite{State: scenarios.RealizationStateLinked, TransactionID: &txID}); err != nil {
		t.Fatalf("RealizeEvent: %v", err)
	}

	if _, err := transactions.SetInclusion(ctx, f.conn, txID, money.Ignored, transactions.InclusionOriginManual, nil, nil, payables.UnlinkIfPresent); err != nil {
		t.Fatalf("SetInclusion: %v", err)
	}

	events, err := projections.List(ctx, f.conn, projections.ProjectionQuery{
		From: day, To: day, Selection: projections.SelectionExplicit, IDs: []string{s.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Realized {
		t.Fatalf("events = %+v, want one unrealized event", events)
	}
}
