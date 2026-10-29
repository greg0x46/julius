package httpapi_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/greg0x46/julius/internal/categories"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/httpapi"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/scenarios"
)

// Require every execution read/write to share the serializable transaction.
// In particular, the approval recheck must not run before BeginTx.
type planningAtomicConnector struct {
	backend driver.Driver
	path    string
	begins  int
}

func (c *planningAtomicConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := c.backend.Open(c.path)
	if err != nil {
		return nil, err
	}
	if _, err = conn.(driver.ExecerContext).ExecContext(context.Background(), "PRAGMA foreign_keys = ON", nil); err != nil {
		conn.Close()
		return nil, err
	}
	return &planningAtomicConn{Conn: conn, owner: c}, nil
}

func (c *planningAtomicConnector) Driver() driver.Driver { return c.backend }

type planningAtomicConn struct {
	driver.Conn
	inTx  bool
	owner *planningAtomicConnector
}

func (c *planningAtomicConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if opts.Isolation != driver.IsolationLevel(sql.LevelSerializable) {
		return nil, errors.New("expected serializable transaction")
	}
	tx, err := c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	c.inTx = true
	c.owner.begins++
	return &planningAtomicTx{Tx: tx, conn: c}, nil
}

func (c *planningAtomicConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if !c.inTx {
		return nil, errors.New("planning read outside approval transaction")
	}
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
}

func (c *planningAtomicConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if !c.inTx {
		return nil, errors.New("planning write outside approval transaction")
	}
	return c.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
}

type planningAtomicTx struct {
	driver.Tx
	conn *planningAtomicConn
}

func (tx *planningAtomicTx) Commit() error {
	defer func() { tx.conn.inTx = false }()
	return tx.Tx.Commit()
}
func (tx *planningAtomicTx) Rollback() error {
	defer func() { tx.conn.inTx = false }()
	return tx.Tx.Rollback()
}

func TestMCPPlanningRecurrenceApprovalIsAtomic(t *testing.T) {
	for _, operation := range []string{"update_recurring_commitment", "delete_recurring_commitment"} {
		t.Run(operation, func(t *testing.T) {
			ctx := context.Background()
			conn := dbtest.OpenSQLite(t)
			cat, err := categories.Create(ctx, conn, "Synthetic rent", money.Expense, "coffee", "#2a78d6")
			if err != nil {
				t.Fatal(err)
			}
			data := map[string]any{"name": "Synthetic rent", "kind": "expense", "amount": "1500", "category_id": cat.ID, "cadence": "monthly", "day_of_month": 31, "start_date": "2026-01-01", "is_active": true}
			id := planningResultID(t, applyWrite(t, conn, writePreview(t, conn, "create_recurring_commitment", "", data)))
			if _, err := scenarios.RealizeEvent(ctx, conn, id, "scenario:"+id+":occurrence:2026-02-28", scenarios.RealizationWrite{State: "detached"}); err != nil {
				t.Fatal(err)
			}
			data["day_of_month"] = 15
			body := data
			if operation == "delete_recurring_commitment" {
				body = map[string]any{}
			}
			preview := writePreview(t, conn, operation, id, body)
			var seq int
			var name, path string
			if err := conn.QueryRowContext(ctx, "PRAGMA database_list").Scan(&seq, &name, &path); err != nil {
				t.Fatal(err)
			}
			interceptor := &planningAtomicConnector{backend: conn.Driver(), path: path}
			guarded := sql.OpenDB(interceptor)
			guarded.SetMaxOpenConns(1)
			defer guarded.Close()
			result, err := httpapi.ExecuteMCPWrite(ctx, guarded, preview)
			if err != nil || result.Status >= 400 {
				t.Fatalf("atomic execution: %+v, %v", result, err)
			}
			if interceptor.begins != 1 {
				t.Fatalf("expected one approval and mutation transaction, got %d", interceptor.begins)
			}
			links, err := scenarios.ListRealizations(ctx, conn, id)
			if err != nil || len(links) != 0 {
				t.Fatalf("orphan decisions: %+v, %v", links, err)
			}
		})
	}
}
