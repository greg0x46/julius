package networth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/networth"
	"github.com/greg0x46/julius/internal/payables"
	"github.com/greg0x46/julius/internal/valuation"
)

// TestComputeIsCashPlusInvestments pins that wealth counts only what has
// already been realized: no card, no debt, no receivable.
func TestComputeIsCashPlusInvestments(t *testing.T) {
	f := newFixture(t)
	f.addAccount("", "1000.00")
	f.addAccount("CREDIT", "300.00") // a card is credit, not wealth
	f.addInvestment("500.00")
	ctx := context.Background()
	if _, err := payables.Create(ctx, f.conn, payables.KindDebt, "Empréstimo", mustDecimal(t, "200"), mustDecimal(t, "200")); err != nil {
		t.Fatal(err)
	}
	if _, err := payables.Create(ctx, f.conn, payables.KindReceivable, "A receber", mustDecimal(t, "150"), mustDecimal(t, "150")); err != nil {
		t.Fatal(err)
	}

	b, err := networth.Compute(context.Background(), f.conn)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	assertDecimalEqual(t, "CashBalance", b.CashBalance, "1000")
	assertDecimalEqual(t, "InvestmentBalance", b.InvestmentBalance, "500")
	assertDecimalEqual(t, "Total", b.Total, "1500")
}

// TestComputeValuesForeignAccountsInBRL pins that a USD account is valued at
// the explicit rate, never added unqualified or left out.
func TestComputeValuesForeignAccountsInBRL(t *testing.T) {
	f := newFixture(t)
	f.addAccount("", "2000.00")
	usd := f.addAccount("", "100.00")
	f.setCurrency("financial_accounts", usd, "USD")

	b, err := networth.ComputeValued(context.Background(), f.conn, valuation.New(f.fx(map[string]string{"USD/BRL": "5"}), time.Now()))
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	assertDecimalEqual(t, "CashBalance", b.CashBalance, "2500")
	assertDecimalEqual(t, "Total", b.Total, "2500")
}

func TestMissingRatesNeverDropCashOrInvestments(t *testing.T) {
	for _, kind := range []string{"cash", "investment"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			f.addAccount("", "2000")
			if kind == "cash" {
				f.setCurrency("financial_accounts", f.addAccount("", "100"), "USD")
			} else {
				f.setCurrency("financial_investments", f.addInvestment("100"), "USD")
			}
			_, err := networth.ComputeValued(context.Background(), f.conn, valuation.New(f.fx(nil), time.Now()))
			if !errors.Is(err, valuation.ErrMissingRate) {
				t.Fatalf("missing %s rate: %v", kind, err)
			}
		})
	}
}
