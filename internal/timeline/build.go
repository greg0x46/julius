package timeline

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/dates"
	"github.com/greg0x46/julius/internal/exchange"
	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/projections"
	"github.com/greg0x46/julius/internal/reporting"
	"github.com/greg0x46/julius/internal/transactions"
)

// Querier is satisfied by both *sql.DB and *sql.Tx — identical in shape to
// transactions.Querier, reused as an alias so BuildSeries can pass q
// straight through to transactions.Query without a type assertion.
type Querier = transactions.Querier

// BuildParams scopes BuildSeries — dates are calendar days (time-of-day
// ignored), AccountIDs/CategoryIDs/CardNumbers empty means "no filter".
// ScenarioIDs selects an explicit simulation set when non-empty. When empty,
// the unified projector selects active scenarios, so inactive standalone
// scenarios remain out of the default Timeline without special-case logic.
type BuildParams struct {
	From, To      time.Time
	ReferenceDate time.Time
	Timezone      string
	AccountIDs    []string
	CategoryIDs   []string
	CardNumbers   []string
	ScenarioIDs   []string
}

func contains(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}

// eligibleRealItems loads real transactions in [from, to], applying the
// accountIDs/categoryIDs/cardNumbers filters that transactions.Filters
// can't express directly (it only takes one AccountID/CategoryID, not a
// slice). Projected events are resolved by internal/projections; this helper
// intentionally knows only about the financial transaction side.
//
// The filter is MovesCash, not Included: this series is a balance over time,
// not an income/expense report. A transfer between the user's own accounts is
// deliberately outside the totals, but the money really did leave the origin
// account — and buildPoints anchors the curve by subtracting past entries
// from today's real balance, so dropping one leaves every day before it short
// by the transfer's full amount. On an account-filtered series, where the
// counterpart leg is not in the set at all, that error never cancels out.
//
// A transaction the user marked "ignored" is the one place where the walk and
// the anchor genuinely disagree, and MovesCash's answer (drop it) is the one
// this package takes: "ignored" marks a row that should not be in the ledger
// — a duplicate, a reversal — and such a row is not a distinct movement the
// bank's reported balance counted either, so dropping it keeps the two
// definitions aligned in the common case. The cost is the uncommon one: an
// ignored row that *did* move money (transactions.CashOnHand's example of a
// transfer to an untracked account) leaves every past day before it short by
// its amount. The anchor is unaffected — today's point is the reported
// balance whatever the ledger says — so the error is confined to the shape of
// the past, never to the balance the user is shown for today.
func eligibleRealItems(ctx context.Context, q Querier, from, to time.Time, accountIDs, categoryIDs, cardNumbers []string, timezone string) ([]transactions.Item, error) {
	fromDate := money.Date{Year: from.Year(), Month: from.Month(), Day: from.Day()}
	toDate := money.Date{Year: to.Year(), Month: to.Month(), Day: to.Day()}

	result, err := transactions.Query(ctx, q, transactions.QueryRequest{
		Timezone: timezone,
		GroupBy:  money.GroupNone,
		Page:     1,
		PageSize: 1_000_000,
		Filters: transactions.Filters{
			DateFrom: &fromDate,
			DateTo:   &toDate,
		},
	})
	if err != nil {
		return nil, err
	}

	items := make([]transactions.Item, 0, len(result.Items))
	for _, item := range result.Items {
		if !item.TotalsEligibility.MovesCash() || item.EffectiveMoney == nil || item.EffectiveMoney.CurrencyCode != "BRL" || item.OccurredAt == nil {
			continue
		}
		if len(accountIDs) > 0 && !contains(accountIDs, item.Account.ID) {
			continue
		}
		if len(categoryIDs) > 0 {
			if item.InternalCategory == nil || !contains(categoryIDs, item.InternalCategory.ID) {
				continue
			}
		}
		if len(cardNumbers) > 0 {
			if item.Card == nil || !contains(cardNumbers, item.Card.Number) {
				continue
			}
		}
		items = append(items, item)
	}
	return items, nil
}

const noCategoryName = "Sem categoria"

// realEntries projects eligible real transactions into TierRealizado/
// SourceReal entries. Credit card transactions are the exception: the
// purchase already happened, but the cash only leaves the paying account
// when the bill is due, so their entry is dated (and ranked) as
// TierConfirmado at the projected due date instead of occurred_at — see
// transactions.CardDueDates.ProjectedEntryDate. A card-payment leg (see
// categories.CardPaymentCategoryID) is dated the same way but by the bill
// it settles rather than the one still accruing, so it lands on the same
// day as the purchases it cancels out instead of a full cycle later.
//
// The second return value classifies the card entries by when their bill
// falls due relative to reference; see billTiming.
func realEntries(ctx context.Context, q Querier, items []transactions.Item, reference time.Time, loc *time.Location) ([]Entry, billTiming, error) {
	creditAccounts, err := transactions.CreditAccountIDs(ctx, q)
	if err != nil {
		return nil, billTiming{}, err
	}
	transactionIDs := make([]string, 0, len(items))
	for _, item := range items {
		if creditAccounts[item.Account.ID] {
			transactionIDs = append(transactionIDs, item.ID)
		}
	}
	cardSignals, err := transactions.CardTransactionSignals(ctx, q, transactionIDs)
	if err != nil {
		return nil, billTiming{}, err
	}
	var dueDates transactions.CardDueDates
	if len(transactionIDs) > 0 {
		dueDates, err = transactions.FetchCardDueDates(ctx, q)
		if err != nil {
			return nil, billTiming{}, err
		}
	}

	entries := make([]Entry, 0, len(items))
	rates := exchange.New(q, nil)
	bills := billTiming{settled: map[string]bool{}, dueOnReference: map[string]bool{}}
	for _, item := range items {
		amount, err := decimal.NewFromString(item.EffectiveMoney.Value)
		if err != nil {
			return nil, billTiming{}, fmt.Errorf("parse transaction amount %q: %w", item.EffectiveMoney.Value, err)
		}
		cash := item.Reporting
		if cash.Status != "unavailable" {
			reportingDate := item.ReportingDate
			if reportingDate != nil {
				local := reportingDate.In(loc)
				reportingDate = &local
			}
			cash = reporting.Convert(ctx, rates, amount, item.EffectiveMoney.CurrencyCode, reportingDate)
		}
		amount = decimal.Zero
		if cash.Amount != nil {
			amount = *cash.Amount
		}
		// Classification, not the raw value's sign, decides direction —
		// mirrors currencyTotalsFor's Value.Abs() in transactions/query.go.
		amount = amount.Abs()
		if item.Classification == money.Outflow {
			amount = amount.Neg()
		}
		var reportable *decimal.Decimal
		if item.ReportableAmount != nil {
			value, err := decimal.NewFromString(*item.ReportableAmount)
			if err != nil {
				return nil, billTiming{}, err
			}
			if item.TotalsEligibility.Included {
				value = decimal.Zero
				if item.Reporting.Amount != nil {
					value = *item.Reporting.Amount
				}
			}
			if item.Classification == money.Outflow {
				value = value.Neg()
			}
			reportable = &value
		}
		var categoryID *string
		categoryName := noCategoryName
		if item.InternalCategory != nil {
			categoryID = &item.InternalCategory.ID
			categoryName = item.InternalCategory.Name
		}
		description := ""
		if item.Description != nil {
			description = *item.Description
		}
		date := dates.Day(item.OccurredAt.In(loc))
		tier := TierRealizado
		eventKey := "transaction:" + item.ID
		if creditAccounts[item.Account.ID] {
			signal := cardSignals[item.ID]
			date = dates.Day(dueDates.ProjectedEntryDate(item.Account.ID, *item.OccurredAt, signal.Metadata, signal.IsPaymentLeg))
			tier = TierConfirmado
			if date.Before(reference) {
				bills.settled[eventKey] = true
			} else if date.Equal(reference) {
				bills.dueOnReference[eventKey] = true
			}
		}
		originalReportable := decimal.Zero
		if item.ReportableAmount != nil {
			originalReportable, _ = decimal.NewFromString(*item.ReportableAmount)
		}
		if item.Classification == money.Outflow {
			originalReportable = originalReportable.Neg()
		}
		entries = append(entries, Entry{
			OriginalCurrency:         item.EffectiveMoney.CurrencyCode,
			OriginalReportableAmount: &originalReportable,
			ReportingStatus:          cash.Status,
			Date:                     date,
			Description:              description,
			Amount:                   amount,
			ReportableAmount:         reportable,
			CategoryID:               categoryID,
			CategoryName:             categoryName,
			Tier:                     tier,
			Source:                   SourceReal,
			SourceRefID:              item.ID,
			EventKey:                 eventKey,
		})
	}
	return entries, bills, nil
}

// billTiming holds the EventKeys of card entries by when their bill falls due.
// settled: before reference — the past half of the series, where the re-dating
// is a substitute for a payment that is no longer hypothetical; buildPoints
// must not walk those (see cashEntries). dueOnReference: on reference itself —
// whether it was paid yet is unknown here, so they are walked like any future
// bill (a payment already made shows as a real bank leg in cash plus a card
// leg that cancels the purchases) but anchored as not-yet-in-cash; see
// awaitingCash.
type billTiming struct {
	settled, dueOnReference map[string]bool
}

// BuildSeries merges real transactions and the unified Scenario projection
// stream into one Series. Scenario kinds, schedules, automation, links, and
// realization precedence all stay behind projections.List.
func BuildSeries(ctx context.Context, q Querier, params BuildParams) (Series, error) {
	timezone := params.Timezone
	if timezone == "" {
		timezone = "UTC"
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return Series{}, fmt.Errorf("invalid timeline timezone: %w", err)
	}
	from, to, reference := dates.Day(params.From), dates.Day(params.To), dates.Day(params.ReferenceDate)

	// Convert the cash anchor at the reference date, independently of period income/expense.
	balance, balanceReporting, err := reportingCashBalance(ctx, q, params.AccountIDs, reference)
	if err != nil {
		return Series{}, err
	}

	// The fetch reaches back before from because a credit-card purchase is
	// re-dated to its bill's due date (see realEntries): a purchase made in
	// the cycle or two before the window is paid inside it, and querying
	// only [from, to] would leave that bill — real money about to leave the
	// account — out of the projection. Entries whose projected date still
	// falls outside the window are dropped by withinWindow below.
	//
	// Only the window's future half needs the reach-back: a bill that already
	// fell due is paid by a real cash transaction inside [from, to], and its
	// re-dated purchases are dropped from the walk by cashEntries either way.
	// The two months stay because they cost one wider query and nothing else,
	// and because the entries they bring in still belong in Series.Entries.
	items, err := eligibleRealItems(ctx, q, from.AddDate(0, -2, 0), to, params.AccountIDs, params.CategoryIDs, params.CardNumbers, timezone)
	if err != nil {
		return Series{}, err
	}
	entries, bills, err := realEntries(ctx, q, items, reference, loc)
	if err != nil {
		return Series{}, err
	}
	// Investment-account income/costs belong in period totals but do not
	// change the bank-cash anchor. Linked portions are already represented by
	// their bank entries, and are removed by the investment reporting helper.
	if len(params.AccountIDs) == 0 && len(params.CategoryIDs) == 0 && len(params.CardNumbers) == 0 {
		manual, err := investments.ManualReportingEntries(ctx, q)
		if err != nil {
			return Series{}, err
		}
		for _, movement := range manual {
			reportable := movement.Amount
			entry := Entry{Date: movement.OccurredOn, Amount: decimal.Zero,
				ReportableAmount: &reportable, Tier: TierRealizado, Source: SourceInvestment,
				SourceRefID: movement.ID, EventKey: "investment:" + movement.ID,
				CategoryName: noCategoryName}
			switch movement.Kind {
			case investments.OperationDeposit, investments.OperationWithdrawal:
				reportable = decimal.Zero
				entry.Description = "Resgate de investimento"
				if movement.Kind == investments.OperationDeposit {
					entry.Description = "Aporte em investimento"
				}
			case investments.OperationIncome:
				entry.Description = "Rendimento de investimento"
			case investments.OperationTax:
				entry.Description = "Imposto sobre investimento"
			default:
				entry.Description = "Custo de investimento"
			}
			entries = append(entries, entry)
		}
	}
	selection := projections.SelectionActive
	if len(params.ScenarioIDs) > 0 {
		selection = projections.SelectionExplicit
	}
	planned, err := projections.List(ctx, q, projections.ProjectionQuery{
		From: from, To: to, Selection: selection, IDs: params.ScenarioIDs,
	})
	if err != nil {
		return Series{}, err
	}
	for _, event := range planned {
		// A linked planned event is represented by the real transaction in
		// entries already. Keeping the suppression here makes it impossible
		// for a future adapter to accidentally reintroduce double counting.
		if event.Realized {
			continue
		}
		scenarioID := event.ScenarioID
		entries = append(entries, Entry{

			Date: event.Date, Description: event.Description, Amount: event.Amount,
			CategoryID: event.CategoryID, CategoryName: event.CategoryName,
			Tier: CertaintyTier(event.Tier), Source: SourceKind(event.Source),
			SourceRefID: event.EventKey, EventKey: event.EventKey, ScenarioID: &scenarioID,
		})
	}

	entries = withinWindow(entries, from, to)

	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Date.Before(entries[j].Date) })

	manualCash, err := anchoredManualCash(ctx, q, params)
	if err != nil {
		return Series{}, err
	}
	anchored := make(map[string]bool, len(manualCash))
	for _, entry := range manualCash {
		anchored[entry.EventKey] = true
	}
	walk := make([]Entry, 0, len(entries)+len(manualCash))
	for _, entry := range cashEntries(entries, bills.settled) {
		if !anchored[entry.EventKey] {
			walk = append(walk, entry)
		}
	}
	walk = append(walk, manualCash...)
	points := buildPoints(walk, balance, from, to, func(e Entry) bool {
		return anchored[e.EventKey] || (!e.Date.After(reference) && !awaitingCash(e, reference, bills.dueOnReference))
	})
	lowest, firstNegative := lowestAndFirstNegative(points, reference)

	var coverage reporting.Accumulator
	raw := map[string]*reporting.Accumulator{}
	for _, e := range entries {
		original := reportingAmount(e)
		currency := reporting.ReferenceCurrency
		if e.OriginalReportableAmount != nil {
			original = *e.OriginalReportableAmount
			currency = e.OriginalCurrency
		}
		if !original.IsZero() {
			if raw[currency] == nil {
				raw[currency] = &reporting.Accumulator{}
			}
			direction := money.Inflow
			if original.IsNegative() {
				direction = money.Outflow
			}
			raw[currency].Add(reporting.Conversion{Amount: &original, Status: "complete"}, direction)
		}
		if e.ReportingStatus == "unavailable" {
			coverage.Add(reporting.Conversion{Status: "unavailable"}, money.Unclassified)
			continue
		}
		amount := reportingAmount(e)
		if amount.IsZero() {
			continue
		}
		direction := money.Inflow
		if amount.IsNegative() {
			direction = money.Outflow
		}
		coverage.Add(reporting.Conversion{Amount: &amount, Status: e.ReportingStatus}, direction)
	}
	rawTotals := make([]reporting.Totals, 0, len(raw))
	for currency, a := range raw {
		totals := *a.Summary().Totals
		totals.CurrencyCode = currency
		rawTotals = append(rawTotals, totals)
	}
	sort.Slice(rawTotals, func(i, j int) bool { return rawTotals[i].CurrencyCode < rawTotals[j].CurrencyCode })
	if summary := coverage.Summary(); summary.Missing > 0 {
		return Series{}, &reporting.IncompleteError{Coverage: summary, Totals: rawTotals}
	}
	return Series{
		Totals:           rawTotals,
		Reporting:        coverage.Summary(),
		BalanceReporting: balanceReporting,
		Points:           points,
		Entries:          entries,
		StartingBalance:  balance,
		LowestBalance:    lowest,
		FirstNegative:    firstNegative,
	}, nil
}

// withinWindow drops entries whose date falls outside [from, to]. Only a
// real credit-card entry can land there: it is re-dated from occurred_at to
// its bill's due date (see realEntries), which can sit before from or past
// to even though the purchase itself was inside the queried window. Such an
// entry belongs to another window's series, and leaving it here would do
// more than list a row the chart cannot show: buildPoints anchors the
// balance by subtracting every entry dated on or before reference, so an
// entry before from would be subtracted from the anchor and never added
// back by any day in the loop — shifting every single point, and with them
// LowestBalance, away from the real balance.
func withinWindow(entries []Entry, from, to time.Time) []Entry {
	kept := entries[:0]
	for _, e := range entries {
		if e.Date.Before(from) || e.Date.After(to) {
			continue
		}
		kept = append(kept, e)
	}
	return kept
}

// cashEntries drops the entries in settled — the ones buildPoints must not
// walk — while leaving Series.Entries itself untouched.
//
// The split exists because the series answers two different questions on
// either side of the reference date, and only the balance walk cares:
//
//   - After reference the bill has not been paid yet, so the only way to
//     model its future cash impact is the purchases themselves, re-dated to
//     the due date (see realEntries). That is a forecast of a payment.
//   - On or before reference the payment is not hypothetical: it is a real
//     transaction on a cash account, already in this very series (a card-bill
//     payment carries a transfer-kind category, and MovesCash deliberately
//     keeps those — see eligibleRealItems). Walking the re-dated purchases
//     too charges the same bill twice.
//
// The second charge is invisible in a short window, where each bill's
// purchases and its payment leg roughly cancel, and grows without bound in a
// long one: the payments settle debt accrued before the window while the
// purchases are only those inside it, so the leftover shifts every past
// point by the difference — it was what put "Todo período" tens of
// thousands below any balance the user ever had.
//
// Entries stay in the Series regardless: the drill-down and the Relatório
// Financeiro's breakdowns read them as a ledger of what was spent, a
// question the anchor's cash-only definition does not govern.
func cashEntries(entries []Entry, settled map[string]bool) []Entry {
	if len(settled) == 0 {
		return entries
	}
	kept := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if settled[e.EventKey] {
			continue
		}
		kept = append(kept, e)
	}
	return kept
}

// awaitingCash reports whether e is dated on reference without being in cash
// on hand yet. The anchor is the bank's balance right now, so a real
// transaction dated today is already in it, but an event that is only
// projected for today (a recurrence, plan installment or scenario entry not
// yet reconciled with a transaction) has not moved any money, and neither
// has a card bill due today whose payment is not in the ledger.
func awaitingCash(e Entry, reference time.Time, dueOnReference map[string]bool) bool {
	if !e.Date.Equal(reference) {
		return false
	}
	switch e.Source {
	case SourceReal:
		return dueOnReference[e.EventKey]
	case SourceInvestment:
		return false
	default:
		return true
	}
}

// buildPoints produces one DayPoint per calendar day in [from, to] from the
// entries that model cash (cashEntries, not Series.Entries),
// anchoring the running balance so it equals startingBalance on
// reference (see the package's build.go comment for the reasoning): the
// balance right after every entry up to and including reference must sum
// to startingBalance, and every entry after reference adds onward from
// there — so a purely historical M3 series (all entries <= reference)
// reads as "how did the balance get to what it is today", while future
// entries (from M4 on) read as a forward projection from today.
//
// The exception is awaitingCash: entries dated on reference that cash on hand
// does not include yet are walked as future ones, so the point for reference
// is the balance at the end of the day, startingBalance minus what is still
// due today, not startingBalance itself. Future-dated posted manual movements
// already in startingBalance are also reversed before walking their dates.
func buildPoints(entries []Entry, startingBalance decimal.Decimal, from, to time.Time, inAnchor func(Entry) bool) []DayPoint {
	pastSum := decimal.Zero
	for _, e := range entries {
		if inAnchor(e) {
			pastSum = pastSum.Add(e.Amount)
		}
	}
	anchor := startingBalance.Sub(pastSum)

	byDay := map[time.Time]struct {
		net, inflow, outflow decimal.Decimal
		weakestTier          CertaintyTier
	}{}
	for _, e := range entries {
		d := byDay[e.Date]
		d.net = d.net.Add(e.Amount)
		if e.Amount.IsPositive() {
			d.inflow = d.inflow.Add(e.Amount)
		} else {
			d.outflow = d.outflow.Add(e.Amount)
		}
		if weaker(e.Tier, d.weakestTier) {
			d.weakestTier = e.Tier
		}
		byDay[e.Date] = d
	}

	var points []DayPoint
	running := anchor
	tier := TierRealizado
	for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
		d := byDay[day]
		running = running.Add(d.net)
		if d.weakestTier != "" {
			tier = d.weakestTier // a day with no entries of its own keeps the balance's last-known certainty
		}
		points = append(points, DayPoint{
			Date:       day,
			Balance:    running,
			Inflow:     d.inflow,
			Outflow:    d.outflow,
			LowestTier: tier,
		})
	}
	return points
}

// tierRank orders certainty from most to least sure — used to track, per
// day, the weakest tier among that day's own entries (a day with a
// Projetado entry reads as Projetado even if a Realizado entry also landed
// that day, since the balance is only as certain as its shakiest input).
var tierRank = map[CertaintyTier]int{
	TierRealizado:  0,
	TierConfirmado: 1,
	TierProjetado:  2,
	TierHipotetico: 3,
}

func weaker(candidate, current CertaintyTier) bool {
	return current == "" || tierRank[candidate] > tierRank[current]
}

// lowestAndFirstNegative looks at points from reference onward for warnings.
// A wholly historical window returns its own minimum, but never a future
// negative-balance warning. This also keeps LowestBalance a valid day point
// when the requested window ends before reference.
func lowestAndFirstNegative(points []DayPoint, reference time.Time) (DayPoint, *time.Time) {
	var lowest DayPoint
	var firstNegative *time.Time
	first := true
	for _, p := range points {
		if p.Date.Before(reference) {
			continue
		}
		if first || p.Balance.LessThan(lowest.Balance) {
			lowest = p
			first = false
		}
		if firstNegative == nil && p.Balance.IsNegative() {
			d := p.Date
			firstNegative = &d
		}
	}
	if first && len(points) > 0 {
		lowest = points[0]
		for _, p := range points[1:] {
			if p.Balance.LessThan(lowest.Balance) {
				lowest = p
			}
		}
	}
	return lowest, firstNegative
}
