package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/automation"
	"github.com/greg0x46/julius/internal/dates"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/recurrences"
	"github.com/greg0x46/julius/internal/transactions"
)

// candidateWindowDays is the domain's manual-link reach (see
// recurrences.ManualLinkReachDays) used as the candidate window, so what this
// layer offers and what the resolver treats as reachable can't drift apart.
const candidateWindowDays = recurrences.ManualLinkReachDays

// occurrenceHistoryMonths is how far back the default occurrence window
// reaches. Reviewing reconciliation is a backward-looking task — the
// question is "did last month's rent get matched", not "will next year's" —
// so the default runs from the start of the month this many months ago
// through the end of the current one.
const occurrenceHistoryMonths = 5

type reconciledTransactionDTO struct {
	ID             string                      `json:"id"`
	OccurredAt     *time.Time                  `json:"occurred_at"`
	Description    *string                     `json:"description"`
	AccountName    *string                     `json:"account_name"`
	EffectiveMoney eligibleTransactionMoneyDTO `json:"effective_money"`
}

type recurrenceOccurrenceDTO struct {
	Date           string                    `json:"date"`
	ExpectedAmount string                    `json:"expected_amount"`
	Status         string                    `json:"status"` // reconciled | unreconciled | detached
	Origin         *string                   `json:"origin"` // rule | manual, nil when unreconciled
	Transaction    *reconciledTransactionDTO `json:"transaction"`
}

func recurrenceReconciliationUnavailableProblem(w http.ResponseWriter) {
	writeProblem(w, 503, "recurrence-reconciliation-unavailable", "Conciliação temporariamente indisponível",
		"Tente novamente em instantes.")
}

func recurringCommitmentNotFoundProblem(w http.ResponseWriter) {
	writeProblem(w, 404, "recurring-commitment-not-found", "Compromisso não encontrado", "")
}

// ineligibleReasonDetail turns the domain's reason into something a user can
// act on, rather than echoing the enum.
func ineligibleReasonDetail(reason recurrences.ReconcileIneligibilityReason) string {
	switch reason {
	case recurrences.ReasonIgnored:
		return "Esta transação está ignorada e não entra nos totais, então não pode conciliar um compromisso."
	case recurrences.ReasonNotInflow:
		return "Este compromisso é uma receita, então só pode ser conciliado por uma entrada."
	case recurrences.ReasonNotOutflow:
		return "Este compromisso é uma despesa, então só pode ser conciliado por uma saída."
	case recurrences.ReasonMissingBRLPair:
		return "Só transações em reais podem conciliar um compromisso."
	case recurrences.ReasonAlreadyReconciled:
		return "Esta transação já concilia outra ocorrência."
	}
	return ""
}

// toReconciledTransactionDTO renders a transaction for display next to the
// occurrence it settles. The value is a magnitude — the direction is already
// implied by the commitment's Kind — matching how the payables candidate DTO
// reports one.
//
// Every item reaching here comes from realItemsIn, which guarantees an
// effective money pair; the fallback exists only so a future caller passing
// something looser degrades to a readable zero instead of an empty string
// the client's parser would reject.
func toReconciledTransactionDTO(item transactions.Item) reconciledTransactionDTO {
	dto := reconciledTransactionDTO{
		ID: item.ID, OccurredAt: item.OccurredAt, Description: item.Description,
		AccountName: item.Account.Name,
		EffectiveMoney: eligibleTransactionMoneyDTO{
			Value: money.CanonicalDecimal(decimal.Zero), CurrencyCode: "BRL",
		},
	}
	if item.EffectiveMoney != nil {
		value := item.EffectiveMoney.Value
		if parsed, err := decimal.NewFromString(value); err == nil {
			value = money.CanonicalDecimal(parsed.Abs())
		}
		dto.EffectiveMoney = eligibleTransactionMoneyDTO{
			Value: value, CurrencyCode: item.EffectiveMoney.CurrencyCode,
		}
	}
	return dto
}

func toOccurrenceDTO(resolved recurrences.Reconciliation) recurrenceOccurrenceDTO {
	dto := recurrenceOccurrenceDTO{
		Date:           resolved.Occurrence.Date.Format(dateOnlyLayout),
		ExpectedAmount: money.CanonicalDecimal(resolved.Occurrence.ExpectedAmount),
		Status:         "unreconciled",
	}
	if resolved.Detached {
		dto.Status = "detached"
	}
	if resolved.Reconciled() {
		dto.Status = "reconciled"
		origin := string(resolved.Origin)
		dto.Origin = &origin
		if resolved.Transaction != nil {
			transaction := toReconciledTransactionDTO(*resolved.Transaction)
			dto.Transaction = &transaction
		}
	}
	return dto
}

// realItemsIn loads the real transactions of a window that are eligible to
// reconcile anything, applying exactly the filter timeline.eligibleRealItems
// applies.
//
// Matching that filter is not cosmetic. The Reconciler is fed this list on
// both sides, so if this layer accepted a transaction the Timeline rejects —
// an ignored one, say — the same occurrence would read as "Conciliada
// (automática)" on screen while still projecting in the report. The two
// views would disagree about the same fact. That is also why the test is
// MovesCash rather than Included: the Timeline moved to it for balance
// correctness, and this layer has to move with it.
func realItemsIn(ctx context.Context, conn *sql.DB, from, to time.Time) ([]transactions.Item, error) {
	fromDate := money.Date{Year: from.Year(), Month: from.Month(), Day: from.Day()}
	toDate := money.Date{Year: to.Year(), Month: to.Month(), Day: to.Day()}
	result, err := transactions.Query(ctx, conn, transactions.QueryRequest{
		Timezone: "UTC",
		GroupBy:  money.GroupNone,
		Page:     1,
		PageSize: 1_000_000,
		Filters:  transactions.Filters{DateFrom: &fromDate, DateTo: &toDate},
	})
	if err != nil {
		return nil, err
	}
	eligible := make([]transactions.Item, 0, len(result.Items))
	for _, item := range result.Items {
		if !item.TotalsEligibility.MovesCash() || item.EffectiveMoney == nil || item.OccurredAt == nil {
			continue
		}
		eligible = append(eligible, item)
	}
	return eligible, nil
}

// reconcilerFor assembles everything needed to resolve one commitment's
// occurrences over [from, to]: its automation rule (if any), the user's
// overrides in that window, and the real transactions of the window. The
// candidate window is widened by candidateWindowDays so a manual link
// pointing just outside [from, to] still resolves with its transaction
// attached instead of showing as a bare id.
func reconcilerFor(ctx context.Context, conn *sql.DB, commitment recurrences.RecurringCommitment, from, to time.Time) (recurrences.Reconciler, error) {
	targets, err := automation.ListActiveReconcileTargets(ctx, conn)
	if err != nil {
		return recurrences.Reconciler{}, err
	}
	// Widened like the candidate window below, and for the same reason the
	// Timeline widens its own lookup: a link on an occurrence just outside
	// [from, to] still governs whether the rule may reuse its transaction.
	overrides, err := recurrences.ListOverrides(ctx, conn, []string{commitment.ID},
		from.AddDate(0, 0, -candidateWindowDays), to.AddDate(0, 0, candidateWindowDays))
	if err != nil {
		return recurrences.Reconciler{}, err
	}
	items, err := realItemsIn(ctx, conn,
		from.AddDate(0, 0, -candidateWindowDays), to.AddDate(0, 0, candidateWindowDays))
	if err != nil {
		return recurrences.Reconciler{}, err
	}
	rule, hasRule := targets[commitment.ID]
	return recurrences.NewReconciler(commitment, rule.Conditions, rule.LogicOperator, hasRule,
		overrides[commitment.ID], items), nil
}

// occurrenceWindow reads the optional from/to query params, defaulting to
// the trailing months that make reconciliation reviewable (see
// occurrenceHistoryMonths).
func occurrenceWindow(r *http.Request) (from, to time.Time, err error) {
	now := dates.Day(time.Now())
	from = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -occurrenceHistoryMonths, 0)
	to = time.Date(now.Year(), now.Month(), dates.DaysInMonth(now.Year(), now.Month()), 0, 0, 0, 0, time.UTC)

	if v := r.URL.Query().Get("from"); v != "" {
		if from, err = time.Parse(dateOnlyLayout, v); err != nil {
			return time.Time{}, time.Time{}, errors.New("from inválido")
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if to, err = time.Parse(dateOnlyLayout, v); err != nil {
			return time.Time{}, time.Time{}, errors.New("to inválido")
		}
	}
	if dates.ExceedsMaxSpan(from, to) {
		return time.Time{}, time.Time{}, errRangeTooLong
	}
	if to.Before(from) {
		return time.Time{}, time.Time{}, errors.New("to não pode ser anterior a from")
	}
	return from, to, nil
}

func handleListRecurrenceOccurrences(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		commitment, err := recurrences.Get(ctx, conn, r.PathValue("id"))
		if errors.Is(err, recurrences.ErrNotFound) {
			recurringCommitmentNotFoundProblem(w)
			return
		}
		if err != nil {
			recurrenceReconciliationUnavailableProblem(w)
			return
		}
		from, to, err := occurrenceWindow(r)
		if err != nil {
			writeProblem(w, 422, "invalid-occurrence-window", "Período inválido", err.Error())
			return
		}
		reconciler, err := reconcilerFor(ctx, conn, commitment, from, to)
		if err != nil {
			recurrenceReconciliationUnavailableProblem(w)
			return
		}
		resolved := reconciler.ResolveRange(from, to)
		dtos := make([]recurrenceOccurrenceDTO, len(resolved))
		for i, one := range resolved {
			dtos[i] = toOccurrenceDTO(one)
		}
		// Newest first: the occurrence a user is coming to check is almost
		// always the most recent one.
		sort.Slice(dtos, func(i, j int) bool { return dtos[i].Date > dtos[j].Date })
		writeJSON(w, http.StatusOK, dtos)
	}
}

// requireOccurrence resolves {id} and {date} into a commitment and a date
// that is genuinely one of its occurrences. Validating the date against the
// schedule (rather than accepting any date) is what keeps
// scenario_realizations from accumulating rows no read will ever look
// at — occurrences have no stored identity beyond their date, so a typo'd
// date would silently create an override that never resolves.
func requireOccurrence(w http.ResponseWriter, r *http.Request, conn *sql.DB) (recurrences.RecurringCommitment, time.Time, bool) {
	commitment, err := recurrences.Get(r.Context(), conn, r.PathValue("id"))
	if errors.Is(err, recurrences.ErrNotFound) {
		recurringCommitmentNotFoundProblem(w)
		return recurrences.RecurringCommitment{}, time.Time{}, false
	}
	if err != nil {
		recurrenceReconciliationUnavailableProblem(w)
		return recurrences.RecurringCommitment{}, time.Time{}, false
	}
	date, err := time.Parse(dateOnlyLayout, r.PathValue("date"))
	if err != nil {
		writeProblem(w, 422, "invalid-recurrence-occurrence", "Ocorrência inválida",
			"A data da ocorrência deve estar no formato AAAA-MM-DD.")
		return recurrences.RecurringCommitment{}, time.Time{}, false
	}
	// IsActive gates OccurrencesInRange, so a paused commitment reports no
	// occurrences at all — checked here so the 422 explains that instead of
	// claiming the date is wrong.
	if !commitment.IsActive {
		writeProblem(w, 422, "invalid-recurrence-occurrence", "Compromisso pausado",
			"Reative o compromisso para conciliar suas ocorrências.")
		return recurrences.RecurringCommitment{}, time.Time{}, false
	}
	if len(commitment.Occurrences(date, date)) == 0 {
		writeProblem(w, 422, "invalid-recurrence-occurrence", "Ocorrência inválida",
			"Esta data não é uma ocorrência deste compromisso.")
		return recurrences.RecurringCommitment{}, time.Time{}, false
	}
	return commitment, date, true
}

// normalizedSearch folds a search box's value into the form matchesSearch
// compares against; an empty needle means "no filter".
func normalizedSearch(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// matchesSearch is the case-insensitive substring match the payables
// candidate search uses, applied to a description that may be absent.
func matchesSearch(description *string, needle string) bool {
	if needle == "" {
		return true
	}
	if description == nil {
		return false
	}
	return strings.Contains(strings.ToLower(*description), needle)
}

func handleListReconciliationCandidates(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		commitment, date, ok := requireOccurrence(w, r, conn)
		if !ok {
			return
		}
		limit := 20
		if v := r.URL.Query().Get("limit"); v != "" {
			if parsed, err := parsePositiveInt(v); err == nil && parsed >= 1 && parsed <= 50 {
				limit = parsed
			}
		}
		search := normalizedSearch(r.URL.Query().Get("search"))

		items, err := realItemsIn(ctx, conn,
			date.AddDate(0, 0, -candidateWindowDays), date.AddDate(0, 0, candidateWindowDays))
		if err != nil {
			recurrenceReconciliationUnavailableProblem(w)
			return
		}
		linked, err := recurrences.LinkedTransactionIDs(ctx, conn)
		if err != nil {
			recurrenceReconciliationUnavailableProblem(w)
			return
		}

		eligible := make([]transactions.Item, 0, len(items))
		for _, item := range items {
			if !matchesSearch(item.Description, search) {
				continue
			}
			if !recurrences.EligibilityForItem(commitment.Kind, item, linked[item.ID]).Eligible {
				continue
			}
			eligible = append(eligible, item)
		}
		sortCandidates(eligible, date, commitment.Amount)
		if len(eligible) > limit {
			eligible = eligible[:limit]
		}

		dtos := make([]eligibleTransactionDTO, len(eligible))
		for i, item := range eligible {
			one := toReconciledTransactionDTO(item)
			dtos[i] = eligibleTransactionDTO{
				ID: one.ID, OccurredAt: one.OccurredAt, Description: one.Description,
				AccountName: one.AccountName, EffectiveMoney: one.EffectiveMoney,
			}
		}
		writeJSON(w, http.StatusOK, dtos)
	}
}

// sortCandidates puts the likeliest pick first: closest to the occurrence's
// date, then closest to its expected amount. Both signals matter — a
// recurring charge lands near its day and near its value — and date leads
// because a wrong-by-a-few-reais charge on the right day is far more often
// the one than an exact-value charge three weeks off.
func sortCandidates(items []transactions.Item, occurrence time.Time, expected decimal.Decimal) {
	distance := func(item transactions.Item) time.Duration {
		delta := dates.Day(*item.OccurredAt).Sub(dates.Day(occurrence))
		if delta < 0 {
			return -delta
		}
		return delta
	}
	amountGap := func(item transactions.Item) decimal.Decimal {
		if item.EffectiveMoney == nil {
			return decimal.NewFromInt(1 << 30)
		}
		value, err := decimal.NewFromString(item.EffectiveMoney.Value)
		if err != nil {
			return decimal.NewFromInt(1 << 30)
		}
		return value.Abs().Sub(expected).Abs()
	}
	sort.SliceStable(items, func(i, j int) bool {
		di, dj := distance(items[i]), distance(items[j])
		if di != dj {
			return di < dj
		}
		return amountGap(items[i]).LessThan(amountGap(items[j]))
	})
}

type reconciliationWriteRequest struct {
	State         string  `json:"state"`
	TransactionID *string `json:"transaction_id"`
}

func handlePutRecurrenceReconciliation(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		commitment, date, ok := requireOccurrence(w, r, conn)
		if !ok {
			return
		}
		var req reconciliationWriteRequest
		if err := decodeStrict(r, &req); err != nil {
			writeProblem(w, 422, "invalid-reconciliation", "Conciliação inválida", "")
			return
		}

		state := recurrences.OverrideState(req.State)
		switch state {
		case recurrences.StateDetached:
			req.TransactionID = nil
		case recurrences.StateLinked:
			if req.TransactionID == nil || *req.TransactionID == "" {
				writeProblem(w, 422, "invalid-reconciliation", "Conciliação inválida",
					"Escolha a transação que concilia esta ocorrência.")
				return
			}
			item, found, err := transactions.GetItem(ctx, conn, *req.TransactionID)
			if err != nil {
				recurrenceReconciliationUnavailableProblem(w)
				return
			}
			if !found {
				writeProblem(w, 404, "transaction-not-found", "Transação não encontrada", "")
				return
			}
			// Already linked to *this* occurrence is a no-op replacement, not
			// a conflict — only another occurrence's claim is.
			existing, hasExisting, err := recurrences.OverrideForTransaction(ctx, conn, item.ID)
			if err != nil {
				recurrenceReconciliationUnavailableProblem(w)
				return
			}
			takenElsewhere := hasExisting &&
				!(existing.ScenarioID == commitment.ID && dates.Day(existing.OccurrenceDate).Equal(dates.Day(date)))
			if takenElsewhere {
				writeProblem(w, 409, "transaction-already-reconciled", "Transação já conciliada",
					ineligibleReasonDetail(recurrences.ReasonAlreadyReconciled))
				return
			}
			if eligibility := recurrences.EligibilityForItem(commitment.Kind, item, false); !eligibility.Eligible {
				writeProblem(w, 422, "ineligible-transaction", "Transação inelegível",
					ineligibleReasonDetail(*eligibility.Reason))
				return
			}
		default:
			writeProblem(w, 422, "invalid-reconciliation", "Conciliação inválida",
				`state deve ser "linked" ou "detached".`)
			return
		}

		if _, err := recurrences.PutOverride(ctx, conn, commitment.ID, date, state, req.TransactionID); err != nil {
			if errors.Is(err, recurrences.ErrTransactionAlreadyReconciled) {
				writeProblem(w, 409, "transaction-already-reconciled", "Transação já conciliada",
					ineligibleReasonDetail(recurrences.ReasonAlreadyReconciled))
				return
			}
			recurrenceReconciliationUnavailableProblem(w)
			return
		}
		writeResolvedOccurrence(w, ctx, conn, commitment, date)
	}
}

func handleDeleteRecurrenceReconciliation(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		commitment, date, ok := requireOccurrence(w, r, conn)
		if !ok {
			return
		}
		err := recurrences.DeleteOverride(r.Context(), conn, commitment.ID, date)
		if errors.Is(err, recurrences.ErrOverrideNotFound) {
			writeProblem(w, 404, "recurrence-reconciliation-not-found", "Conciliação não encontrada",
				"Esta ocorrência não tem nenhuma decisão manual para desfazer.")
			return
		}
		if err != nil {
			recurrenceReconciliationUnavailableProblem(w)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// writeResolvedOccurrence answers a write with the occurrence as the next
// read would resolve it, so the client never has to guess what the write
// produced — a manual link can still be overridden by nothing, but a detach
// changes which of the three states the row is in.
func writeResolvedOccurrence(w http.ResponseWriter, ctx context.Context, conn *sql.DB, commitment recurrences.RecurringCommitment, date time.Time) {
	reconciler, err := reconcilerFor(ctx, conn, commitment, date, date)
	if err != nil {
		recurrenceReconciliationUnavailableProblem(w)
		return
	}
	resolved := reconciler.ResolveRange(date, date)
	if len(resolved) == 0 {
		recurrenceReconciliationUnavailableProblem(w)
		return
	}
	writeJSON(w, http.StatusOK, toOccurrenceDTO(resolved[0]))
}
