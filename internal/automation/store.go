package automation

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/greg0x46/julius/internal/db"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
)

// ErrNotFound is returned by Get/Update/SetActive/Delete when id has no
// matching row.
var ErrNotFound = errors.New("automation rule not found")

// ErrInvalidActionTarget is returned by Create/Update when an action's
// target doesn't reference an existing row — a reconcile action's ScenarioID
// or a set_category action's CategoryID.
var ErrInvalidActionTarget = errors.New("automation action target not found")

// Rule mirrors AutomationRule (with its conditions and actions eager-loaded,
// as the reference always fetches them together).
type Rule struct {
	ID            string
	Name          string
	IsActive      bool
	LogicOperator LogicOperator
	Conditions    []Condition
	Actions       []Action
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Write mirrors RuleWrite: the fields a create/update request supplies.
type Write struct {
	Name          string
	IsActive      bool
	LogicOperator LogicOperator
	Conditions    []Condition
	Actions       []ActionWrite
}

// Querier is satisfied by both *sql.DB and *sql.Tx.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func insertConditions(ctx context.Context, q Querier, ruleID string, conditions []Condition) error {
	for i, c := range conditions {
		if _, err := q.ExecContext(ctx, `
			INSERT INTO automation_rule_conditions (id, rule_id, field, operator, value, position)
			VALUES (?, ?, ?, ?, ?, ?)`,
			uuid.NewString(), ruleID, string(c.Field), string(c.Operator), c.Value, i,
		); err != nil {
			return err
		}
	}
	return nil
}

func loadConditions(ctx context.Context, q Querier, ruleID string) ([]Condition, error) {
	byRule, err := loadConditionsFor(ctx, q, []string{ruleID})
	if err != nil {
		return nil, err
	}
	return byRule[ruleID], nil
}

// loadConditionsFor reads the conditions of any number of rules in one query,
// keyed by rule id and ordered by position within each key. Readers that walk
// a whole rule set at once — ListActiveReconcileTargets, which the Timeline
// calls on every projection — would otherwise pay a round trip per rule. A
// rule with no conditions is simply absent from the map.
func loadConditionsFor(ctx context.Context, q Querier, ruleIDs []string) (map[string][]Condition, error) {
	result := make(map[string][]Condition, len(ruleIDs))
	if len(ruleIDs) == 0 {
		return result, nil
	}
	in, args := db.InClause(ruleIDs)
	rows, err := q.QueryContext(ctx,
		`SELECT rule_id, field, operator, value FROM automation_rule_conditions
		 WHERE rule_id IN (`+in+`) ORDER BY rule_id, position`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			ruleID string
			c      Condition
		)
		if err := rows.Scan(dbtypes.ID(&ruleID), &c.Field, &c.Operator, &c.Value); err != nil {
			return nil, err
		}
		result[ruleID] = append(result[ruleID], c)
	}
	return result, rows.Err()
}

func insertActions(ctx context.Context, q Querier, ruleID string, actions []ActionWrite) error {
	for i, a := range actions {
		var scenarioID *string
		if a.Type == ActionReconcile {
			var err error
			scenarioID, err = normalizeReconcileTarget(ctx, q, a)
			if err != nil {
				return err
			}
		}
		if _, err := q.ExecContext(ctx, `
			INSERT INTO automation_rule_actions (id, rule_id, action_type, scenario_id, category_id, position)
			VALUES (?, ?, ?, ?, ?, ?)`,
			uuid.NewString(), ruleID, string(a.Type), scenarioID, a.CategoryID, i,
		); err != nil {
			return err
		}
	}
	return nil
}

func loadActions(ctx context.Context, q Querier, ruleID string) ([]Action, error) {
	byRule, err := loadActionsFor(ctx, q, []string{ruleID})
	if err != nil {
		return nil, err
	}
	return byRule[ruleID], nil
}

// loadActionsFor is the batched form, for the same reason loadConditionsFor
// is: ListActive runs on every synced transaction, and one round trip per
// rule there is a cost paid per rule per sync.
func loadActionsFor(ctx context.Context, q Querier, ruleIDs []string) (map[string][]Action, error) {
	result := make(map[string][]Action, len(ruleIDs))
	if len(ruleIDs) == 0 {
		return result, nil
	}
	in, args := db.InClause(ruleIDs)
	rows, err := q.QueryContext(ctx,
		`SELECT rule_id, id, action_type, scenario_id, category_id FROM automation_rule_actions
		 WHERE rule_id IN (`+in+`) ORDER BY rule_id, position`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			ruleID     string
			a          Action
			actionType string
		)
		if err := rows.Scan(dbtypes.ID(&ruleID), dbtypes.ID(&a.ID), &actionType, dbtypes.ID(&a.ScenarioID), dbtypes.ID(&a.CategoryID)); err != nil {
			return nil, err
		}
		a.Type = ActionType(actionType)
		result[ruleID] = append(result[ruleID], a)
	}
	return result, rows.Err()
}

// normalizeReconcileTarget validates that a reconcile action points at a
// recurring Scenario, which is the only thing the resolver can key on.
func normalizeReconcileTarget(ctx context.Context, q Querier, action ActionWrite) (*string, error) {
	if action.ScenarioID == nil || strings.TrimSpace(*action.ScenarioID) == "" {
		return nil, ErrInvalidActionTarget
	}
	scenarioID := strings.TrimSpace(*action.ScenarioID)
	var kind string
	if err := q.QueryRowContext(ctx, `SELECT kind FROM scenarios WHERE id = ?`, scenarioID).Scan(&kind); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidActionTarget
		}
		return nil, err
	}
	if kind != "recurring" {
		return nil, ErrInvalidActionTarget
	}
	return &scenarioID, nil
}

// Create mirrors create_rule.
func Create(ctx context.Context, conn *sql.DB, write Write) (Rule, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return Rule{}, err
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	id := uuid.NewString()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO automation_rules (id, name, is_active, logic_operator, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		id, write.Name, write.IsActive, string(write.LogicOperator), db.FormatTime(now), db.FormatTime(now),
	)
	if err != nil {
		return Rule{}, err
	}
	if err := insertConditions(ctx, tx, id, write.Conditions); err != nil {
		return Rule{}, err
	}
	if err := insertActions(ctx, tx, id, write.Actions); err != nil {
		if db.IsForeignKeyViolation(err) {
			return Rule{}, ErrInvalidActionTarget
		}
		return Rule{}, err
	}
	if err := tx.Commit(); err != nil {
		return Rule{}, err
	}
	return Get(ctx, conn, id)
}

// Update mirrors update_rule: it replaces every condition rather than
// diffing them, same as the reference's clear-then-reassign.
func Update(ctx context.Context, conn *sql.DB, id string, write Write) (Rule, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return Rule{}, err
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	res, err := tx.ExecContext(ctx, `
		UPDATE automation_rules SET name = ?, is_active = ?, logic_operator = ?, updated_at = ? WHERE id = ?`,
		write.Name, write.IsActive, string(write.LogicOperator), db.FormatTime(now), id,
	)
	if err != nil {
		return Rule{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Rule{}, ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM automation_rule_conditions WHERE rule_id = ?`, id); err != nil {
		return Rule{}, err
	}
	if err := insertConditions(ctx, tx, id, write.Conditions); err != nil {
		return Rule{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM automation_rule_actions WHERE rule_id = ?`, id); err != nil {
		return Rule{}, err
	}
	if err := insertActions(ctx, tx, id, write.Actions); err != nil {
		if db.IsForeignKeyViolation(err) {
			return Rule{}, ErrInvalidActionTarget
		}
		return Rule{}, err
	}

	if err := tx.Commit(); err != nil {
		return Rule{}, err
	}
	return Get(ctx, conn, id)
}

// SetActive mirrors set_rule_active.
func SetActive(ctx context.Context, conn *sql.DB, id string, isActive bool) (Rule, error) {
	now := db.FormatTime(time.Now())
	res, err := conn.ExecContext(ctx, `UPDATE automation_rules SET is_active = ?, updated_at = ? WHERE id = ?`,
		isActive, now, id)
	if err != nil {
		return Rule{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Rule{}, ErrNotFound
	}
	return Get(ctx, conn, id)
}

// Delete mirrors delete_rule (automation_rule_conditions cascades via the
// schema's ON DELETE CASCADE).
func Delete(ctx context.Context, conn *sql.DB, id string) error {
	res, err := conn.ExecContext(ctx, `DELETE FROM automation_rules WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanRule(row *sql.Row) (Rule, error) {
	var (
		r            Rule
		isActive     int
		createdAtRaw string
		updatedAtRaw string
	)
	if err := row.Scan(dbtypes.ID(&r.ID), &r.Name, dbtypes.Bool(&isActive), &r.LogicOperator, dbtypes.Time(&createdAtRaw), dbtypes.Time(&updatedAtRaw)); err != nil {
		return Rule{}, err
	}
	r.IsActive = isActive != 0
	var err error
	if r.CreatedAt, err = dbtypes.ParseTime(createdAtRaw); err != nil {
		return Rule{}, err
	}
	if r.UpdatedAt, err = dbtypes.ParseTime(updatedAtRaw); err != nil {
		return Rule{}, err
	}
	return r, nil
}

// Get mirrors get_rule.
func Get(ctx context.Context, q Querier, id string) (Rule, error) {
	row := q.QueryRowContext(ctx, `SELECT id, name, is_active, logic_operator, created_at, updated_at FROM automation_rules WHERE id = ?`, id)
	r, err := scanRule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Rule{}, ErrNotFound
	}
	if err != nil {
		return Rule{}, err
	}
	if r.Conditions, err = loadConditions(ctx, q, id); err != nil {
		return Rule{}, err
	}
	if r.Actions, err = loadActions(ctx, q, id); err != nil {
		return Rule{}, err
	}
	return r, nil
}

func listWhere(ctx context.Context, q Querier, where string) ([]Rule, error) {
	query := `SELECT id, name, is_active, logic_operator, created_at, updated_at FROM automation_rules`
	if where != "" {
		query += " WHERE " + where
	}
	query += " ORDER BY created_at"
	rows, err := q.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []Rule
	for rows.Next() {
		var (
			r            Rule
			isActive     int
			createdAtRaw string
			updatedAtRaw string
		)
		if err := rows.Scan(dbtypes.ID(&r.ID), &r.Name, dbtypes.Bool(&isActive), &r.LogicOperator, dbtypes.Time(&createdAtRaw), dbtypes.Time(&updatedAtRaw)); err != nil {
			return nil, err
		}
		r.IsActive = isActive != 0
		if r.CreatedAt, err = dbtypes.ParseTime(createdAtRaw); err != nil {
			return nil, err
		}
		if r.UpdatedAt, err = dbtypes.ParseTime(updatedAtRaw); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	ruleIDs := make([]string, 0, len(rules))
	for i := range rules {
		ruleIDs = append(ruleIDs, rules[i].ID)
	}
	conditionsByRule, err := loadConditionsFor(ctx, q, ruleIDs)
	if err != nil {
		return nil, err
	}
	actionsByRule, err := loadActionsFor(ctx, q, ruleIDs)
	if err != nil {
		return nil, err
	}
	for i := range rules {
		rules[i].Conditions = conditionsByRule[rules[i].ID]
		rules[i].Actions = actionsByRule[rules[i].ID]
	}
	return rules, nil
}

// List mirrors list_rules.
func List(ctx context.Context, q Querier) ([]Rule, error) {
	return listWhere(ctx, q, "")
}

// ListActive mirrors load_active_rules.
func ListActive(ctx context.Context, q Querier) ([]Rule, error) {
	return listWhere(ctx, q, "is_active = TRUE")
}

// ListActiveReconcileTargets returns, for every active rule whose action is
// 'reconcile', the rule keyed by the recurring Scenario it targets. The
// database's partial unique index on automation_rule_actions guarantees at
// most one entry per scenario. internal/projections uses this to look up
// which rule's conditions (if any) resolve a given scenario's occurrences.
func ListActiveReconcileTargets(ctx context.Context, q Querier) (map[string]Rule, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT r.id, r.name, r.is_active, r.logic_operator, r.created_at, r.updated_at,
		       a.scenario_id
		FROM automation_rules r
		JOIN automation_rule_actions a ON a.rule_id = r.id
		WHERE r.is_active = TRUE AND a.action_type = 'reconcile'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	targets := make(map[string]Rule)
	for rows.Next() {
		var (
			r            Rule
			isActive     int
			createdAtRaw string
			updatedAtRaw string
			scenarioID   sql.NullString
		)
		if err := rows.Scan(dbtypes.ID(&r.ID), &r.Name, dbtypes.Bool(&isActive), &r.LogicOperator, dbtypes.Time(&createdAtRaw), dbtypes.Time(&updatedAtRaw), dbtypes.ID(&scenarioID)); err != nil {
			return nil, err
		}
		r.IsActive = isActive != 0
		if r.CreatedAt, err = dbtypes.ParseTime(createdAtRaw); err != nil {
			return nil, err
		}
		if r.UpdatedAt, err = dbtypes.ParseTime(updatedAtRaw); err != nil {
			return nil, err
		}
		if scenarioID.Valid {
			targets[scenarioID.String] = r
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	ruleIDs := make([]string, 0, len(targets))
	for _, r := range targets {
		ruleIDs = append(ruleIDs, r.ID)
	}
	conditionsByRule, err := loadConditionsFor(ctx, q, ruleIDs)
	if err != nil {
		return nil, err
	}
	for targetID, r := range targets {
		r.Conditions = conditionsByRule[r.ID]
		targets[targetID] = r
	}
	return targets, nil
}

// ListConditionOptions mirrors list_condition_options: the distinct
// account labels and card numbers available across existing data, to
// populate the rule-builder UI's autocomplete.
func ListConditionOptions(ctx context.Context, q Querier) (accounts []string, cards []string, err error) {
	rows, err := q.QueryContext(ctx, `SELECT name, institution FROM financial_accounts ORDER BY name, institution`)
	if err != nil {
		return nil, nil, err
	}
	accountSet := make(map[string]bool)
	for rows.Next() {
		var name, institution sql.NullString
		if err := rows.Scan(&name, &institution); err != nil {
			rows.Close()
			return nil, nil, err
		}
		label := strings.TrimSpace(name.String)
		if label == "" {
			label = strings.TrimSpace(institution.String)
		}
		if label != "" {
			accountSet[label] = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, err
	}
	rows.Close()

	rows, err = q.QueryContext(ctx, `SELECT credit_card_metadata FROM financial_transactions WHERE credit_card_metadata IS NOT NULL`)
	if err != nil {
		return nil, nil, err
	}
	cardSet := make(map[string]bool)
	for rows.Next() {
		var metadata string
		if err := rows.Scan(&metadata); err != nil {
			rows.Close()
			return nil, nil, err
		}
		if card := extractCardNumber(metadata); card != "" {
			cardSet[card] = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, err
	}
	rows.Close()

	accounts = setToSortedSlice(accountSet)
	cards = setToSortedSlice(cardSet)
	return accounts, cards, nil
}

func setToSortedSlice(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}
