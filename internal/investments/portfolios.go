package investments

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/exchange"
	"github.com/greg0x46/julius/internal/valuation"
)

const portfolioColumns = `id, name, target_amount, target_date, notes, created_at, updated_at`

func scanPortfolio(row interface{ Scan(...any) error }) (Portfolio, error) {
	var (
		p                                     Portfolio
		targetAmountRaw, targetDateRaw, notes sql.NullString
		createdAtRaw, updatedAtRaw            string
	)
	if err := row.Scan(dbtypes.ID(&p.ID), &p.Name, dbtypes.Decimal(&targetAmountRaw), dbtypes.Date(&targetDateRaw), &notes, dbtypes.Time(&createdAtRaw), dbtypes.Time(&updatedAtRaw)); err != nil {
		return Portfolio{}, err
	}
	if targetAmountRaw.Valid {
		amount, err := decimal.NewFromString(targetAmountRaw.String)
		if err != nil {
			return Portfolio{}, err
		}
		p.TargetAmount = &amount
	}
	if targetDateRaw.Valid {
		date, err := parseDate(targetDateRaw.String)
		if err != nil {
			return Portfolio{}, err
		}
		p.TargetDate = &date
	}
	if notes.Valid {
		v := notes.String
		p.Notes = &v
	}
	var err error
	if p.CreatedAt, err = parseTimestamp(createdAtRaw); err != nil {
		return Portfolio{}, err
	}
	if p.UpdatedAt, err = parseTimestamp(updatedAtRaw); err != nil {
		return Portfolio{}, err
	}
	return p, nil
}

func listRawPortfolios(ctx context.Context, q Querier) ([]Portfolio, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+portfolioColumns+` FROM investment_portfolios ORDER BY name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Portfolio{}
	for rows.Next() {
		p, err := scanPortfolio(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func getRawPortfolio(ctx context.Context, q Querier, id string) (Portfolio, error) {
	p, err := scanPortfolio(q.QueryRowContext(ctx, `SELECT `+portfolioColumns+` FROM investment_portfolios WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Portfolio{}, ErrPortfolioNotFound
	}
	return p, err
}

// portfolioValues groups holdings by goal. A goal only restates value that
// already belongs to a position, so nothing here may be added to a total.
func portfolioValues(positions []Position) map[string]decimal.Decimal {
	values := map[string]decimal.Decimal{}
	for _, position := range positions {
		if position.PortfolioID == nil {
			continue
		}
		values[*position.PortfolioID] = values[*position.PortfolioID].Add(position.CurrentValue)
	}
	return values
}

func applyPortfolioValue(p *Portfolio, value decimal.Decimal) {
	p.CurrentValue = value
	p.Progress = progress(value, p.TargetAmount)
}

// progress stays nil without a usable target: a goal with no amount has no
// completion to report, and zero would read as "nothing achieved".
func progress(value decimal.Decimal, target *decimal.Decimal) *decimal.Decimal {
	if target == nil || target.IsZero() {
		return nil
	}
	ratio := value.Div(*target)
	return &ratio
}

func ListPortfolios(ctx context.Context, q Querier) ([]Portfolio, error) {
	portfolios, err := listRawPortfolios(ctx, q)
	if err != nil {
		return nil, err
	}
	positions, err := ListPositions(ctx, q, PositionFilter{IncludeClosed: true})
	if err != nil {
		return nil, err
	}
	values, err := valuedPortfolioValues(ctx, q, positions)
	if err != nil {
		return nil, err
	}
	for i := range portfolios {
		applyPortfolioValue(&portfolios[i], values[portfolios[i].ID])
	}
	return portfolios, nil
}

func GetPortfolio(ctx context.Context, q Querier, id string) (Portfolio, error) {
	portfolio, err := getRawPortfolio(ctx, q, id)
	if err != nil {
		return Portfolio{}, err
	}
	positions, err := ListPositions(ctx, q, PositionFilter{PortfolioID: &id, IncludeClosed: true})
	if err != nil {
		return Portfolio{}, err
	}
	values, err := valuedPortfolioValues(ctx, q, positions)
	if err != nil {
		return Portfolio{}, err
	}
	applyPortfolioValue(&portfolio, values[id])
	return portfolio, nil
}

func validatePortfolioInput(in PortfolioInput) (string, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || (in.TargetAmount != nil && in.TargetAmount.IsNegative()) {
		return "", ErrInvalidInput
	}
	return name, nil
}

func CreatePortfolio(ctx context.Context, q Querier, in PortfolioInput) (Portfolio, error) {
	name, err := validatePortfolioInput(in)
	if err != nil {
		return Portfolio{}, err
	}
	id := uuid.NewString()
	now := db.FormatTime(time.Now())
	if _, err := q.ExecContext(ctx, `
		INSERT INTO investment_portfolios (id, name, target_amount, target_date, notes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, name, nullableDecimal(in.TargetAmount), nullableDate(in.TargetDate),
		nullableString(trimOptional(in.Notes)), now, now); err != nil {
		return Portfolio{}, err
	}
	return GetPortfolio(ctx, q, id)
}

func UpdatePortfolio(ctx context.Context, q Querier, id string, in PortfolioInput) (Portfolio, error) {
	name, err := validatePortfolioInput(in)
	if err != nil {
		return Portfolio{}, err
	}
	if _, err := getRawPortfolio(ctx, q, id); err != nil {
		return Portfolio{}, err
	}
	if _, err := q.ExecContext(ctx, `
		UPDATE investment_portfolios SET name = ?, target_amount = ?, target_date = ?, notes = ?, updated_at = ?
		WHERE id = ?`,
		name, nullableDecimal(in.TargetAmount), nullableDate(in.TargetDate),
		nullableString(trimOptional(in.Notes)), db.FormatTime(time.Now()), id); err != nil {
		return Portfolio{}, err
	}
	return GetPortfolio(ctx, q, id)
}

// DeletePortfolio drops the goal and, through the cascade, only its
// annotations. The holdings it grouped keep their value and their operations.
func DeletePortfolio(ctx context.Context, q Querier, id string) error {
	if _, err := getRawPortfolio(ctx, q, id); err != nil {
		return err
	}
	_, err := q.ExecContext(ctx, `DELETE FROM investment_portfolios WHERE id = ?`, id)
	return err
}

// valuedPortfolioValues groups holdings by goal in BRL, valuing foreign
// holdings at today's rate.
func valuedPortfolioValues(ctx context.Context, q Querier, positions []Position) (map[string]decimal.Decimal, error) {
	valued := valuation.New(exchange.New(q, nil), time.Now())
	converted := make([]Position, 0, len(positions))
	for _, position := range positions {
		if position.PortfolioID == nil {
			continue
		}
		value, err := valued.Convert(ctx, position.CurrentValue, valuation.Currency(position.CurrencyCode))
		if err != nil {
			return nil, err
		}
		position.CurrentValue = value
		converted = append(converted, position)
	}
	return portfolioValues(converted), nil
}
