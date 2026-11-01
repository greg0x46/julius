// Package projections is the read boundary between Scenario data and
// consumers such as the financial Timeline. It deliberately returns one
// event shape for plans, standalone scenarios, and recurring scenarios; the
// Timeline does not need to know which kind produced an event.
package projections

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/automation"
	"github.com/greg0x46/julius/internal/dates"
	"github.com/greg0x46/julius/internal/db"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/recurrences"
	"github.com/greg0x46/julius/internal/scenarios"
	"github.com/greg0x46/julius/internal/timeline/types"
	"github.com/greg0x46/julius/internal/transactions"
)

// SelectionMode makes the difference between the normal Timeline and an
// explicit simulation unambiguous. In particular, an empty ID slice is not
// overloaded to mean either "all active" or "none".
type SelectionMode string

const (
	SelectionActive   SelectionMode = "active"
	SelectionExplicit SelectionMode = "explicit"
	// SelectionActivePlus is the active scenarios plus the explicit IDs
	// (deduplicated): a simulation layered over the normal Timeline.
	SelectionActivePlus SelectionMode = "active_plus"
)

type ProjectionQuery struct {
	From      time.Time
	To        time.Time
	Selection SelectionMode
	IDs       []string
}

// PlannedTransaction is the canonical event emitted by a Scenario. EventKey
// is stable: it is derived from the scenario identity and the persisted
// transaction/occurrence identity, never generated during a read.
type PlannedTransaction struct {
	ScenarioID        string
	EventKey          string
	ScenarioKind      scenarios.Kind
	Date              time.Time
	Description       string
	Amount            decimal.Decimal
	CategoryID        *string
	CategoryName      string
	Tier              types.Tier
	Source            types.SourceKind
	PayableID         *string
	Realized          bool
	RealizationOrigin string
	Detached          bool
}

type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

var ErrInvalidSelection = errors.New("invalid projection selection")

// List reads all scenarios selected by query and delegates each kind to its
// adapter. The result is sorted deterministically so callers can merge it
// with real transactions without depending on table or adapter order.
//
// Every per-scenario read below is batched into one query for the whole
// selection. This is the read behind the dashboard and the Timeline, and the
// number of scenarios is user-controlled, so the round-trip count has to stay
// flat in it: a query per scenario (or worse, a full transaction scan per
// scenario) turns one screen into a load test.
func List(ctx context.Context, q Querier, query ProjectionQuery) ([]PlannedTransaction, error) {
	from, to := dates.Day(query.From), dates.Day(query.To)
	if to.Before(from) {
		return nil, fmt.Errorf("projection range: %w", ErrInvalidSelection)
	}
	selected, err := selectScenarios(ctx, q, query.Selection, query.IDs)
	if err != nil {
		return nil, err
	}
	planned, recurring := partitionByKind(selected)

	out, err := listPlannedTransactionProjections(ctx, q, planned, from, to)
	if err != nil {
		return nil, err
	}
	occurrences, err := listRecurringProjections(ctx, q, recurring, from, to)
	if err != nil {
		return nil, err
	}
	out = append(out, occurrences...)

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Date.Equal(out[j].Date) {
			return out[i].EventKey < out[j].EventKey
		}
		return out[i].Date.Before(out[j].Date)
	})
	return out, nil
}

// partitionByKind splits the selection in one pass. Plans and standalone
// scenarios share a projector because they are the same shape — stored
// installments with allocations — and differ only in tier and source.
// Recurring scenarios are the other shape: a schedule that generates its
// events on read.
func partitionByKind(selected []scenarios.Scenario) (planned, recurring []scenarios.Scenario) {
	for _, scenario := range selected {
		switch scenario.Kind {
		case scenarios.KindDebtPlan, scenarios.KindReceivablePlan:
			if scenario.PayableID != nil {
				planned = append(planned, scenario)
			}
		case scenarios.KindStandalone:
			planned = append(planned, scenario)
		case scenarios.KindRecurring:
			recurring = append(recurring, scenario)
		}
	}
	return planned, recurring
}

func selectScenarios(ctx context.Context, q Querier, selection SelectionMode, ids []string) ([]scenarios.Scenario, error) {
	switch selection {
	case SelectionActive:
		active := true
		return scenarios.ListScenarios(ctx, q, scenarios.ListFilter{IsActive: &active})
	case SelectionExplicit:
		if len(ids) == 0 {
			return []scenarios.Scenario{}, nil
		}
		return scenarios.ListScenariosByIDs(ctx, q, unique(ids))
	case SelectionActivePlus:
		active := true
		selected, err := scenarios.ListScenarios(ctx, q, scenarios.ListFilter{IsActive: &active})
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			return selected, nil
		}
		have := make(map[string]struct{}, len(selected))
		for _, scenario := range selected {
			have[scenario.ID] = struct{}{}
		}
		var missing []string
		for _, id := range unique(ids) {
			if _, ok := have[id]; !ok {
				missing = append(missing, id)
			}
		}
		if len(missing) == 0 {
			return selected, nil
		}
		extra, err := scenarios.ListScenariosByIDs(ctx, q, missing)
		if err != nil {
			return nil, err
		}
		return append(selected, extra...), nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrInvalidSelection, selection)
	}
}

func unique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

// listPlannedTransactionProjections projects every stored installment of the
// selection — payable-backed plans and standalone scenarios alike. The two
// differ only in amount sign, tier and source, so they share one pass and,
// more importantly, one batch of reads: the installments of the whole
// selection in one query, their allocations in another.
func listPlannedTransactionProjections(ctx context.Context, q Querier, planned []scenarios.Scenario, from, to time.Time) ([]PlannedTransaction, error) {
	if len(planned) == 0 {
		return nil, nil
	}
	scenarioIDs := make([]string, 0, len(planned))
	for _, scenario := range planned {
		scenarioIDs = append(scenarioIDs, scenario.ID)
	}
	installmentsByScenario, err := scenarios.ListScenarioTransactionsFor(ctx, q, scenarioIDs)
	if err != nil {
		return nil, err
	}

	// Walk the window once, keeping the installments that survive it next to
	// the scenario that owns them. The ids collected here are exactly the
	// rows this projection will emit, which is what lets the allocation
	// lookup be one query over precisely that set.
	type inRange struct {
		scenario    scenarios.Scenario
		installment scenarios.ScenarioTransaction
		day         time.Time
	}
	var (
		selected []inRange
		ids      []string
	)
	for _, scenario := range planned {
		for _, installment := range installmentsByScenario[scenario.ID] {
			day := dates.Day(installment.ProjectedAt)
			if day.Before(from) || day.After(to) {
				continue
			}
			selected = append(selected, inRange{scenario: scenario, installment: installment, day: day})
			ids = append(ids, installment.ID)
		}
	}
	allocations, err := allocationRealizations(ctx, q, ids)
	if err != nil {
		return nil, err
	}

	out := make([]PlannedTransaction, 0, len(selected))
	for _, one := range selected {
		// A standalone scenario has no payable behind it: its amount already
		// carries its own sign, and it never rises above hypothetical. A plan
		// is anchored to a real target, so it is Confirmado and its magnitude
		// is signed by the plan's direction.
		tier, source := types.TierHipotetico, types.SourceScenario
		amount := one.installment.Amount
		if one.scenario.Kind != scenarios.KindStandalone {
			tier, source = types.TierConfirmado, types.SourcePayablePlan
			amount = scenarios.SignedAmount(one.scenario.Kind, one.installment.Amount)
		}
		categoryName := noCategoryName
		if one.installment.Category != nil && *one.installment.Category != "" {
			categoryName = *one.installment.Category
		}
		allocation := allocations[one.installment.ID]
		out = append(out, PlannedTransaction{
			ScenarioID:        one.scenario.ID,
			EventKey:          transactionEventKey(one.scenario.ID, one.installment.ID),
			ScenarioKind:      one.scenario.Kind,
			Date:              one.day,
			Description:       one.installment.Description,
			Amount:            amount,
			CategoryName:      categoryName,
			Tier:              tier,
			Source:            source,
			PayableID:         one.scenario.PayableID,
			Realized:          allocation.realized,
			RealizationOrigin: allocation.origin,
		})
	}
	return out, nil
}

const noCategoryName = "Sem categoria"

// isoDate is how every date column in this schema is stored.
const isoDate = "2006-01-02"

// allRowsPageSize asks transactions.Query for the whole result set. The
// matcher this feeds is pure and reasons over a full period at once, so
// paging it would only mean reassembling the pages here.
const allRowsPageSize = 1_000_000

func transactionEventKey(scenarioID, transactionID string) string {
	return "scenario:" + scenarioID + ":transaction:" + transactionID
}

func occurrenceEventKey(scenarioID string, day time.Time) string {
	return "scenario:" + scenarioID + ":occurrence:" + dates.Day(day).Format("2006-01-02")
}

// allocationSummary is the answer the projector needs about one installment:
// whether any allocation realized it, and who made the most recent one that
// did — the rows arrive newest first, and the first origin seen wins.
type allocationSummary struct {
	realized bool
	origin   string
}

// allocationRealizations preserves the current ListPlanInstallments rule —
// any positive allocation suppresses the complete planned event — for a whole
// batch of installments in one query. Installments with no allocation are
// simply absent from the map, and the zero allocationSummary is the right
// answer for them.
func allocationRealizations(ctx context.Context, q Querier, scenarioTransactionIDs []string) (map[string]allocationSummary, error) {
	result := make(map[string]allocationSummary, len(scenarioTransactionIDs))
	if len(scenarioTransactionIDs) == 0 {
		return result, nil
	}
	in, args := db.InClause(scenarioTransactionIDs)
	rows, err := q.QueryContext(ctx, `
		SELECT scenario_transaction_id, state, origin, allocated_amount
		FROM scenario_realizations
		WHERE scenario_transaction_id IN (`+in+`) AND relation_type = 'allocation'
		ORDER BY created_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var installmentID, state, origin, amountRaw string
		if err := rows.Scan(dbtypes.ID(&installmentID), &state, &origin, dbtypes.Decimal(&amountRaw)); err != nil {
			return nil, err
		}
		amount, err := decimal.NewFromString(amountRaw)
		if err != nil {
			return nil, err
		}
		if state != "linked" || !amount.GreaterThan(decimal.Zero) {
			continue
		}
		summary := result[installmentID]
		summary.realized = true
		if summary.origin == "" {
			summary.origin = origin
		}
		result[installmentID] = summary
	}
	return result, rows.Err()
}

type scheduleRow struct {
	schedule     recurrences.RecurringSchedule
	categoryName string
}

// loadSchedules reads the schedules of a whole batch of recurring scenarios
// in one query. A recurring Scenario without a schedule row cannot be
// projected at all, so its absence from the map is an error the caller
// raises rather than a silent empty projection.
func loadSchedules(ctx context.Context, q Querier, scenarioIDs []string) (map[string]scheduleRow, error) {
	result := make(map[string]scheduleRow, len(scenarioIDs))
	if len(scenarioIDs) == 0 {
		return result, nil
	}
	in, args := db.InClause(scenarioIDs)
	rows, err := q.QueryContext(ctx, `
		SELECT s.scenario_id, s.cashflow_kind, s.amount, s.category_id, s.account_id, s.cadence,
		       s.day_of_month, s.month_of_year, s.start_date, s.end_date,
		       COALESCE(c.name, '')
		FROM scenario_recurring_schedules s
		LEFT JOIN categories c ON c.id = s.category_id
		WHERE s.scenario_id IN (`+in+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			scenarioID, cashflowKind, amountRaw, cadence, startRaw string
			categoryID, accountID, endRaw, categoryName            sql.NullString
			dayOfMonth                                             int
			monthOfYear                                            sql.NullInt64
		)
		if err := rows.Scan(
			dbtypes.ID(&scenarioID), &cashflowKind, dbtypes.Decimal(&amountRaw), dbtypes.ID(&categoryID), dbtypes.ID(&accountID), &cadence,
			&dayOfMonth, &monthOfYear, dbtypes.Date(&startRaw), dbtypes.Date(&endRaw), &categoryName,
		); err != nil {
			return nil, err
		}
		amount, err := decimal.NewFromString(amountRaw)
		if err != nil {
			return nil, err
		}
		start, err := dbtypes.ParseDate(startRaw)
		if err != nil {
			return nil, err
		}
		var end *time.Time
		if endRaw.Valid {
			parsed, err := dbtypes.ParseDate(endRaw.String)
			if err != nil {
				return nil, err
			}
			end = &parsed
		}
		var month *int
		if monthOfYear.Valid {
			value := int(monthOfYear.Int64)
			month = &value
		}
		var category *string
		if categoryID.Valid {
			category = &categoryID.String
		}
		var account *string
		if accountID.Valid {
			account = &accountID.String
		}
		result[scenarioID] = scheduleRow{schedule: recurrences.RecurringSchedule{
			CashflowKind: recurrences.Kind(cashflowKind), Amount: amount,
			CategoryID: category, AccountID: account, Cadence: recurrences.Cadence(cadence),
			DayOfMonth: dayOfMonth, MonthOfYear: month, StartDate: start, EndDate: end,
		}, categoryName: categoryName.String}
	}
	return result, rows.Err()
}

type occurrenceRealization struct {
	state, origin string
	transactionID *string
}

// cardReachMonths is how far before the window an occurrence on a credit card
// can fall and still have its bill due inside it. The Timeline reads real card
// entries back by the same amount (see timeline.Build).
const cardReachMonths = 2

// cardAccounts returns the credit-card accounts that some schedule is bound to,
// reading them only when a schedule names an account at all.
func cardAccounts(ctx context.Context, q Querier, schedules map[string]scheduleRow) (map[string]bool, error) {
	named := false
	for _, loaded := range schedules {
		named = named || loaded.schedule.AccountID != nil
	}
	if !named {
		return nil, nil
	}
	return transactions.CreditAccountIDs(ctx, q)
}

// listRecurringProjections dates an occurrence on its schedule day, except on a
// credit card: there it takes the day the Timeline gives the card's real
// entries (`CardDueDates.ProjectedEntryDate`, the bill's due date), so the
// projected and the realized cash fall on the same day. The event key and the
// reconciliation keep the occurrence day, the one the matcher works on.
func listRecurringProjections(ctx context.Context, q Querier, recurring []scenarios.Scenario, from, to time.Time) ([]PlannedTransaction, error) {
	if len(recurring) == 0 {
		return nil, nil
	}
	scenarioIDs := make([]string, 0, len(recurring))
	for _, scenario := range recurring {
		scenarioIDs = append(scenarioIDs, scenario.ID)
	}
	schedules, err := loadSchedules(ctx, q, scenarioIDs)
	if err != nil {
		return nil, err
	}
	cards, err := cardAccounts(ctx, q, schedules)
	if err != nil {
		return nil, err
	}
	onCard := func(schedule recurrences.RecurringSchedule) bool {
		return schedule.AccountID != nil && cards[*schedule.AccountID]
	}
	// A card occurrence before the window can still be paid inside it, so the
	// occurrences, their decisions and their matches are read that much earlier.
	scanFrom := from
	var dueDates transactions.CardDueDates
	for _, loaded := range schedules {
		if onCard(loaded.schedule) {
			scanFrom = dates.AddMonths(from, -cardReachMonths)
			if dueDates, err = transactions.FetchCardDueDates(ctx, q); err != nil {
				return nil, err
			}
			break
		}
	}

	// The manual decisions are read once, for every scenario, over the
	// widened window ManualLinkReachDays demands. They serve both readers
	// below: the override layer that wins over the rule, and the matcher's
	// own input — same rows, one query, so the two can never disagree.
	overrideFrom := scanFrom.AddDate(0, 0, -recurrences.ManualLinkReachDays)
	overrideTo := to.AddDate(0, 0, recurrences.ManualLinkReachDays)
	overrides, err := recurrences.ListOverrides(ctx, q, scenarioIDs, overrideFrom, overrideTo)
	if err != nil {
		return nil, err
	}
	resolvedByRule, err := resolveAutomaticOccurrences(ctx, q, recurring, schedules, overrides, scanFrom, to)
	if err != nil {
		return nil, err
	}

	var out []PlannedTransaction
	for _, scenario := range recurring {
		loaded, ok := schedules[scenario.ID]
		if !ok {
			return nil, fmt.Errorf("recurring scenario %s has no schedule", scenario.ID)
		}
		manual := make(map[time.Time]occurrenceRealization, len(overrides[scenario.ID]))
		for _, override := range overrides[scenario.ID] {
			one := occurrenceRealization{
				state:         string(override.State),
				origin:        string(override.Origin),
				transactionID: override.TransactionID,
			}
			manual[dates.Day(override.OccurrenceDate)] = one
		}
		categoryName := loaded.categoryName
		if categoryName == "" {
			categoryName = noCategoryName
		}
		card := onCard(loaded.schedule)
		occurrencesFrom := from
		if card {
			occurrencesFrom = scanFrom
		}
		for _, occurrence := range recurrences.OccurrencesInRange(loaded.schedule, occurrencesFrom, to) {
			day := dates.Day(occurrence.Date)
			date := day
			if card {
				date = dates.Day(dueDates.ProjectedEntryDate(*loaded.schedule.AccountID, day, nil, false))
				if date.Before(from) || date.After(to) {
					continue
				}
			}
			resolution := occurrenceRealization{}
			if override, ok := manual[day]; ok {
				resolution = override
			} else if automatic, ok := resolvedByRule[scenario.ID][day]; ok {
				resolution = automatic
			}
			amount := occurrence.ExpectedAmount
			if loaded.schedule.CashflowKind == recurrences.KindExpense {
				amount = amount.Neg()
			}
			out = append(out, PlannedTransaction{
				ScenarioID:        scenario.ID,
				EventKey:          occurrenceEventKey(scenario.ID, day),
				ScenarioKind:      scenario.Kind,
				Date:              date,
				Description:       scenario.Name,
				Amount:            amount,
				CategoryID:        loaded.schedule.CategoryID,
				CategoryName:      categoryName,
				Tier:              types.TierProjetado,
				Source:            types.SourceRecurring,
				Realized:          resolution.state == "linked" && resolution.transactionID != nil,
				RealizationOrigin: resolution.origin,
				Detached:          resolution.state == "detached",
			})
		}
	}
	return out, nil
}

func commitmentFromScenario(scenario scenarios.Scenario, schedule recurrences.RecurringSchedule) recurrences.RecurringCommitment {
	categoryID := ""
	if schedule.CategoryID != nil {
		categoryID = *schedule.CategoryID
	}
	return recurrences.RecurringCommitment{
		ID: scenario.ID, Name: scenario.Name, Kind: schedule.CashflowKind, Amount: schedule.Amount,
		CategoryID: categoryID, AccountID: schedule.AccountID, Cadence: schedule.Cadence,
		DayOfMonth: schedule.DayOfMonth, MonthOfYear: schedule.MonthOfYear,
		StartDate: schedule.StartDate, EndDate: schedule.EndDate, IsActive: true,
	}
}

// resolveAutomaticOccurrences applies the reconcile rules targeting the
// selection, keyed by scenario id. It runs the same pure matcher the
// occurrence endpoint uses, fed the same widened override window, so the
// Timeline and the reconciliation screen can never disagree about who claimed
// a transaction.
//
// The two expensive inputs — the active rules and the eligible transactions —
// depend on the period, not on the scenario, so they are read once for the
// whole batch. The candidate scan is skipped entirely when no scenario in the
// selection is targeted by a rule, which is the common case for a Timeline
// made only of plans and standalone scenarios.
func resolveAutomaticOccurrences(
	ctx context.Context,
	q Querier,
	recurring []scenarios.Scenario,
	schedules map[string]scheduleRow,
	overrides map[string][]recurrences.Override,
	from, to time.Time,
) (map[string]map[time.Time]occurrenceRealization, error) {
	result := map[string]map[time.Time]occurrenceRealization{}
	targets, err := automation.ListActiveReconcileTargets(ctx, q)
	if err != nil {
		return nil, err
	}
	targeted := make([]scenarios.Scenario, 0, len(recurring))
	for _, scenario := range recurring {
		if _, ok := targets[scenario.ID]; ok {
			targeted = append(targeted, scenario)
		}
	}
	if len(targeted) == 0 {
		return result, nil
	}
	items, err := eligibleCandidates(ctx, q,
		from.AddDate(0, 0, -recurrences.ManualLinkReachDays),
		to.AddDate(0, 0, recurrences.ManualLinkReachDays))
	if err != nil {
		return nil, err
	}
	for _, scenario := range targeted {
		loaded, ok := schedules[scenario.ID]
		if !ok {
			return nil, fmt.Errorf("recurring scenario %s has no schedule", scenario.ID)
		}
		target := targets[scenario.ID]
		// The caller already decided this scenario participates; IsActive
		// only gates OccurrencesInRange, which the caller runs itself.
		commitment := commitmentFromScenario(scenario, loaded.schedule)
		reconciler := recurrences.NewReconciler(
			commitment, target.Conditions, target.LogicOperator, true, overrides[scenario.ID], items)
		resolved := map[time.Time]occurrenceRealization{}
		for _, one := range reconciler.ResolveRange(from, to) {
			if one.Reconciled() {
				resolved[dates.Day(one.Occurrence.Date)] = occurrenceRealization{
					state: "linked", origin: string(one.Origin), transactionID: one.TransactionID,
				}
			}
		}
		result[scenario.ID] = resolved
	}
	return result, nil
}

// eligibleCandidates loads every transaction in [from, to] that could satisfy
// an occurrence: considered for totals, with an effective amount and a date.
// The matcher is pure and needs the whole window in memory, so this is a
// deliberate full read of the period — which is exactly why the caller does
// it once per request instead of once per scenario.
func eligibleCandidates(ctx context.Context, q Querier, from, to time.Time) ([]transactions.Item, error) {
	fromDate := money.Date{Year: from.Year(), Month: from.Month(), Day: from.Day()}
	toDate := money.Date{Year: to.Year(), Month: to.Month(), Day: to.Day()}
	result, err := transactions.Query(ctx, q, transactions.QueryRequest{
		Timezone: "UTC", GroupBy: money.GroupNone, Page: 1, PageSize: allRowsPageSize,
		Filters: transactions.Filters{DateFrom: &fromDate, DateTo: &toDate},
	})
	if err != nil {
		return nil, err
	}
	items := make([]transactions.Item, 0, len(result.Items))
	for _, item := range result.Items {
		// MovesCash, not Included: a commitment is realized by money actually
		// leaving the account, and a transfer categorized as such still did
		// that. The recurring-commitment form refuses to *project* a transfer
		// category (see AutomationEntryForm), but a rule is free to put one on
		// a real transaction, and that must not make the occurrence look
		// unpaid — the Timeline would then draw the projection and the
		// transaction side by side, double-counting the same money.
		if item.TotalsEligibility.MovesCash() && item.EffectiveMoney != nil && item.OccurredAt != nil {
			items = append(items, item)
		}
	}
	return items, nil
}
