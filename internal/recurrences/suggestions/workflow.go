package suggestions

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/greg0x46/julius/internal/automation"
	"github.com/greg0x46/julius/internal/categories"
	"github.com/greg0x46/julius/internal/dates"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/recurrences"
	"github.com/greg0x46/julius/internal/rules"
	"github.com/greg0x46/julius/internal/transactions"
)

var (
	ErrStale     = errors.New("suggestion changed; discover and review again")
	ErrDuplicate = errors.New("equivalent recurring commitment already exists")
	ErrInvalid   = errors.New("select an active category and valid schedule and conditions")
	ErrReview    = errors.New("review ambiguous matches or unsupported conditions")
)

func digest(value any) string {
	data, _ := json.Marshal(value)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

// Key survives additional observations and amount changes for the same pattern.
func Key(s recurrences.Suggestion) string { return digest(recurrences.SuggestionPattern(s)) }

func List(ctx context.Context, q recurrences.Querier, now time.Time) ([]recurrences.Suggestion, error) {
	candidates, err := recurrences.Discover(ctx, q, now)
	if err != nil {
		return nil, err
	}
	rows, err := q.QueryContext(ctx, `SELECT pattern_key FROM recurrence_suggestion_decisions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hidden := map[string]bool{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		hidden[key] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]recurrences.Suggestion, 0, len(candidates))
	for _, candidate := range candidates {
		if !hidden[Key(candidate)] {
			result = append(result, candidate)
		}
	}
	return result, nil
}

type Review struct {
	Draft   Draft
	Preview PreviewResult
	History []transactions.Item
	Token   string
}

func ReviewCandidate(ctx context.Context, q recurrences.Querier, now time.Time, key string, write recurrences.Write, conditions []rules.Condition) (Review, error) {
	candidates, err := List(ctx, q, now)
	if err != nil {
		return Review{}, err
	}
	var candidate *recurrences.Suggestion
	for i := range candidates {
		if Key(candidates[i]) == key {
			candidate = &candidates[i]
			break
		}
	}
	if candidate == nil {
		return Review{}, ErrStale
	}
	if _, err := uuid.Parse(write.CategoryID); err != nil {
		return Review{}, ErrInvalid
	}
	if write.AccountID != nil {
		if _, err := uuid.Parse(*write.AccountID); err != nil {
			return Review{}, ErrInvalid
		}
	}
	category, err := categories.Get(ctx, q, write.CategoryID)
	if errors.Is(err, categories.ErrNotFound) {
		return Review{}, ErrInvalid
	}
	if err != nil {
		return Review{}, err
	}
	if !category.IsActive || category.Kind == money.Transfer {
		return Review{}, ErrInvalid
	}
	if write.AccountID != nil {
		var n int
		if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM financial_accounts WHERE id = ?`, *write.AccountID).Scan(&n); err != nil {
			return Review{}, err
		}
		if n != 1 {
			return Review{}, ErrInvalid
		}
	}
	history, err := transactions.History(ctx, q)
	if err != nil {
		return Review{}, err
	}
	sort.Slice(history, func(i, j int) bool { return history[i].ID < history[j].ID })
	target := uuid.NewSHA1(uuid.NameSpaceOID, []byte(key)).String()
	draft, err := FromSuggestion(*candidate, target, write.CategoryID, history)
	if err != nil {
		return Review{}, ErrInvalid
	}
	draft.Commitment = recurrences.RecurringCommitment{ID: target, Name: write.Name, Kind: write.Kind, Amount: write.Amount, CategoryID: write.CategoryID, AccountID: write.AccountID, Cadence: write.Cadence, DayOfMonth: write.DayOfMonth, MonthOfYear: write.MonthOfYear, StartDate: write.StartDate, EndDate: write.EndDate, IsActive: write.IsActive}
	if err := draft.Commitment.Validate(); err != nil {
		return Review{}, ErrInvalid
	}
	draft.Rule.Name, draft.Rule.IsActive = write.Name, true
	// Discovery approval never categorizes history or adds future category actions.
	draft.Rule.Actions = []automation.ActionWrite{{Type: automation.ActionReconcile, ScenarioID: &draft.Commitment.ID}}
	if conditions != nil {
		draft.Rule.Conditions = conditions
	}
	if err := draft.Rule.Validate(); err != nil {
		return Review{}, ErrInvalid
	}
	existing, err := recurrences.List(ctx, q)
	if err != nil {
		return Review{}, err
	}
	sort.Slice(existing, func(i, j int) bool { return existing[i].ID < existing[j].ID })
	proposed := *candidate
	proposed.Draft = draft.Commitment
	// An owner-selected category must not merge distinct merchants.
	proposed.Draft.CategoryID = ""
	if recurrences.HasCoveringCommitment(proposed, existing) {
		return Review{}, ErrDuplicate
	}

	existingRules, err := automation.List(ctx, q)
	if err != nil {
		return Review{}, err
	}
	sort.Slice(existingRules, func(i, j int) bool { return existingRules[i].ID < existingRules[j].ID })
	ids := make([]string, len(existing))
	for i, c := range existing {
		ids[i] = c.ID
	}
	overrides, err := recurrences.ListOverrides(ctx, q, ids, time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC))
	if err != nil {
		return Review{}, err
	}
	var flat []recurrences.Override
	for _, id := range ids {
		flat = append(flat, overrides[id]...)
	}
	from, to := draft.Evidence[0].OccurrenceDate, draft.Evidence[0].OccurrenceDate
	for _, o := range draft.Evidence {
		if o.OccurrenceDate.Before(from) {
			from = o.OccurrenceDate
		}
		if o.OccurrenceDate.After(to) {
			to = o.OccurrenceDate
		}
	}
	from = recurrences.MonthKey(from)
	to = time.Date(to.Year(), to.Month(), dates.DaysInMonth(to.Year(), to.Month()), 0, 0, 0, 0, time.UTC)
	previewHistory := make([]transactions.Item, 0)
	for _, item := range history {
		if item.OccurredAt != nil && !recurrences.MonthKey(*item.OccurredAt).Before(recurrences.MonthKey(from)) && !recurrences.MonthKey(*item.OccurredAt).After(recurrences.MonthKey(to)) {
			previewHistory = append(previewHistory, item)
		}
	}
	preview, err := Preview(PreviewRequest{Draft: draft, From: from, To: to, History: previewHistory, Rules: existingRules, Commitments: existing, Overrides: flat})
	if err != nil {
		return Review{}, ErrInvalid
	}
	// A wrapping day condition cannot relax the reconciler's month boundary.
	for _, c := range draft.Rule.Conditions {
		if c.Field == rules.FieldDayOfMonth {
			var first, last int
			if _, err := fmt.Sscanf(c.Value, "%d:%d", &first, &last); err == nil && first > last {
				preview.Warnings = appendUnique(preview.Warnings, "cross_month_unsupported")
				preview.NeedsReview = true
			}
		}
	}
	token := digest([]any{dates.Day(now), key, draft, history, preview})
	return Review{Draft: draft, Preview: preview, History: previewHistory, Token: token}, nil
}

// Accept commits one approved item, including the retry receipt, in one transaction.
func Accept(ctx context.Context, conn *sql.DB, now time.Time, key string, write recurrences.Write, conditions []rules.Condition, token string, acknowledge bool) (string, error) {
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var state string
	var id, saved sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT state, scenario_id, approval_token FROM recurrence_suggestion_decisions WHERE pattern_key = ?`, key).Scan(&state, &id, &saved)
	if err == nil {
		if state == "accepted" && saved.String == token && token != "" {
			return id.String, nil
		}
		return "", ErrStale
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	review, err := ReviewCandidate(ctx, tx, now, key, write, conditions)
	if err != nil {
		return "", err
	}
	if token == "" || review.Token != token {
		return "", ErrStale
	}
	if review.Preview.NeedsReview && !acknowledge {
		return "", ErrReview
	}
	for _, warning := range review.Preview.Warnings {
		if warning == "cross_month_unsupported" {
			return "", ErrReview
		}
	}
	c, err := recurrences.CreateWithQuerier(ctx, tx, write)
	if err != nil {
		return "", err
	}
	rule := review.Draft.Rule
	rule.Actions[0].ScenarioID = &c.ID
	if _, err := automation.CreateWithQuerier(ctx, tx, rule); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO recurrence_suggestion_decisions (pattern_key,state,scenario_id,approval_token) VALUES (?,'accepted',?,?)`, key, c.ID, token); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return c.ID, nil
}

func Dismiss(ctx context.Context, conn *sql.DB, now time.Time, key string) error {
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	candidates, err := List(ctx, tx, now)
	if err != nil {
		return err
	}
	found := false
	for _, c := range candidates {
		found = found || Key(c) == key
	}
	if !found {
		return ErrStale
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO recurrence_suggestion_decisions (pattern_key,state) VALUES (?,'dismissed')`, key); err != nil {
		return err
	}
	return tx.Commit()
}
