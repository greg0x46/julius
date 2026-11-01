package recurrences

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/greg0x46/julius/internal/dates"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/transactions"
	"github.com/shopspring/decimal"
)

// Suggestion is an unpersisted BRL draft. An empty category requires owner input.
type Suggestion struct {
	Draft          RecurringCommitment
	AmountKind     string
	MinimumAmount  decimal.Decimal
	MaximumAmount  decimal.Decimal
	Confidence     string
	Reasons        []string
	TransactionIDs []string
	CardNumber     string
}

// Discover only reads ledger history, schedules and explicit recurrence links.
func Discover(ctx context.Context, q Querier, asOf time.Time) ([]Suggestion, error) {
	items, err := transactions.History(ctx, q)
	if err != nil {
		return nil, err
	}
	existing, err := List(ctx, q)
	if err != nil {
		return nil, err
	}
	linked, err := LinkedTransactionIDs(ctx, q)
	if err != nil {
		return nil, err
	}
	history := make([]transactions.Item, 0, len(items))
	for _, item := range items {
		if !linked[item.ID] {
			history = append(history, item)
		}
	}
	return Detect(history, existing, asOf), nil
}

var installmentDescription = regexp.MustCompile(`(?i)(\b\d{1,3}\s*/\s*\d{1,3}\b|\bparc(?:ela)?\.?\s*\d+)`)
var descriptionDate = regexp.MustCompile(`\b(?:\d{4}-\d{2}-\d{2}|\d{2}/\d{2}/\d{4})\b`)

// Keep merchant identifiers; unlike learned categorization, installments must
// be rejected before normalization rather than merged into a recurring group.
func discoveryKey(description string) string {
	description = descriptionDate.ReplaceAllString(description, " ")
	return strings.Join(strings.FieldsFunc(strings.ToLower(description), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}), " ")
}

type discoveryGroup struct {
	name, account, card string
	kind                Kind
}
type observation struct {
	item   transactions.Item
	day    time.Time
	amount decimal.Decimal
}

// Detect applies conservative calendar and amount heuristics without mutating inputs.
func Detect(items []transactions.Item, existing []RecurringCommitment, asOf time.Time) []Suggestion {
	asOf = dates.Day(asOf)
	groups := map[discoveryGroup][]observation{}
	for _, item := range items {
		if !item.TotalsEligibility.Included || item.OccurredAt == nil || item.Description == nil || item.EffectiveMoney == nil || item.EffectiveMoney.CurrencyCode != "BRL" {
			continue
		}
		if item.ProviderStatus == nil || *item.ProviderStatus != "POSTED" || item.Inclusion.State == money.Ignored {
			continue
		}
		if item.Classification != money.Inflow && item.Classification != money.Outflow {
			continue
		}
		if item.InvestmentTransferAmount != "" && item.InvestmentTransferAmount != "0" {
			continue
		}
		description := descriptionDate.ReplaceAllString(*item.Description, " ")
		if installmentDescription.MatchString(description) {
			continue
		}
		card := ""
		if item.Card != nil {
			if item.Card.InstallmentNumber != nil || (item.Card.TotalInstallments != nil && *item.Card.TotalInstallments > 1) {
				continue
			}
			card = item.Card.Number
		}
		name := discoveryKey(*item.Description)
		if len([]rune(name)) < 3 || item.Account.ID == "" {
			continue
		}
		amount, err := decimal.NewFromString(item.EffectiveMoney.Value)
		if err != nil || amount.IsZero() {
			continue
		}
		day := dates.Day(*item.OccurredAt)
		if day.After(asOf) {
			continue
		}
		kind := KindExpense
		if item.Classification == money.Inflow {
			kind = KindIncome
		}
		key := discoveryGroup{name, item.Account.ID, card, kind}
		groups[key] = append(groups[key], observation{item, day, amount.Abs()})
	}
	keys := make([]discoveryGroup, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.name != b.name {
			return a.name < b.name
		}
		if a.account != b.account {
			return a.account < b.account
		}
		if a.card != b.card {
			return a.card < b.card
		}
		return a.kind < b.kind
	})
	result := []Suggestion{}
	for _, key := range keys {
		observations := groups[key]
		sort.Slice(observations, func(i, j int) bool {
			if observations[i].day.Equal(observations[j].day) {
				return observations[i].item.ID < observations[j].item.ID
			}
			return observations[i].day.Before(observations[j].day)
		})
		// Identical same-day movements cannot supply additional cadence evidence.
		unique := make([]observation, 0, len(observations))
		seen := map[string]bool{}
		for _, o := range observations {
			fingerprint := o.day.Format(dateLayout) + "/" + o.amount.String()
			if seen[fingerprint] {
				continue
			}
			seen[fingerprint] = true
			unique = append(unique, o)
		}
		suggestion, ok := detectGroup(key, unique, asOf)
		if !ok || coveredSuggestion(suggestion, existing) {
			continue
		}
		result = append(result, suggestion)
	}
	return result
}

func monthIndex(day time.Time) int { return day.Year()*12 + int(day.Month()) }

func detectGroup(key discoveryGroup, observations []observation, asOf time.Time) (Suggestion, bool) {
	if len(observations) < 3 {
		return Suggestion{}, false
	}
	first, last := observations[0].day, observations[len(observations)-1].day
	cadence := CadenceMonthly
	annual := true
	for i := 1; i < len(observations); i++ {
		if monthIndex(observations[i].day)-monthIndex(observations[i-1].day) != 12 {
			annual = false
		}
	}
	missing := 0
	if annual {
		cadence = CadenceAnnual
		if monthIndex(asOf)-monthIndex(last) > 14 {
			return Suggestion{}, false
		}
	} else {
		span := monthIndex(last) - monthIndex(first) + 1
		if len(observations) < 4 || len(observations)*4 < span*3 || monthIndex(asOf)-monthIndex(last) > 2 {
			return Suggestion{}, false
		}
		for i := 1; i < len(observations); i++ {
			gap := monthIndex(observations[i].day) - monthIndex(observations[i-1].day)
			if gap < 1 || gap > 2 {
				return Suggestion{}, false
			}
			missing += gap - 1
		}
	}
	// Pick the calendar anchor minimizing total day deviation, clamping short months.
	anchor, best := 1, len(observations)*32
	for day := 1; day <= 31; day++ {
		deviation := 0
		for _, o := range observations {
			deviation += absDay(o.day.Day() - min(day, dates.DaysInMonth(o.day.Year(), o.day.Month())))
		}
		if deviation < best {
			anchor, best = day, deviation
		}
	}
	tight := true
	for _, o := range observations {
		delta := absDay(o.day.Day() - min(anchor, dates.DaysInMonth(o.day.Year(), o.day.Month())))
		if delta > 3 {
			return Suggestion{}, false
		}
		if delta > 1 {
			tight = false
		}
	}
	amounts := make([]decimal.Decimal, len(observations))
	for i, o := range observations {
		amounts[i] = o.amount
	}
	sort.Slice(amounts, func(i, j int) bool { return amounts[i].LessThan(amounts[j]) })
	low, high := amounts[0], amounts[len(amounts)-1]
	if high.GreaterThan(low.Mul(decimal.NewFromInt(2))) {
		return Suggestion{}, false
	}
	median := amounts[len(amounts)/2]
	if len(amounts)%2 == 0 {
		median = median.Add(amounts[len(amounts)/2-1]).Mul(decimal.New(5, -1))
		median = decimal.RequireFromString(median.String())
	}
	amountKind := "fixed"
	if high.Sub(low).GreaterThan(median.Mul(decimal.RequireFromString("0.02"))) {
		amountKind = "variable"
	}
	categoryCounts := map[string]int{}
	for _, o := range observations {
		if c := o.item.InternalCategory; c != nil && c.IsActive {
			categoryCounts[c.ID]++
		}
	}
	category := ""
	for id, count := range categoryCounts {
		if count*2 > len(observations) {
			category = id
		}
	}
	account := key.account
	step := 1
	var month *int
	if annual {
		step = 12
		m := int(last.Month())
		month = &m
	}
	start := dates.AddMonthsOnDay(last, anchor, step)
	for !start.After(asOf) {
		start = dates.AddMonthsOnDay(start, anchor, step)
	}
	confidence := "medium"
	if amountKind == "fixed" && missing == 0 && tight && (annual || len(observations) >= 6) {
		confidence = "high"
	}
	reasons := []string{"consistent_calendar", "one_observation_per_period", amountKind + "_amount"}
	if missing > 0 {
		reasons = append(reasons, "missing_months")
	}
	if category == "" {
		reasons = append(reasons, "category_required")
	}
	ids := make([]string, len(observations))
	for i, o := range observations {
		ids[i] = o.item.ID
	}
	return Suggestion{
		Draft:      RecurringCommitment{Name: strings.TrimSpace(*observations[len(observations)-1].item.Description), Kind: key.kind, Amount: median, CategoryID: category, AccountID: &account, Cadence: cadence, DayOfMonth: anchor, MonthOfYear: month, StartDate: start, IsActive: true},
		AmountKind: amountKind, MinimumAmount: low, MaximumAmount: high, Confidence: confidence, Reasons: reasons, TransactionIDs: ids, CardNumber: key.card,
	}, true
}

func absDay(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func coveredSuggestion(s Suggestion, existing []RecurringCommitment) bool {
	draft := s.Draft
	for _, c := range existing {
		if c.Kind != draft.Kind || c.Cadence != draft.Cadence || (c.AccountID != nil && *c.AccountID != *draft.AccountID) {
			continue
		}
		if c.Cadence == CadenceAnnual && (c.MonthOfYear == nil || *c.MonthOfYear != *draft.MonthOfYear) {
			continue
		}
		if discoveryKey(c.Name) == discoveryKey(draft.Name) {
			return true
		}
		if absDay(c.DayOfMonth-draft.DayOfMonth) <= 3 && c.CategoryID != "" && c.CategoryID == draft.CategoryID && c.Amount.GreaterThanOrEqual(s.MinimumAmount.Mul(decimal.RequireFromString("0.9"))) && c.Amount.LessThanOrEqual(s.MaximumAmount.Mul(decimal.RequireFromString("1.1"))) {
			return true
		}
	}
	return false
}
