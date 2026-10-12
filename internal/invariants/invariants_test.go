package invariants_test

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/automation"
	"github.com/greg0x46/julius/internal/categories"
	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/networth"
	"github.com/greg0x46/julius/internal/payables"
	"github.com/greg0x46/julius/internal/projections"
	"github.com/greg0x46/julius/internal/recurrences"
	"github.com/greg0x46/julius/internal/scenarios"
	"github.com/greg0x46/julius/internal/timeline"
	"github.com/greg0x46/julius/internal/transactions"
)

func reasonOf(item transactions.Item) string {
	if item.TotalsEligibility.Reason == nil {
		return ""
	}
	return string(*item.TotalsEligibility.Reason)
}

// netChange is the balance the series gained across the window: the walk
// from the first day's closing balance to the last.
func netChange(series timeline.Series) decimal.Decimal {
	return series.Points[len(series.Points)-1].Balance.Sub(series.Points[0].Balance)
}

func TestTransfersCreateNoIncomeOrExpense(t *testing.T) {
	f := newFixture(t)
	origin := f.addAccount("1000.00", "")
	destination := f.addAccount("500.00", "")
	salary := f.addTx(origin, "3000.00", day(5), "POSTED")
	f.setCategory(salary, categorySalario)
	groceries := f.addTx(origin, "-200.00", day(6), "POSTED")
	f.setCategory(groceries, categorySupermercado)

	cashBefore, netWorthBefore := f.cash(), f.netWorth()

	sent := f.addTx(origin, "-500.00", day(10), "POSTED")
	f.setCategory(sent, categoryTransferencia)
	received := f.addTx(destination, "500.00", day(10), "POSTED")
	f.setCategory(received, categoryTransferencia)

	inflow, outflow := f.totals()
	assertDec(t, "Query inflow", inflow, "3000")
	assertDec(t, "Query outflow", outflow, "200")
	for _, id := range []string{sent, received} {
		item := f.item(id)
		if item.TotalsEligibility.Included || reasonOf(item) != string(money.ReasonTransferCategory) {
			t.Errorf("transfer leg eligibility = %+v, want excluded as transfer_category", item.TotalsEligibility)
		}
		if !item.TotalsEligibility.MovesCash() {
			t.Errorf("transfer leg %s does not move cash", id)
		}
	}

	// The reported balance is authoritative: the pair is already inside it.
	assertDec(t, "cash after the pair", f.cash(), cashBefore.String())
	assertDec(t, "net worth after the pair", f.netWorth().NetWorth, netWorthBefore.NetWorth.String())

	combined := f.series(timeline.BuildParams{})
	periodTotals := timeline.TotalsForPeriod(combined)
	assertDec(t, "timeline income", periodTotals.Income, "3000")
	assertDec(t, "timeline expense", periodTotals.Expense, "200")
	assertDec(t, "combined net change", netChange(combined), "2800")

	// On the origin alone the counterpart leg is out of scope, so the
	// transfer must still leave the account.
	assertDec(t, "origin net change", netChange(f.series(timeline.BuildParams{AccountIDs: []string{origin}})), "2300")

	boundary := func(t *testing.T, status string, prepare func(f *fixture, id string)) transactions.Item {
		t.Helper()
		f := newFixture(t)
		account := f.addAccount("100.00", "")
		id := f.addTx(account, "-50.00", day(10), status)
		f.setCategory(id, categoryTransferencia)
		if prepare != nil {
			prepare(f, id)
		}
		inflow, outflow := f.totals()
		assertDec(t, "inflow", inflow, "0")
		assertDec(t, "outflow", outflow, "0")
		return f.item(id)
	}
	cases := []struct {
		name      string
		status    string
		prepare   func(f *fixture, id string)
		reason    money.EligibilityReason
		movesCash bool
	}{
		{"ignored wins over transfer", "POSTED", func(f *fixture, id string) { f.setInclusion(id, money.Ignored, nil) }, money.ReasonIgnored, false},
		{"pending transfer is still a transfer", "PENDING", nil, money.ReasonTransferCategory, true},
		{"unsettled transfer moved nothing", "PROCESSING", nil, money.ReasonIneligibleStatus, false},
		{"unclassified transfer moved nothing", "POSTED", func(f *fixture, id string) {
			f.exec(`UPDATE financial_transactions SET movement_type = NULL WHERE id = ?`, id)
		}, money.ReasonUnclassified, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item := boundary(t, tc.status, tc.prepare)
			if reasonOf(item) != string(tc.reason) {
				t.Errorf("reason = %q, want %q", reasonOf(item), tc.reason)
			}
			if item.TotalsEligibility.MovesCash() != tc.movesCash {
				t.Errorf("MovesCash = %v, want %v", item.TotalsEligibility.MovesCash(), tc.movesCash)
			}
			if money.MovedCash(item.TotalsEligibility.Reason) != tc.movesCash {
				t.Errorf("money.MovedCash disagrees with MovesCash for %q", tc.reason)
			}
		})
	}
}

func TestCategorizationDoesNotAlterProviderBalances(t *testing.T) {
	f := newFixture(t)
	account := f.addAccount("1500.00", "")
	f.addAccount("2000.00", "CREDIT")
	payment := f.addTx(account, "-300.00", day(10), "POSTED")
	fresh := f.addTx(account, "-50.00", day(11), "POSTED")

	type balances struct{ cash, starting, netWorth, netWorthCash decimal.Decimal }
	read := func() balances {
		breakdown := f.netWorth()
		return balances{f.cash(), f.series(timeline.BuildParams{}).StartingBalance, breakdown.NetWorth, breakdown.CashBalance}
	}
	base := read()
	assertDec(t, "CashOnHand (credit account excluded)", base.cash, "1500")
	assertDec(t, "StartingBalance", base.starting, "1500")
	assertDec(t, "net worth cash", base.netWorthCash, "1500")

	groceries := "Groceries"
	steps := []struct {
		name  string
		apply func() error
	}{
		{"categorized as transfer", func() error {
			_, err := categories.AssignManual(f.ctx, f.conn, payment, categoryTransferencia)
			return err
		}},
		{"recategorized as expense", func() error {
			_, err := categories.AssignManual(f.ctx, f.conn, payment, categorySupermercado)
			return err
		}},
		{"back to transfer", func() error {
			_, err := categories.AssignManual(f.ctx, f.conn, payment, categoryTransferencia)
			return err
		}},
		{"automatic category", func() error { return categories.ApplyAutomatic(f.ctx, f.conn, fresh, &groceries) }},
		{"rule category over automatic", func() error {
			_, _, err := categories.ApplyRule(f.ctx, f.conn, fresh, categoryTransferencia)
			return err
		}},
		{"ignored", func() error {
			_, err := transactions.SetInclusion(f.ctx, f.conn, payment, money.Ignored, transactions.InclusionOriginManual, nil, nil, payables.UnlinkIfPresent)
			return err
		}},
		{"considered again", func() error {
			_, err := transactions.SetInclusion(f.ctx, f.conn, payment, money.Considered, transactions.InclusionOriginManual, nil, nil, nil)
			return err
		}},
	}
	for _, step := range steps {
		if err := step.apply(); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
		if got := read(); !got.cash.Equal(base.cash) || !got.starting.Equal(base.starting) ||
			!got.netWorth.Equal(base.netWorth) || !got.netWorthCash.Equal(base.netWorthCash) {
			t.Errorf("after %s: balances = %+v, want %+v", step.name, got, base)
		}
	}
}

func TestObligationCannotSettleTwice(t *testing.T) {
	f := newFixture(t)
	account := f.addAccount("1000.00", "")
	debt, err := payables.Create(f.ctx, f.conn, payables.KindDebt, "Empréstimo", mustDec(t, "1000"), mustDec(t, "1000"))
	if err != nil {
		t.Fatalf("payables.Create: %v", err)
	}
	other, err := payables.Create(f.ctx, f.conn, payables.KindDebt, "Outra dívida", mustDec(t, "500"), mustDec(t, "500"))
	if err != nil {
		t.Fatalf("payables.Create: %v", err)
	}
	// The plan is the accounting source: only its settlements count.
	if _, err := scenarios.CreateScenario(f.ctx, f.conn, scenarios.KindDebtPlan, "Plano", &debt.ID); err != nil {
		t.Fatalf("CreateScenario: %v", err)
	}

	link := func(p payables.Payable, transactionID string) payables.CreateLinkStatus {
		t.Helper()
		result, err := payables.CreateLink(f.ctx, f.conn, p.Kind, p.ID, transactionID)
		if err != nil {
			t.Fatalf("CreateLink: %v", err)
		}
		return result.Status
	}
	summary := func(p payables.Payable) payables.Summary {
		t.Helper()
		s, err := payables.Summarize(f.ctx, f.conn, p)
		if err != nil {
			t.Fatalf("Summarize: %v", err)
		}
		return s
	}
	links := func(transactionID string) (int, int) {
		return f.count(`SELECT COUNT(*) FROM payable_transaction_links WHERE transaction_id = ?`, transactionID),
			f.count(`SELECT COUNT(*) FROM scenario_realizations WHERE relation_type = 'settlement' AND transaction_id = ?`, transactionID)
	}

	payment := f.addTx(account, "-300.00", day(10), "POSTED")
	if got := link(debt, payment); got != payables.StatusCreated {
		t.Fatalf("first link = %s, want created", got)
	}
	if got := link(debt, payment); got != payables.StatusConflict {
		t.Errorf("same payable again = %s, want conflict", got)
	}
	if got := link(other, payment); got != payables.StatusConflict {
		t.Errorf("second payable = %s, want conflict", got)
	}
	if l, s := links(payment); l != 1 || s != 1 {
		t.Errorf("links=%d settlements=%d, want 1/1", l, s)
	}
	s := summary(debt)
	assertDec(t, "settled", s.Settled, "300")
	assertDec(t, "remaining", s.Remaining, "700")
	assertDec(t, "other payable settled", summary(other).Settled, "0")

	t.Run("ignoring unlinks and relinking settles once", func(t *testing.T) {
		f.setInclusion(payment, money.Ignored, payables.UnlinkIfPresent)
		if l, s := links(payment); l != 0 || s != 0 {
			t.Errorf("after ignore: links=%d settlements=%d, want 0/0", l, s)
		}
		assertDec(t, "settled after ignore", summary(debt).Settled, "0")
		if got := link(debt, payment); got != payables.StatusIneligible {
			t.Errorf("link while ignored = %s, want ineligible", got)
		}
		f.setInclusion(payment, money.Considered, nil)
		if l, _ := links(payment); l != 0 {
			t.Errorf("considering again restored %d links, want 0", l)
		}
		if got := link(debt, payment); got != payables.StatusCreated {
			t.Fatalf("relink = %s, want created", got)
		}
		if l, s := links(payment); l != 1 || s != 1 {
			t.Errorf("after relink: links=%d settlements=%d, want 1/1", l, s)
		}
		assertDec(t, "settled after relink", summary(debt).Settled, "300")
	})

	t.Run("overpayment clamps remaining at zero", func(t *testing.T) {
		big := f.addTx(account, "-900.00", day(12), "POSTED")
		if got := link(debt, big); got != payables.StatusCreated {
			t.Fatalf("link = %s, want created", got)
		}
		s := summary(debt)
		assertDec(t, "settled", s.Settled, "1200")
		assertDec(t, "remaining", s.Remaining, "0")
		if s.Status != payables.StatusSettled {
			t.Errorf("status = %s, want settled", s.Status)
		}
		remaining, err := payables.RemainingTotal(f.ctx, f.conn, payables.KindDebt)
		if err != nil {
			t.Fatalf("RemainingTotal: %v", err)
		}
		assertDec(t, "debt remaining total (only the other payable)", remaining, "500")
	})

	t.Run("one transaction realizes one complete event", func(t *testing.T) {
		commitment := func(name string) string {
			t.Helper()
			c, err := recurrences.Create(f.ctx, f.conn, recurrences.Write{
				Name: name, Kind: recurrences.KindIncome, Amount: mustDec(t, "200.00"),
				CategoryID: categorySalario, Cadence: recurrences.CadenceMonthly, DayOfMonth: 20,
				StartDate: day(1), IsActive: true,
			})
			if err != nil {
				t.Fatalf("recurrences.Create: %v", err)
			}
			return c.ID
		}
		first, second := commitment("Salário A"), commitment("Salário B")
		income := f.addTx(account, "200.00", day(20), "POSTED")
		realize := func(scenarioID string) error {
			_, err := scenarios.RealizeEvent(f.ctx, f.conn, scenarioID, "scenario:"+scenarioID+":occurrence:2026-08-20",
				scenarios.RealizationWrite{State: scenarios.RealizationStateLinked, TransactionID: &income})
			return err
		}
		if err := realize(first); err != nil {
			t.Fatalf("first realization: %v", err)
		}
		if err := realize(second); !errors.Is(err, scenarios.ErrTransactionAlreadyRealized) {
			t.Errorf("second realization = %v, want ErrTransactionAlreadyRealized", err)
		}
	})
}

func TestInvestmentPrincipalIsNotIncome(t *testing.T) {
	f := newFixture(t)
	bank := f.addAccount("5000.00", "")
	custody, err := investments.CreateAccount(f.ctx, f.conn, investments.AccountInput{Name: "Corretora"})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	operation := func(kind investments.OperationKind, amount string, n int) string {
		t.Helper()
		op, err := investments.CreateOperation(f.ctx, f.conn, investments.OperationInput{
			AccountID: custody.ID, Kind: kind, OccurredOn: day(n), Amount: mustDec(t, amount),
		})
		if err != nil {
			t.Fatalf("CreateOperation(%s): %v", kind, err)
		}
		return op.ID
	}
	reconcile := func(operationID, transactionID, amount string) {
		t.Helper()
		if _, err := investments.CreateReconciliation(f.ctx, f.conn, investments.ReconciliationInput{
			OperationID: operationID, FinancialTransactionID: &transactionID, Amount: mustDec(t, amount),
		}); err != nil {
			t.Fatalf("CreateReconciliation: %v", err)
		}
	}

	fullDeposit := f.addTx(bank, "-1000.00", day(3), "POSTED")
	reconcile(operation(investments.OperationDeposit, "1000", 3), fullDeposit, "1000")
	// The bank line carried 50 more than the deposit: the rest is spending.
	oversizedDeposit := f.addTx(bank, "-1050.00", day(4), "POSTED")
	reconcile(operation(investments.OperationDeposit, "1000", 4), oversizedDeposit, "1000")
	withdrawal := f.addTx(bank, "700.00", day(5), "POSTED")
	reconcile(operation(investments.OperationWithdrawal, "700", 5), withdrawal, "700")
	// Only 600 of this line is explained as principal so far.
	partialLink := f.addTx(bank, "-1000.00", day(6), "POSTED")
	reconcile(operation(investments.OperationDeposit, "1000", 6), partialLink, "600")
	linkedIncome := f.addTx(bank, "20.00", day(7), "POSTED")
	reconcile(operation(investments.OperationIncome, "20", 7), linkedIncome, "20")
	operation(investments.OperationIncome, "30", 8)

	cases := []struct {
		id         string
		reason     string
		reportable string
		effective  string
	}{
		{fullDeposit, string(money.ReasonInvestmentTransfer), "0", "-1000"},
		{oversizedDeposit, "", "50", "-1050"},
		{withdrawal, string(money.ReasonInvestmentTransfer), "0", "700"},
		{partialLink, "", "400", "-1000"},
		{linkedIncome, "", "20", "20"},
	}
	for _, tc := range cases {
		item := f.item(tc.id)
		if reasonOf(item) != tc.reason {
			t.Errorf("%s reason = %q, want %q", tc.effective, reasonOf(item), tc.reason)
		}
		if !item.TotalsEligibility.MovesCash() {
			t.Errorf("%s does not move cash", tc.effective)
		}
		if item.ReportableAmount == nil {
			t.Errorf("%s ReportableAmount = nil, want %s", tc.effective, tc.reportable)
		} else {
			assertDec(t, tc.effective+" ReportableAmount", mustDec(t, *item.ReportableAmount), tc.reportable)
		}
		assertDec(t, tc.effective+" EffectiveMoney", mustDec(t, item.EffectiveMoney.Value), tc.effective)
	}

	inflow, outflow := f.totals()
	assertDec(t, "Query inflow (linked income only)", inflow, "20")
	assertDec(t, "Query outflow (unreconciled remainders)", outflow, "450")
	assertDec(t, "cash", f.cash(), "5000")

	series := f.series(timeline.BuildParams{})
	periodTotals := timeline.TotalsForPeriod(series)
	// 20 from the bank line, 30 from the unlinked income operation; the linked
	// income operation reports nothing of its own, so it counts once.
	assertDec(t, "timeline income", periodTotals.Income, "50")
	assertDec(t, "timeline expense", periodTotals.Expense, "450")
	for _, entry := range series.Entries {
		if entry.Source == timeline.SourceReal && entry.SourceRefID == fullDeposit {
			assertDec(t, "full deposit entry amount", entry.Amount, "-1000")
		}
		if entry.Source == timeline.SourceInvestment && !entry.Amount.IsZero() {
			t.Errorf("investment entry %s moves bank cash: %s", entry.SourceRefID, entry.Amount)
		}
	}
}

func TestHypotheticalMovementsAreNotRealized(t *testing.T) {
	f := newFixture(t)
	account := f.addAccount("1000.00", "")
	f.addTx(account, "3000.00", day(5), "POSTED")

	standalone, err := scenarios.CreateScenario(f.ctx, f.conn, scenarios.KindStandalone, "Viagem", nil)
	if err != nil {
		t.Fatalf("CreateScenario: %v", err)
	}
	if standalone.IsActive {
		t.Fatalf("standalone scenario starts active")
	}
	planned, err := scenarios.CreateScenarioTransaction(f.ctx, f.conn, standalone.ID, "Passagem", mustDec(t, "-800.00"), day(20), nil)
	if err != nil {
		t.Fatalf("CreateScenarioTransaction: %v", err)
	}

	if n := f.count(`SELECT COUNT(*) FROM financial_transactions`); n != 1 {
		t.Errorf("financial_transactions = %d, want 1", n)
	}
	assertDec(t, "cash", f.cash(), "1000")
	inflow, outflow := f.totals()
	assertDec(t, "Query inflow", inflow, "3000")
	assertDec(t, "Query outflow", outflow, "0")

	build := func(ids []string) timeline.Series {
		t.Helper()
		series, err := timeline.BuildSeries(f.ctx, f.conn, timeline.BuildParams{
			From: day(1), To: day(31), ReferenceDate: day(10), ScenarioIDs: ids,
		})
		if err != nil {
			t.Fatalf("BuildSeries: %v", err)
		}
		return series
	}
	scenarioEntries := func(series timeline.Series) []timeline.Entry {
		var out []timeline.Entry
		for _, entry := range series.Entries {
			if entry.ScenarioID != nil && *entry.ScenarioID == standalone.ID {
				out = append(out, entry)
			}
		}
		return out
	}

	if got := scenarioEntries(build(nil)); len(got) != 0 {
		t.Errorf("inactive scenario in the default series: %+v", got)
	}
	explicit := build([]string{standalone.ID})
	got := scenarioEntries(explicit)
	if len(got) != 1 || got[0].Tier != timeline.TierHipotetico || got[0].Source != timeline.SourceScenario {
		t.Fatalf("explicit series scenario entries = %+v, want one hipotetico/scenario entry", got)
	}
	assertDec(t, "hypothetical amount", got[0].Amount, "-800")
	assertDec(t, "StartingBalance with a scenario", explicit.StartingBalance, "1000")

	if _, err := scenarios.SetActive(f.ctx, f.conn, standalone.ID, true); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	if got := scenarioEntries(build(nil)); len(got) != 1 || got[0].Tier != timeline.TierHipotetico {
		t.Errorf("active scenario in the default series = %+v, want one hipotetico entry", got)
	}
	if _, err := scenarios.SetActive(f.ctx, f.conn, standalone.ID, false); err != nil {
		t.Fatalf("SetActive: %v", err)
	}

	listEvents := func(ids []string) []projections.PlannedTransaction {
		t.Helper()
		events, err := projections.List(f.ctx, f.conn, projections.ProjectionQuery{
			From: day(1), To: day(31), Selection: projections.SelectionExplicit, IDs: ids,
		})
		if err != nil {
			t.Fatalf("projections.List: %v", err)
		}
		return events
	}
	if events := listEvents(nil); len(events) != 0 {
		t.Errorf("empty explicit selection emitted %d events", len(events))
	}
	events := listEvents([]string{standalone.ID})
	if len(events) != 1 || events[0].Realized {
		t.Fatalf("events = %+v, want one unrealized event", events)
	}

	payment := f.addTx(account, "-800.00", day(20), "POSTED")
	if _, err := scenarios.RealizeEvent(f.ctx, f.conn, standalone.ID, events[0].EventKey, scenarios.RealizationWrite{
		State: scenarios.RealizationStateLinked, TransactionID: &payment,
	}); err != nil {
		t.Fatalf("RealizeEvent: %v", err)
	}
	if events := listEvents([]string{standalone.ID}); len(events) != 1 || !events[0].Realized {
		t.Errorf("after the link events = %+v, want one realized event", events)
	}
	linked := build([]string{standalone.ID})
	if got := scenarioEntries(linked); len(got) != 0 {
		t.Errorf("realized event still in the series: %+v", got)
	}
	periodTotals := timeline.TotalsForPeriod(linked)
	assertDec(t, "expense after the link (the real payment once)", periodTotals.Expense, "800")
	inflow, outflow = f.totals()
	assertDec(t, "Query inflow after the link", inflow, "3000")
	assertDec(t, "Query outflow after the link", outflow, "800")
	if n := f.count(`SELECT COUNT(*) FROM financial_transactions`); n != 2 {
		t.Errorf("financial_transactions = %d, want 2 (the planned row is never copied)", n)
	}
	if n := f.count(`SELECT COUNT(*) FROM scenario_transactions WHERE id = ?`, planned.ID); n != 1 {
		t.Errorf("planned row count = %d, want 1", n)
	}

	// Divergence 4 in the matrix: allocations are outside the one-complete-
	// event index, so the same 800 payment also realizes a second 800 event.
	second, err := scenarios.CreateScenarioTransaction(f.ctx, f.conn, standalone.ID, "Hotel", mustDec(t, "-800.00"), day(25), nil)
	if err != nil {
		t.Fatalf("CreateScenarioTransaction: %v", err)
	}
	if _, err := scenarios.RealizeEvent(f.ctx, f.conn, standalone.ID, "scenario:"+standalone.ID+":transaction:"+second.ID,
		scenarios.RealizationWrite{State: scenarios.RealizationStateLinked, TransactionID: &payment}); err != nil {
		t.Fatalf("second allocation of the same payment: %v", err)
	}
	if got := scenarioEntries(build([]string{standalone.ID})); len(got) != 0 {
		t.Errorf("scenario entries = %+v, want both events suppressed by one payment", got)
	}
}

func TestRepeatedProcessingDoesNotDuplicateFinancialEffects(t *testing.T) {
	f := newFixture(t)
	account := f.addAccount("1000.00", "")

	t.Run("SetInclusion", func(t *testing.T) {
		id := f.addTx(account, "-120.00", day(10), "POSTED")
		events := func() int {
			return f.count(`SELECT COUNT(*) FROM transaction_inclusion_events WHERE transaction_id = ?`, id)
		}
		if c := f.setInclusion(id, money.Ignored, nil); !c.Changed {
			t.Errorf("first ignore did not change")
		}
		if c := f.setInclusion(id, money.Ignored, nil); c.Changed {
			t.Errorf("second ignore changed again")
		}
		if n := events(); n != 1 {
			t.Errorf("inclusion events = %d, want 1", n)
		}
		f.setInclusion(id, money.Considered, nil)
		f.setInclusion(id, money.Considered, nil)
		if n := events(); n != 2 {
			t.Errorf("inclusion events = %d, want 2", n)
		}
	})

	t.Run("automation", func(t *testing.T) {
		id := f.addTx(account, "-40.00", day(11), "POSTED")
		f.exec(`UPDATE financial_transactions SET description = 'Assinatura streaming' WHERE id = ?`, id)
		category := categorySupermercado
		rule, err := automation.Create(f.ctx, f.conn, automation.Write{
			Name: "Assinaturas", IsActive: true, LogicOperator: automation.LogicAnd,
			Conditions: []automation.Condition{{Field: automation.FieldDescription, Operator: automation.OperatorContains, Value: "assinatura"}},
			Actions: []automation.ActionWrite{
				{Type: automation.ActionSetCategory, CategoryID: &category},
				{Type: automation.ActionIgnore},
			},
		})
		if err != nil {
			t.Fatalf("automation.Create: %v", err)
		}
		state := func() [2]int {
			return [2]int{
				f.count(`SELECT COUNT(*) FROM transaction_inclusion_events WHERE transaction_id = ?`, id),
				f.count(`SELECT COUNT(*) FROM transaction_category_events WHERE transaction_id = ?`, id),
			}
		}
		first, err := automation.ApplyRetroactively(f.ctx, f.conn, rule.ID, payables.UnlinkIfPresent)
		if err != nil {
			t.Fatalf("ApplyRetroactively: %v", err)
		}
		if first.Ignored != 1 || first.Categorized != 1 {
			t.Errorf("first run = %+v, want Ignored=1 Categorized=1", first)
		}
		after := state()
		second, err := automation.ApplyRetroactively(f.ctx, f.conn, rule.ID, payables.UnlinkIfPresent)
		if err != nil {
			t.Fatalf("ApplyRetroactively: %v", err)
		}
		if second.Matched != 1 || second.Ignored != 0 || second.Categorized != 0 {
			t.Errorf("second run = %+v, want Matched=1 and no changes", second)
		}
		for i := 0; i < 2; i++ {
			if err := automation.ApplyToNewTransaction(f.ctx, f.conn, id, payables.UnlinkIfPresent); err != nil {
				t.Fatalf("ApplyToNewTransaction: %v", err)
			}
		}
		if got := state(); got != after {
			t.Errorf("events after reapplying = %v, want %v", got, after)
		}
	})

	t.Run("categories", func(t *testing.T) {
		id := f.addTx(account, "-15.00", day(12), "POSTED")
		events := func() int {
			return f.count(`SELECT COUNT(*) FROM transaction_category_events WHERE transaction_id = ?`, id)
		}
		groceries := "Groceries"
		for i := 0; i < 2; i++ {
			if err := categories.ApplyAutomatic(f.ctx, f.conn, id, &groceries); err != nil {
				t.Fatalf("ApplyAutomatic: %v", err)
			}
		}
		if n := events(); n != 1 {
			t.Errorf("category events after ApplyAutomatic twice = %d, want 1", n)
		}
		for i, wantChanged := range []bool{true, false} {
			_, changed, err := categories.ApplyRule(f.ctx, f.conn, id, categoryTransferencia)
			if err != nil {
				t.Fatalf("ApplyRule: %v", err)
			}
			if changed != wantChanged {
				t.Errorf("ApplyRule run %d changed = %v, want %v", i+1, changed, wantChanged)
			}
		}
		if n := events(); n != 2 {
			t.Errorf("category events after ApplyRule twice = %d, want 2", n)
		}
	})

	t.Run("net worth snapshot", func(t *testing.T) {
		for i := 0; i < 2; i++ {
			if _, err := networth.Snapshot(f.ctx, f.conn); err != nil {
				t.Fatalf("Snapshot: %v", err)
			}
		}
		if n := f.count(`SELECT COUNT(*) FROM net_worth_snapshots WHERE is_backfilled = 0`); n != 1 {
			t.Errorf("live snapshots = %d, want 1", n)
		}
	})

	t.Run("net worth backfill", func(t *testing.T) {
		rows := func() map[string]string {
			out := map[string]string{}
			r, err := f.conn.Query(`SELECT captured_at, net_worth FROM net_worth_snapshots WHERE is_backfilled = 1`)
			if err != nil {
				t.Fatalf("query snapshots: %v", err)
			}
			defer r.Close()
			for r.Next() {
				var capturedAt, value string
				if err := r.Scan(&capturedAt, &value); err != nil {
					t.Fatalf("scan: %v", err)
				}
				out[capturedAt] = value
			}
			return out
		}
		if err := networth.Backfill(f.ctx, f.conn, day(31)); err != nil {
			t.Fatalf("Backfill: %v", err)
		}
		first := rows()
		if len(first) == 0 {
			t.Fatalf("Backfill wrote no rows")
		}
		// A changed balance would reconstruct different numbers; existing
		// rows must keep the ones they were written with.
		f.exec(`UPDATE financial_accounts SET balance = '9999.00' WHERE id = ?`, account)
		if err := networth.Backfill(f.ctx, f.conn, day(31)); err != nil {
			t.Fatalf("Backfill: %v", err)
		}
		second := rows()
		if len(second) != len(first) {
			t.Errorf("backfilled rows = %d, want %d", len(second), len(first))
		}
		for capturedAt, value := range first {
			if second[capturedAt] != value {
				t.Errorf("%s overwritten: %s -> %s", capturedAt, value, second[capturedAt])
			}
		}
		f.exec(`UPDATE financial_accounts SET balance = '1000.00' WHERE id = ?`, account)
	})

	t.Run("investment reconciliation", func(t *testing.T) {
		custody, err := investments.CreateAccount(f.ctx, f.conn, investments.AccountInput{Name: "Corretora"})
		if err != nil {
			t.Fatalf("CreateAccount: %v", err)
		}
		deposit := func() string {
			op, err := investments.CreateOperation(f.ctx, f.conn, investments.OperationInput{
				AccountID: custody.ID, Kind: investments.OperationDeposit, OccurredOn: day(13), Amount: mustDec(t, "100"),
			})
			if err != nil {
				t.Fatalf("CreateOperation: %v", err)
			}
			return op.ID
		}
		line := f.addTx(account, "-100.00", day(13), "POSTED")
		first, second := deposit(), deposit()
		link := func(operationID string) error {
			_, err := investments.CreateReconciliation(f.ctx, f.conn, investments.ReconciliationInput{
				OperationID: operationID, FinancialTransactionID: &line, Amount: mustDec(t, "100"),
			})
			return err
		}
		if err := link(first); err != nil {
			t.Fatalf("first link: %v", err)
		}
		if err := link(first); !errors.Is(err, investments.ErrReconciliationDuplicate) {
			t.Errorf("same pair again = %v, want ErrReconciliationDuplicate", err)
		}
		if err := link(second); !errors.Is(err, investments.ErrReconciliationConflict) {
			t.Errorf("second operation on a fully allocated line = %v, want ErrReconciliationConflict", err)
		}
		if n := f.count(`SELECT COUNT(*) FROM investment_reconciliations WHERE financial_transaction_id = ?`, line); n != 1 {
			t.Errorf("reconciliations = %d, want 1", n)
		}
	})
}

// TestIgnoredRowDivergesBetweenTimelineAndBackfill pins a known divergence
// (see "Divergências" in the matrix): the timeline drops an ignored row from
// its past walk, while the net-worth backfill reverses it out of the anchor.
// Both keep today's reported balance; only past days disagree.
func TestIgnoredRowDivergesBetweenTimelineAndBackfill(t *testing.T) {
	f := newFixture(t)
	account := f.addAccount("1000.00", "")
	f.addTx(account, "10.00", day(2), "POSTED")
	ignored := f.addTx(account, "-300.00", day(10), "POSTED")
	f.setInclusion(ignored, money.Ignored, nil)

	series := f.series(timeline.BuildParams{})
	var timelineDay2 *decimal.Decimal
	for _, point := range series.Points {
		if point.Date.Day() == 2 {
			balance := point.Balance
			timelineDay2 = &balance
		}
	}
	if timelineDay2 == nil {
		t.Fatalf("no timeline point for 2026-08-02")
	}
	assertDec(t, "timeline balance on 08-02 (ignored row dropped)", *timelineDay2, "1000")

	if err := networth.Backfill(f.ctx, f.conn, day(31)); err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	var cashRaw string
	if err := f.conn.QueryRow(`SELECT cash_balance FROM net_worth_snapshots WHERE captured_at = '2026-08-02'`).Scan(&cashRaw); err != nil {
		t.Fatalf("backfilled snapshot: %v", err)
	}
	assertDec(t, "backfilled cash on 08-02 (ignored row reversed)", mustDec(t, cashRaw), "1300")
}

// TestMatrixReferencesExistingTests keeps the matrix honest: every test it
// cites must exist somewhere under internal/.
func TestMatrixReferencesExistingTests(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "..", ".specs", "invariantes-financeiros.md"))
	if err != nil {
		t.Fatalf("read matrix: %v", err)
	}
	cited := regexp.MustCompile("`(Test[A-Za-z0-9_]+)`").FindAllStringSubmatch(string(doc), -1)
	if len(cited) == 0 {
		t.Fatalf("matrix cites no tests")
	}
	var sources strings.Builder
	err = filepath.WalkDir("..", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return err
		}
		content, err := os.ReadFile(path)
		sources.Write(content)
		return err
	})
	if err != nil {
		t.Fatalf("walk internal/: %v", err)
	}
	all := sources.String()
	for _, match := range cited {
		if !strings.Contains(all, "func "+match[1]+"(") {
			t.Errorf("matrix cites %s, which does not exist", match[1])
		}
	}
}
