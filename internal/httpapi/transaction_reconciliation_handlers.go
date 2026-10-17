package httpapi

import (
	"database/sql"
	"net/http"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/automation"
	"github.com/greg0x46/julius/internal/dates"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/recurrences"
	"github.com/greg0x46/julius/internal/transactions"
)

// transactionOptionMonths is how far either side of a transaction's own
// month occurrences are offered. One month of slack on each side is what
// makes the December-salary-paid-in-November case reachable without burying
// the obvious pick under a year of rows.
const transactionOptionMonths = 1

type reconciliationOptionDTO struct {
	CommitmentID   string `json:"commitment_id"`
	CommitmentName string `json:"commitment_name"`
	Kind           string `json:"kind"`
	OccurrenceDate string `json:"occurrence_date"`
	ExpectedAmount string `json:"expected_amount"`
}

type currentReconciliationDTO struct {
	reconciliationOptionDTO
	Origin string `json:"origin"`
}

// transactionReconciliationDTO answers "what is this transaction reconciling,
// and what could it reconcile" in one round trip, because the drawer needs
// both at once and the server computes them in the same pass anyway.
type transactionReconciliationDTO struct {
	Current *currentReconciliationDTO `json:"current"`
	Options []reconciliationOptionDTO `json:"options"`
}

func optionDTO(commitment recurrences.RecurringCommitment, resolved recurrences.Reconciliation) reconciliationOptionDTO {
	return reconciliationOptionDTO{
		CommitmentID:   commitment.ID,
		CommitmentName: commitment.Name,
		Kind:           string(commitment.Kind),
		OccurrenceDate: resolved.Occurrence.Date.Format(dateOnlyLayout),
		ExpectedAmount: money.CanonicalDecimal(resolved.Occurrence.ExpectedAmount),
	}
}

// handleGetTransactionReconciliation reports the occurrence this transaction
// currently settles — whether the user picked it or an automation rule
// matched it — plus the occurrences it could be moved to.
//
// Only commitments whose direction matches the transaction's flow are
// considered: an outflow can only ever settle an expense. Note this endpoint
// only reads; both writes go through the occurrence-scoped PUT/DELETE, so
// there is exactly one place that decides what a valid reconciliation is.
func handleGetTransactionReconciliation(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		item, found, err := transactions.GetItem(ctx, conn, r.PathValue("id"))
		if err != nil {
			recurrenceReconciliationUnavailableProblem(w)
			return
		}
		if !found {
			writeProblem(w, 404, "transaction-not-found", "Transação não encontrada", "")
			return
		}

		response := transactionReconciliationDTO{Options: []reconciliationOptionDTO{}}
		if item.OccurredAt == nil {
			// Without a date there is no occurrence window to search, and
			// nothing could have matched it either.
			writeJSON(w, http.StatusOK, response)
			return
		}
		if !item.TotalsEligibility.Included {
			// A transaction excluded from the totals carries no money the app
			// counts, so it can settle nothing — and realItemsIn drops it from
			// the candidate pool, so nothing could have matched it either.
			// Offering options here would only lead to a 422 on the write.
			writeJSON(w, http.StatusOK, response)
			return
		}

		occurredOn := dates.Day(*item.OccurredAt)
		from := time.Date(occurredOn.Year(), occurredOn.Month(), 1, 0, 0, 0, 0, time.UTC).
			AddDate(0, -transactionOptionMonths, 0)
		lastMonth := time.Date(occurredOn.Year(), occurredOn.Month(), 1, 0, 0, 0, 0, time.UTC).
			AddDate(0, transactionOptionMonths, 0)
		to := time.Date(lastMonth.Year(), lastMonth.Month(),
			dates.DaysInMonth(lastMonth.Year(), lastMonth.Month()), 0, 0, 0, 0, time.UTC)

		commitments, err := recurrences.ListActive(ctx, conn)
		if err != nil {
			recurrenceReconciliationUnavailableProblem(w)
			return
		}
		targets, err := automation.ListActiveReconcileTargets(ctx, conn)
		if err != nil {
			recurrenceReconciliationUnavailableProblem(w)
			return
		}
		scenarioIDs := make([]string, len(commitments))
		for i, commitment := range commitments {
			scenarioIDs[i] = commitment.ID
		}
		// Same widening the Timeline and reconcilerFor apply: a link on an
		// occurrence just outside the window still decides whether the rule
		// may reuse its transaction.
		overrides, err := recurrences.ListOverrides(ctx, conn, scenarioIDs,
			from.AddDate(0, 0, -candidateWindowDays), to.AddDate(0, 0, candidateWindowDays))
		if err != nil {
			recurrenceReconciliationUnavailableProblem(w)
			return
		}
		candidates, err := realItemsIn(ctx, conn,
			from.AddDate(0, 0, -candidateWindowDays), to.AddDate(0, 0, candidateWindowDays))
		if err != nil {
			recurrenceReconciliationUnavailableProblem(w)
			return
		}

		expectedFlow := money.Outflow
		if item.Classification == money.Inflow {
			expectedFlow = money.Inflow
		}

		for _, commitment := range commitments {
			wantsInflow := commitment.Kind == recurrences.KindIncome
			if (expectedFlow == money.Inflow) != wantsInflow {
				continue
			}
			rule, hasRule := targets[commitment.ID]
			reconciler := recurrences.NewReconciler(commitment, rule.Conditions, rule.LogicOperator, hasRule,
				overrides[commitment.ID], candidates)
			for _, resolved := range reconciler.ResolveRange(from, to) {
				if resolved.TransactionID != nil && *resolved.TransactionID == item.ID {
					current := currentReconciliationDTO{
						reconciliationOptionDTO: optionDTO(commitment, resolved),
						Origin:                  string(resolved.Origin),
					}
					response.Current = &current
					continue
				}
				if resolved.Reconciled() {
					continue
				}
				response.Options = append(response.Options, optionDTO(commitment, resolved))
			}
		}

		sortOptions(response.Options, occurredOn, item)
		writeJSON(w, http.StatusOK, response)
	}
}

// sortOptions ranks occurrences the same way sortCandidates ranks
// transactions, from the other side of the same pairing: nearest date first,
// then nearest expected amount.
func sortOptions(options []reconciliationOptionDTO, occurredOn time.Time, item transactions.Item) {
	actual := decimal.Zero
	if item.EffectiveMoney != nil {
		if parsed, err := decimal.NewFromString(item.EffectiveMoney.Value); err == nil {
			actual = parsed.Abs()
		}
	}
	distance := func(option reconciliationOptionDTO) time.Duration {
		date, err := time.Parse(dateOnlyLayout, option.OccurrenceDate)
		if err != nil {
			return 1 << 62
		}
		delta := date.Sub(occurredOn)
		if delta < 0 {
			return -delta
		}
		return delta
	}
	gap := func(option reconciliationOptionDTO) decimal.Decimal {
		expected, err := decimal.NewFromString(option.ExpectedAmount)
		if err != nil {
			return decimal.NewFromInt(1 << 30)
		}
		return expected.Sub(actual).Abs()
	}
	sort.SliceStable(options, func(i, j int) bool {
		di, dj := distance(options[i]), distance(options[j])
		if di != dj {
			return di < dj
		}
		return gap(options[i]).LessThan(gap(options[j]))
	})
}
