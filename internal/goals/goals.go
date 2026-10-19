// Package goals tracks a target against one global metric (today the market
// value of everything invested). A goal stores only its target; the current
// value and progress are recomputed on read from the engines that own the
// figure, so a goal never restates or persists money.
package goals

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/dates"
	"github.com/greg0x46/julius/internal/db"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/networth"
)

// Querier is satisfied by *sql.DB and *sql.Tx.
type Querier = networth.Querier

// Metric names the figure a goal measures. The schema already admits the
// metrics planned next; Input.validate only accepts those listed in metrics.
type Metric string

const (
	// MetricInvestedValue is the market value of every investment, gains and
	// losses included: the investments line of net worth.
	MetricInvestedValue Metric = "invested_value"
)

// figures holds the metric values read once per request, so every goal in it
// agrees. Only the investments are valued: an unrelated foreign-currency
// account without a rate must not make goals fail.
type figures struct {
	investedValue decimal.Decimal
}

var metrics = map[Metric]func(figures) decimal.Decimal{
	MetricInvestedValue: func(f figures) decimal.Decimal { return f.investedValue },
}

func readFigures(ctx context.Context, q Querier) (figures, error) {
	invested, err := networth.InvestmentValue(ctx, q)
	if err != nil {
		return figures{}, err
	}
	return figures{investedValue: invested}, nil
}

var (
	ErrNotFound     = errors.New("goal not found")
	ErrInvalidInput = errors.New("invalid goal input")
)

type Goal struct {
	ID           string
	Name         string
	Metric       Metric
	TargetAmount decimal.Decimal
	TargetDate   *time.Time
	Notes        *string
	// CurrentValue and Progress are computed on read, in BRL. Progress is
	// CurrentValue / TargetAmount and may exceed 1 once the goal is reached.
	CurrentValue decimal.Decimal
	Progress     decimal.Decimal
	// Projection is set when the goal has a future target date.
	MonthlyContribution *MonthlyContribution
	Projection          *Projection
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type Input struct {
	Name         string
	Metric       Metric
	TargetAmount decimal.Decimal
	TargetDate   *time.Time
	Notes        *string
}

func (in Input) validate() (string, error) {
	name := strings.TrimSpace(in.Name)
	if _, ok := metrics[in.Metric]; !ok || name == "" || !in.TargetAmount.IsPositive() {
		return "", ErrInvalidInput
	}
	return name, nil
}

const columns = `id, name, metric, target_amount, target_date, notes, created_at, updated_at`

func scan(row interface{ Scan(...any) error }) (Goal, error) {
	var (
		g                          Goal
		metric, targetAmount       string
		targetDate, notes          sql.NullString
		createdAtRaw, updatedAtRaw string
	)
	if err := row.Scan(dbtypes.ID(&g.ID), &g.Name, &metric, dbtypes.Decimal(&targetAmount), dbtypes.Date(&targetDate), &notes,
		dbtypes.Time(&createdAtRaw), dbtypes.Time(&updatedAtRaw)); err != nil {
		return Goal{}, err
	}
	g.Metric = Metric(metric)
	var err error
	if g.TargetAmount, err = decimal.NewFromString(targetAmount); err != nil {
		return Goal{}, err
	}
	if targetDate.Valid {
		day, err := dbtypes.ParseDate(targetDate.String)
		if err != nil {
			return Goal{}, err
		}
		g.TargetDate = &day
	}
	if notes.Valid {
		g.Notes = &notes.String
	}
	if g.CreatedAt, err = dbtypes.ParseTime(createdAtRaw); err != nil {
		return Goal{}, err
	}
	if g.UpdatedAt, err = dbtypes.ParseTime(updatedAtRaw); err != nil {
		return Goal{}, err
	}
	return g, nil
}

func nullable(s *string) any {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	return strings.TrimSpace(*s)
}

func nullableDay(t *time.Time) any {
	if t == nil {
		return nil
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).Format("2006-01-02")
}

// now is the clock of the projections; tests replace it.
var now = time.Now

// withComputed fills the computed fields from figures read once for all
// the goals of a request, so they agree with each other. The return the
// projections compound is measured once too, and only when a goal has a date.
func withComputed(ctx context.Context, q Querier, list []Goal) ([]Goal, error) {
	f, err := readFigures(ctx, q)
	if err != nil {
		return nil, err
	}
	for i := range list {
		list[i].CurrentValue = metrics[list[i].Metric](f)
		list[i].Progress = list[i].CurrentValue.Div(list[i].TargetAmount)
	}
	if !needsProjection(list) {
		return list, nil
	}
	today := dates.Day(now())
	est, err := estimateReturn(ctx, q, today)
	if err != nil {
		return nil, err
	}
	for i := range list {
		list[i].Projection = projectGoal(list[i], est, today)
	}
	if err := withMonthlyContribution(ctx, q, list, today); err != nil {
		return nil, err
	}
	return list, nil
}

func rawGet(ctx context.Context, q Querier, id string) (Goal, error) {
	g, err := scan(q.QueryRowContext(ctx, `SELECT `+columns+` FROM goals WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Goal{}, ErrNotFound
	}
	return g, err
}

func Get(ctx context.Context, q Querier, id string) (Goal, error) {
	g, err := rawGet(ctx, q, id)
	if err != nil {
		return Goal{}, err
	}
	list, err := withComputed(ctx, q, []Goal{g})
	if err != nil {
		return Goal{}, err
	}
	return list[0], nil
}

func List(ctx context.Context, q Querier) ([]Goal, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+columns+` FROM goals ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	goals := []Goal{}
	for rows.Next() {
		g, err := scan(rows)
		if err != nil {
			return nil, err
		}
		goals = append(goals, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(goals) == 0 {
		return goals, nil
	}
	return withComputed(ctx, q, goals)
}

func Create(ctx context.Context, conn *sql.DB, in Input) (Goal, error) {
	name, err := in.validate()
	if err != nil {
		return Goal{}, err
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return Goal{}, err
	}
	defer tx.Rollback()
	id := uuid.NewString()
	now := db.FormatTime(time.Now())
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO goals (id, name, metric, target_amount, target_date, notes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, name, string(in.Metric), money.CanonicalDecimal(in.TargetAmount), nullableDay(in.TargetDate), nullable(in.Notes), now, now); err != nil {
		return Goal{}, err
	}
	goal, err := Get(ctx, tx, id)
	if err != nil {
		return Goal{}, err
	}
	if err := tx.Commit(); err != nil {
		return Goal{}, err
	}
	return goal, nil
}

func Update(ctx context.Context, conn *sql.DB, id string, in Input) (Goal, error) {
	name, err := in.validate()
	if err != nil {
		return Goal{}, err
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return Goal{}, err
	}
	defer tx.Rollback()
	if _, err := rawGet(ctx, tx, id); err != nil {
		return Goal{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE goals SET name = ?, metric = ?, target_amount = ?, target_date = ?, notes = ?, updated_at = ?
		WHERE id = ?`,
		name, string(in.Metric), money.CanonicalDecimal(in.TargetAmount), nullableDay(in.TargetDate), nullable(in.Notes),
		db.FormatTime(time.Now()), id); err != nil {
		return Goal{}, err
	}
	goal, err := Get(ctx, tx, id)
	if err != nil {
		return Goal{}, err
	}
	if err := tx.Commit(); err != nil {
		return Goal{}, err
	}
	return goal, nil
}

func Delete(ctx context.Context, q Querier, id string) error {
	res, err := q.ExecContext(ctx, `DELETE FROM goals WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}
