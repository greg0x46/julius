// Package categories ports app/categories/service.py (the internal category
// catalog) and the categorization half of app/transactions/categorization.py
// (deciding, and recording the history of, which category a transaction
// belongs to).
package categories

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/greg0x46/julius/internal/db"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/money"
)

// ErrNotFound is returned when a category id has no matching row.
var ErrNotFound = errors.New("category not found")

// Category is the internal catalog entry a transaction can be filed under.
type Category struct {
	ID        string
	Name      string
	Kind      money.CategoryKind
	IsActive  bool
	Icon      string
	Color     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Querier is satisfied by both *sql.DB and *sql.Tx, letting callers control
// transaction boundaries (needed by the decision-writing functions in
// decisions.go, which must keep a decision row and its append-only event row
// in the same transaction).
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func scanCategory(row *sql.Row) (Category, error) {
	var (
		c            Category
		isActive     int
		createdAtRaw string
		updatedAtRaw string
	)
	if err := row.Scan(dbtypes.ID(&c.ID), &c.Name, &c.Kind, dbtypes.Bool(&isActive), &c.Icon, &c.Color, dbtypes.Time(&createdAtRaw), dbtypes.Time(&updatedAtRaw)); err != nil {
		return Category{}, err
	}
	c.IsActive = isActive != 0
	createdAt, err := dbtypes.ParseTime(createdAtRaw)
	if err != nil {
		return Category{}, err
	}
	updatedAt, err := dbtypes.ParseTime(updatedAtRaw)
	if err != nil {
		return Category{}, err
	}
	c.CreatedAt = createdAt
	c.UpdatedAt = updatedAt
	return c, nil
}

// Create adds a new, active category to the catalog.
func Create(ctx context.Context, q Querier, name string, kind money.CategoryKind, icon string, color string) (Category, error) {
	now := time.Now().UTC()
	c := Category{ID: uuid.NewString(), Name: name, Kind: kind, IsActive: true, Icon: icon, Color: color, CreatedAt: now, UpdatedAt: now}
	_, err := q.ExecContext(ctx,
		`INSERT INTO categories (id, name, kind, is_active, icon, color, created_at, updated_at)
		 VALUES (?, ?, ?, 1, ?, ?, ?, ?)`,
		c.ID, c.Name, string(c.Kind), c.Icon, c.Color, db.FormatTime(now), db.FormatTime(now),
	)
	if err != nil {
		return Category{}, err
	}
	return c, nil
}

// Update mirrors CategoryUpdateInput: name, isActive, icon and color are
// applied only when non-nil, so a caller can change just one of them.
func Update(ctx context.Context, q Querier, id string, name *string, isActive *bool, icon *string, color *string) (Category, error) {
	existing, err := Get(ctx, q, id)
	if err != nil {
		return Category{}, err
	}
	if name != nil {
		existing.Name = *name
	}
	if isActive != nil {
		existing.IsActive = *isActive
	}
	if icon != nil {
		existing.Icon = *icon
	}
	if color != nil {
		existing.Color = *color
	}
	existing.UpdatedAt = time.Now().UTC()

	activeInt := 0
	if existing.IsActive {
		activeInt = 1
	}
	_, err = q.ExecContext(ctx,
		`UPDATE categories SET name = ?, is_active = ?, icon = ?, color = ?, updated_at = ? WHERE id = ?`,
		existing.Name, activeInt, existing.Icon, existing.Color, db.FormatTime(existing.UpdatedAt), id,
	)
	if err != nil {
		return Category{}, err
	}
	return existing, nil
}

// Get returns ErrNotFound when id has no matching row.
func Get(ctx context.Context, q Querier, id string) (Category, error) {
	row := q.QueryRowContext(ctx,
		`SELECT id, name, kind, is_active, icon, color, created_at, updated_at FROM categories WHERE id = ?`, id,
	)
	c, err := scanCategory(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Category{}, ErrNotFound
	}
	if err != nil {
		return Category{}, err
	}
	return c, nil
}

// List returns every category (active or not), ordered by kind then name to
// match the reference API's grouping in filter dropdowns.
func List(ctx context.Context, q Querier) ([]Category, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, name, kind, is_active, icon, color, created_at, updated_at FROM categories ORDER BY kind, name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Category
	for rows.Next() {
		var (
			c            Category
			isActive     int
			createdAtRaw string
			updatedAtRaw string
		)
		if err := rows.Scan(dbtypes.ID(&c.ID), &c.Name, &c.Kind, dbtypes.Bool(&isActive), &c.Icon, &c.Color, dbtypes.Time(&createdAtRaw), dbtypes.Time(&updatedAtRaw)); err != nil {
			return nil, err
		}
		c.IsActive = isActive != 0
		if c.CreatedAt, err = dbtypes.ParseTime(createdAtRaw); err != nil {
			return nil, err
		}
		if c.UpdatedAt, err = dbtypes.ParseTime(updatedAtRaw); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}
