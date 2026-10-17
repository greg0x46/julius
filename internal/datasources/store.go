// Package datasources is the registry of provider connections: one row per
// Pluggy item the app syncs. Everything financial is already keyed by
// source_id, so a connection is the unit that owns accounts, transactions,
// investments and bills — this package is what lets there be more than one of
// them, and what a sync run resolves its item id from.
package datasources

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/greg0x46/julius/internal/db"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
)

// ProviderPluggy is the Open Banking provider: the connections users manage on
// the connections screen and that a sync run can refresh.
const ProviderPluggy = "pluggy"

// ProviderFile marks the local accounts a statement import creates. They own
// accounts and transactions like any connection but are never synced, and they
// are not something the connections screen manages.
const ProviderFile = "file"

var (
	// ErrNotFound is returned by Get/Update when an id has no matching row.
	ErrNotFound = errors.New("data source not found")
	// ErrDuplicate is returned by Create when the same provider already has a
	// connection for that external item id — re-adding an item would give its
	// accounts a second identity rather than a second connection.
	ErrDuplicate = errors.New("data source already registered")
)

// Querier is satisfied by both *sql.DB and *sql.Tx.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// DataSource is one connection.
type DataSource struct {
	ID             string
	Provider       string
	ExternalItemID string
	// DisplayName is the institution the provider reported on the last sync;
	// it is overwritten on every run, which is why Label exists separately.
	DisplayName *string
	// Label is the user's own name for the connection, and takes precedence
	// over DisplayName wherever a connection is named.
	Label     *string
	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Name is what to show for this connection: the user's label if they gave
// one, else the institution from the last sync, else the raw item id (which
// is all a connection added but never synced has).
func (d DataSource) Name() string {
	if d.Label != nil && *d.Label != "" {
		return *d.Label
	}
	if d.DisplayName != nil && *d.DisplayName != "" {
		return *d.DisplayName
	}
	return d.ExternalItemID
}

const selectColumns = `id, provider, external_item_id, display_name, label, is_active, created_at, updated_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func scan(row rowScanner) (DataSource, error) {
	var (
		d            DataSource
		isActive     int
		createdAtRaw string
		updatedAtRaw string
	)
	if err := row.Scan(dbtypes.ID(&d.ID), &d.Provider, dbtypes.ID(&d.ExternalItemID), &d.DisplayName, &d.Label,
		dbtypes.Bool(&isActive), dbtypes.Time(&createdAtRaw), dbtypes.Time(&updatedAtRaw)); err != nil {
		return DataSource{}, err
	}
	d.IsActive = isActive == 1
	var err error
	if d.CreatedAt, err = dbtypes.ParseTime(createdAtRaw); err != nil {
		return DataSource{}, err
	}
	if d.UpdatedAt, err = dbtypes.ParseTime(updatedAtRaw); err != nil {
		return DataSource{}, err
	}
	return d, nil
}

// List returns every data source of any provider, oldest first, so the one
// configured at setup stays at the top no matter how many are added later.
// Callers that present connections to the user want ListByProvider instead.
func List(ctx context.Context, q Querier) ([]DataSource, error) {
	return query(ctx, q, `SELECT `+selectColumns+` FROM data_sources ORDER BY created_at, id`)
}

// ListByProvider is List restricted to one provider, in the same order.
func ListByProvider(ctx context.Context, q Querier, provider string) ([]DataSource, error) {
	return query(ctx, q,
		`SELECT `+selectColumns+` FROM data_sources WHERE provider = ? ORDER BY created_at, id`, provider)
}

// ListActive returns the connections a "sync everything" should cover.
func ListActive(ctx context.Context, q Querier) ([]DataSource, error) {
	return query(ctx, q,
		`SELECT `+selectColumns+` FROM data_sources WHERE is_active = TRUE AND provider = 'pluggy' ORDER BY created_at, id`)
}

func query(ctx context.Context, q Querier, sqlText string, args ...any) ([]DataSource, error) {
	rows, err := q.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []DataSource{}
	for rows.Next() {
		d, err := scan(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

// Get returns one connection by id.
func Get(ctx context.Context, q Querier, id string) (DataSource, error) {
	d, err := scan(q.QueryRowContext(ctx, `SELECT `+selectColumns+` FROM data_sources WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return DataSource{}, ErrNotFound
	}
	return d, err
}

// Create registers a connection. A blank label is stored as NULL so Name
// falls through to the institution the first sync reports.
func Create(ctx context.Context, q Querier, provider, externalItemID string, label *string) (DataSource, error) {
	now := time.Now()
	d := DataSource{
		ID:             uuid.NewString(),
		Provider:       provider,
		ExternalItemID: externalItemID,
		Label:          normalizeLabel(label),
		IsActive:       true,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	_, err := q.ExecContext(ctx, `
		INSERT INTO data_sources (id, provider, external_item_id, label, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, TRUE, ?, ?)`,
		d.ID, d.Provider, d.ExternalItemID, d.Label, db.FormatTime(now), db.FormatTime(now))
	// Narrowed to the item-identity constraint on purpose: a primary-key
	// collision on the uuid we just generated is not something the user can
	// resolve by picking a different Item ID, so it must not be reported as
	// "esse Item ID já está cadastrado".
	if db.IsUniqueViolationOn(err, db.ConstraintDataSourceItemSQLite, db.ConstraintDataSourceItemPostgres) {
		return DataSource{}, ErrDuplicate
	}
	if err != nil {
		return DataSource{}, err
	}
	return d, nil
}

// Update applies whichever of label and isActive were supplied; nil means
// "leave as is", so a rename does not have to restate the active flag.
//
// Deactivating is how a connection is retired: its accounts, transactions and
// sync history stay readable and keep their foreign keys, only future syncs
// stop covering it. There is deliberately no delete.
//
// It is one conditional UPDATE rather than a read, a merge and a write: those
// three steps cannot be held together through a Querier (which is deliberately
// satisfied by both *sql.DB and *sql.Tx, so it cannot open a transaction of
// its own), and split apart they let a rename and a deactivation arriving
// together overwrite each other's field. Writing only the supplied columns
// makes "leave as is" mean it literally.
func Update(ctx context.Context, q Querier, id string, label *string, isActive *bool) (DataSource, error) {
	assignments := []string{"updated_at = ?"}
	args := []any{db.FormatTime(time.Now())}
	if label != nil {
		assignments = append(assignments, "label = ?")
		args = append(args, normalizeLabel(label))
	}
	if isActive != nil {
		active := *isActive
		assignments = append(assignments, "is_active = ?")
		args = append(args, active)
	}
	args = append(args, id)

	result, err := q.ExecContext(ctx,
		`UPDATE data_sources SET `+strings.Join(assignments, ", ")+` WHERE id = ?`, args...)
	if err != nil {
		return DataSource{}, err
	}
	// Both dialects count a matched row even when every value is unchanged,
	// so zero rows means the id is unknown rather than the update being a
	// no-op. A driver that cannot report it falls through to Get, which
	// answers ErrNotFound for the same case.
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return DataSource{}, ErrNotFound
	}
	return Get(ctx, q, id)
}

func normalizeLabel(label *string) *string {
	if label == nil || *label == "" {
		return nil
	}
	return label
}
