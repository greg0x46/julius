package investments_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/exchange"
	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/valuation"
)

func TestSummaryCountsLinkedBrokerageCashOnce(t *testing.T) {
	f := newBookFixture(t)
	ctx := context.Background()
	accountID := f.addManualAccount("Custódia")
	f.addFinancialInvestment("2000", "4", "500")
	f.addOperation(accountID, "", "initial_balance", "2026-09-01", "1000", "")
	position, err := investments.CreatePosition(ctx, f.conn, investments.PositionInput{
		AccountID:       accountID,
		Name:            "Ações",
		AssetType:       "Ativo",
		InitialQuantity: bookDec("6"),
		InitialUnitCost: bookDec("100"),
		OccurredOn:      time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreatePosition: %v", err)
	}
	summary := f.summary()
	if summary.CurrencyCode != "BRL" || summary.ManualValue.String() != "600" || summary.SyncedValue.String() != "2000" {
		t.Fatalf("summary = %+v", summary)
	}
	if summary.CashBalance.String() != "1000" || summary.TotalValue.String() != "3600" || !summary.UnrealizedGain.IsZero() {
		t.Fatalf("summary = %+v", summary)
	}

	// Once the custody cash is a synced bank account, the provider balance is
	// the only one that may count.
	bankID := f.addFinancialAccount("900")
	if _, err := investments.UpdateAccount(ctx, f.conn, accountID, investments.AccountInput{
		Name: "Custódia", FinancialAccountID: &bankID,
	}); err != nil {
		t.Fatalf("UpdateAccount: %v", err)
	}
	linked := f.summary()
	if linked.CashBalance.String() != "900" || linked.TotalValue.String() != "3500" {
		t.Fatalf("linked summary = %+v", linked)
	}
	for _, account := range linked.Accounts {
		if account.AccountID != accountID {
			continue
		}
		if account.CashBalance.String() != "900" || account.CurrentValue.String() != "600" {
			t.Fatalf("linked account = %+v", account)
		}
	}
	if _, err := investments.GetPosition(ctx, f.conn, position.ID); err != nil {
		t.Fatalf("linking changed the position: %v", err)
	}
}

func TestSummaryAgreesWithManualNetWorth(t *testing.T) {
	f := newBookFixture(t)
	ctx := context.Background()
	accountID := f.addManualAccount("Custódia")
	f.addFinancialInvestment("2000", "4", "500")
	f.addOperation(accountID, "", "initial_balance", "2026-09-01", "1000", "")
	if _, err := investments.CreatePosition(ctx, f.conn, investments.PositionInput{
		AccountID: accountID, Name: "CDB", AssetType: "Renda fixa", InitialValue: bookDecPtr("300"),
	}); err != nil {
		t.Fatalf("CreatePosition: %v", err)
	}
	manual, err := investments.ManualNetWorth(ctx, f.conn)
	if err != nil {
		t.Fatal(err)
	}
	summary := f.summary()
	if summary.TotalValue.Sub(summary.SyncedValue).String() != manual.String() {
		t.Fatalf("summary %s disagrees with manual net worth %s", summary.TotalValue, manual)
	}
}

type usdProvider struct{ rate string }

func (usdProvider) Name() string { return "stub" }
func (p usdProvider) Rate(_ context.Context, base, quote string, day time.Time) (exchange.ProviderRate, error) {
	if p.rate == "" || base != "USD" || quote != "BRL" {
		return exchange.ProviderRate{}, errors.New("no rate")
	}
	return exchange.ProviderRate{Value: decimal.RequireFromString(p.rate), EffectiveOn: day}, nil
}

func (f *bookFixture) valuedSummary(rate string) (investments.Summary, error) {
	f.t.Helper()
	valued := valuation.New(exchange.New(f.conn, usdProvider{rate}), time.Now())
	return investments.BuildSummaryValued(context.Background(), f.conn, valued)
}

func TestImportedForeignCurrencyIsValuedWithoutChangingTheHolding(t *testing.T) {
	f := newBookFixture(t)
	ctx := context.Background()
	id := f.addFinancialInvestment("2000", "4", "500")
	f.exec(`UPDATE financial_investments SET currency_code='USD' WHERE id=?`, id)

	if _, err := f.valuedSummary(""); !errors.Is(err, valuation.ErrMissingRate) {
		t.Fatalf("a foreign holding without a rate must fail the summary, got %v", err)
	}

	summary, err := f.valuedSummary("5")
	if err != nil {
		t.Fatal(err)
	}
	if summary.CurrencyCode != "BRL" || summary.TotalValue.String() != "10000" || summary.SyncedValue.String() != "10000" || summary.Accounts[0].CurrentValue.String() != "10000" {
		t.Fatalf("converted summary = %+v", summary)
	}
	if rate := summary.Valuation.Rates["USD"]; rate.Value.String() != "5" {
		t.Fatalf("recorded rates: %+v", summary.Valuation)
	}
	position, err := investments.GetPosition(ctx, f.conn, id)
	if err != nil || position.CurrentValue.String() != "2000" || position.CurrencyCode == nil || *position.CurrencyCode != "USD" {
		t.Fatalf("native holding changed: %+v, %v", position, err)
	}
}
