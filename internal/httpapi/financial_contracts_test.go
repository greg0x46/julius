package httpapi

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/networth"
	"github.com/greg0x46/julius/internal/timeline"
	"github.com/greg0x46/julius/internal/transactions"
)

var updateFinancialContracts = flag.Bool("update-financial-contracts", false, "rewrite the shared synthetic API fixture")

func financialContractPayloads() map[string]any {
	const id = "11111111-1111-4111-8111-111111111111"
	instant := time.Date(2026, 7, 30, 12, 34, 56, 123456789, time.UTC)
	day := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	text := func(v string) *string { return &v }
	number := func(v int) *int { return &v }
	amount := decimal.RequireFromString("9007199254740993.123456789012345678")
	small := decimal.RequireFromString("0.000000000000000001")
	moneyText := money.CanonicalDecimal(amount)
	brl := "BRL"

	account := accountDTO{
		ID: id, Origin: "synced", ExternalID: text("provider-account"), HasMovements: true,
		SourceDisplayName: text("Synthetic connection"), Institution: text("Synthetic bank"),
		InstitutionName: text("Synthetic bank"), Name: text("Synthetic credit"), Number: text("1234"),
		AccountType: text("CREDIT"), AccountSubtype: text("credit_card"), Balance: &moneyText,
		CreditLimit: text("10000000000000000.00"), AvailableCreditLimit: text("0.00"), CurrencyCode: &brl,
		BalanceCloseDate: &day, BalanceDueDate: &day, ClosingDay: number(30), ClosingDaySource: text(closingDaySourceManual),
		ProviderUpdatedAt: &instant, UpdatedAt: &instant,
	}
	applyCreditUsage(&account)
	accounts := []accountDTO{account, {ID: id, Origin: "manual", OpeningBalance: text("0.00"), ArchivedAt: text(instant.Format(time.RFC3339Nano)), AccountType: text("BANK")}}
	for _, source := range []string{closingDaySourceReported, closingDaySourceEstimated} {
		copy := account
		copy.ClosingDaySource = text(source)
		accounts = append(accounts, copy)
	}

	items := []transactions.Item{}
	for _, status := range []string{"PENDING", "POSTED", "PROCESSING"} {
		effective := &money.EffectiveMoney{Value: amount, CurrencyCode: brl, Source: money.AccountCurrency}
		included, reason := money.Eligibility(money.Outflow, &status, effective, money.Considered, money.Expense)
		item := transactions.Item{
			ID: id, ExternalID: "provider-transaction", Origin: "synced", SourceProvider: text("pluggy"),
			OccurredAt: &instant, Description: text("Synthetic expense"),
			Account:        transactions.AccountSummary{ID: id, Name: text("Synthetic account"), Institution: text("Synthetic bank"), CurrencyCode: &brl},
			SourceCategory: text("Provider category"), MovementType: text("DEBIT"), ProviderStatus: text(status),
			Classification: money.Outflow, Amount: &moneyText, CurrencyCode: &brl, AmountInAccountCurrency: &moneyText,
			EffectiveMoney:           &transactions.EffectiveMoneyView{Value: moneyText, CurrencyCode: brl, Source: money.AccountCurrency},
			InvestmentTransferAmount: "0", ReportableAmount: &moneyText,
			Inclusion:         transactions.Inclusion{State: money.Considered, Origin: "manual"},
			TotalsEligibility: transactions.TotalsEligibility{Included: included, Reason: reason}, GroupKey: "2026-07-30",
			Card:             &transactions.CardInfo{Number: "1234", InstallmentNumber: number(1), TotalInstallments: number(3)},
			InternalCategory: &transactions.InternalCategory{ID: id, Name: "Synthetic category", Kind: money.Expense, IsActive: true, Icon: "coffee", Color: "#123456", Origin: "manual", ChangedAt: instant},
		}
		items = append(items, item)
	}
	reason := money.ReasonUnclassified
	items = append(items, transactions.Item{
		ID: id, ExternalID: "manual:synthetic", Origin: "manual", Account: transactions.AccountSummary{ID: id},
		Classification: money.Unclassified, InvestmentTransferAmount: "0", Inclusion: transactions.Inclusion{State: money.Considered, Origin: "manual"},
		TotalsEligibility: transactions.TotalsEligibility{Reason: &reason}, GroupKey: "undated",
	})
	ignored := items[0]
	ignored.Inclusion = transactions.Inclusion{State: money.Ignored, ChangedAt: &instant, Origin: "rule", RuleName: text("Synthetic rule")}
	ignoredReason := money.ReasonIgnored
	ignored.TotalsEligibility = transactions.TotalsEligibility{Reason: &ignoredReason}
	items = append(items, ignored)
	result := toResultDTO(transactions.Result{
		ConfirmedAt: instant, StoredTotal: len(items), Items: items,
		Page:             transactions.Page{Number: 1, Size: 50, TotalItems: len(items), TotalPages: 1},
		Totals:           []transactions.CurrencyTotals{{CurrencyCode: brl, Inflow: "0", Outflow: moneyText, Balance: "-" + moneyText}},
		AvailableFilters: transactions.AvailableFilters{Institutions: []string{}},
	})

	positions := []investmentPositionDTO{}
	for _, basis := range []investments.ValuationBasis{investments.ValuationBasisCostBasis, investments.ValuationBasisManualValuation, investments.ValuationBasisMarketQuote, investments.ValuationBasisProviderBalance} {
		source := investments.PositionSourceManual
		if basis == investments.ValuationBasisProviderBalance {
			source = investments.PositionSourceSynced
		}
		positions = append(positions, investmentPositionToDTO(investments.Position{
			ID: id, AccountID: id, Source: source, Name: "Synthetic position", AssetType: "EQUITY", CurrencyCode: &brl,
			Quantity: small, AverageCost: &amount, TotalCost: &amount, CurrentValue: amount, CurrentUnitPrice: &amount, ValuedOn: &day, ValuationBasis: basis,
		}))
	}
	positions = append(positions, investmentPositionToDTO(investments.Position{ID: id, AccountID: id, Source: investments.PositionSourceManual, Name: "Empty position", AssetType: "other", ValuationBasis: investments.ValuationBasisCostBasis}))
	operations := []investmentOperationDTO{}
	for _, kind := range []investments.OperationKind{investments.OperationInitialBalance, investments.OperationDeposit, investments.OperationWithdrawal, investments.OperationBuy, investments.OperationSell, investments.OperationIncome, investments.OperationFee, investments.OperationTax, investments.OperationValuation, investments.OperationTransferOut, investments.OperationTransferIn} {
		operations = append(operations, investmentOperationToDTO(investments.Operation{
			ID: id, AccountID: id, Kind: kind, OccurredOn: day, Amount: amount, Quantity: &small, UnitPrice: &amount,
			Fees: small, Taxes: small, Source: "manual", IsEditable: true, CreatedAt: instant, UpdatedAt: instant,
		}))
	}
	imported := operations[0]
	imported.Source, imported.IsEditable, imported.Quantity, imported.UnitPrice = "synced", false, nil, nil
	operations = append(operations, imported)

	point := timeline.DayPoint{Date: day, Balance: amount, Inflow: small, Outflow: amount, LowestTier: timeline.TierRealizado}
	entries := []timeline.Entry{}
	for _, tier := range []timeline.Tier{timeline.TierRealizado, timeline.TierConfirmado, timeline.TierProjetado, timeline.TierHipotetico} {
		for _, source := range []timeline.SourceKind{timeline.SourceReal, timeline.SourceInvestment, timeline.SourceRecurring, timeline.SourcePayablePlan, timeline.SourceScenario} {
			entries = append(entries, timeline.Entry{Date: day, Description: "Synthetic entry", Amount: amount.Neg(), ReportableAmount: &small, Tier: tier, Source: source, SourceRefID: id})
		}
	}
	series := timelineSeriesToDTO(timeline.Series{Points: []timeline.DayPoint{point}, Entries: entries, StartingBalance: amount, LowestBalance: point})
	simulation := series
	simulation.FirstNegative = text("2026-07-30")
	snapshot := netWorthSnapshotDTO{CapturedAt: instant, netWorthBreakdownDTO: toNetWorthBreakdownDTO(networth.Breakdown{CashBalance: amount, InvestmentBalance: small, TotalAssets: amount.Add(small), NetWorth: amount.Add(small)})}

	return map[string]any{
		"accounts":     accounts,
		"cards":        []accountCardDTO{{CardNumber: "1234", TransactionCount: 1, LastTransactionAt: &instant}, {CardNumber: "5678"}},
		"bills":        []accountBillDTO{{ID: id, ExternalID: "provider-bill", DueDate: &day, ClosingDate: &day, TotalAmount: &moneyText, CurrencyCode: &brl, MinimumPaymentAmount: text("0.01")}, {ID: id, ExternalID: "empty-bill"}},
		"transactions": result,
		"investments":  []investmentDTO{{ID: id, ExternalID: "provider-holding", Balance: &moneyText, CurrencyCode: &brl, Quantity: text(money.CanonicalDecimal(small)), DueDate: &day, AsOfDate: &instant, ProviderUpdatedAt: &instant, YieldValue: text("1.00"), YieldSource: text("informado")}, {ID: id, ExternalID: "empty-holding", YieldUnavailableReason: text("sem_historico")}},
		"investment_accounts": map[string]any{"items": []investmentAccountDTO{
			investmentAccountToDTO(investments.Account{ID: id, Name: "Manual custody", Kind: investments.AccountKindManual, CashBalance: amount, Active: true, CreatedAt: instant, UpdatedAt: instant}),
			investmentAccountToDTO(investments.Account{ID: "integrated:" + id, Name: "Integrated custody", Kind: investments.AccountKindIntegrated, SourceID: text(id), CreatedAt: instant, UpdatedAt: instant}),
		}},
		"positions":           map[string]any{"items": positions},
		"operations":          map[string]any{"items": operations},
		"investment_summary":  investmentSummaryToDTO(investments.Summary{CurrencyCode: brl, TotalValue: amount, ManualValue: amount, UnrealizedGain: small}),
		"timeline":            timelineResponseDTO{Base: series, PeriodTotals: periodTotalsToDTO(timeline.PeriodTotals{Income: small, Expense: amount, Result: small.Sub(amount)}), ScenarioImpacts: []scenarioImpactDTO{}},
		"timeline_simulation": timelineResponseDTO{Base: series, PeriodTotals: periodTotalsDTO{Income: "0", Expense: "0", Result: "0"}, Simulation: &simulation, ScenarioImpacts: []scenarioImpactDTO{{ScenarioID: id, ScenarioName: "Synthetic scenario", Delta: moneyText}}},
		"net_worth":           netWorthResponseDTO{Latest: snapshot, Series: []netWorthSnapshotDTO{snapshot}},
		"money":               []string{moneyText, money.CanonicalDecimal(small), "-123.4500", "0.00", "123.4500"},
	}
}

func TestFinancialContracts(t *testing.T) {
	actual, err := json.MarshalIndent(financialContractPayloads(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", "financial-contracts.json")
	if *updateFinancialContracts {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(actual, '\n'), 0644); err != nil {
			t.Fatal(err)
		}
	}
	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var want, got any
	if err := json.Unmarshal(expected, &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(actual, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatal("financial DTOs drifted from testdata/financial-contracts.json; review the API change, update the fixture and run the frontend contract tests")
	}
}

func TestFinancialContractMoneyRoundTrip(t *testing.T) {
	for _, value := range financialContractPayloads()["money"].([]string) {
		t.Run(value, func(t *testing.T) {
			raw, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			var parsed decimal.Decimal
			if err := json.Unmarshal(raw, &parsed); err != nil {
				t.Fatal(err)
			}
			if got := money.CanonicalDecimal(parsed); got != value {
				t.Fatalf("round trip: got %s, want %s", got, value)
			}
		})
	}
}
