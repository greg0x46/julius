// Command migrate-legacy converts a final legacy SQLite database once at cutover.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/transactions"
	"github.com/shopspring/decimal"
)

func main() {
	source := flag.String("sqlite", "", "fully migrated legacy SQLite source (read-only)")
	target := flag.String("target", "", "fresh SQLite path or postgres:// DSN")
	dry := flag.Bool("dry-run", false, "report all row errors and roll back all imported data")
	flag.Parse()
	if *source == "" || *target == "" {
		log.Fatal("-sqlite and -target are required")
	}
	src, err := openSource(*source)
	if err != nil {
		log.Fatal(err)
	}
	defer src.Close()
	dst, err := db.Open(*target)
	if err != nil {
		log.Fatal(err)
	}
	defer dst.Close()
	if err := migrateLegacy(src, dst, *dry, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func openSource(path string) (*sql.DB, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: absolute, RawQuery: "mode=ro"}
	conn, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func quote(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }

var skipped = map[string]bool{
	"goose_db_version":                  true,
	"scenario_transaction_realizations": true, "recurrence_reconciliations": true,
	"recurring_commitments": true, "recurring_commitment_conditions": true,
	"recurring_commitment_scenario_map": true, "debts": true, "receivables": true,
	"debt_transaction_links": true, "receivable_transaction_links": true,
}

func orderedTables(conn *sql.DB) ([]string, error) {
	rows, err := conn.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	var all []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return nil, err
		}
		all = append(all, name)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	deps := map[string][]string{}
	for _, table := range all {
		rows, err := conn.Query("PRAGMA foreign_key_list(" + quote(table) + ")")
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, seq int
			var ref, from, to, update, delete, match string
			if err := rows.Scan(&id, &seq, &ref, &from, &to, &update, &delete, &match); err != nil {
				rows.Close()
				return nil, err
			}
			if ref != table {
				deps[table] = append(deps[table], ref)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	state := map[string]int{}
	var result []string
	var visit func(string) error
	visit = func(table string) error {
		if state[table] == 1 {
			return fmt.Errorf("foreign-key cycle at %s", table)
		}
		if state[table] == 2 {
			return nil
		}
		state[table] = 1
		for _, dep := range deps[table] {
			if err := visit(dep); err != nil {
				return err
			}
		}
		state[table] = 2
		result = append(result, table)
		return nil
	}
	for _, table := range all {
		if err := visit(table); err != nil {
			return nil, err
		}
	}
	return result, nil
}

type column struct {
	name, kind string
	required   bool
	fallback   any
	primary    int
}

func columns(conn *sql.DB, table string) ([]column, error) {
	rows, err := conn.Query("PRAGMA table_info(" + quote(table) + ")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []column
	for rows.Next() {
		var c column
		var index, required int
		if err := rows.Scan(&index, &c.name, &c.kind, &required, &c.fallback, &c.primary); err != nil {
			return nil, err
		}
		c.required = required != 0
		result = append(result, c)
	}
	return result, rows.Err()
}

func migrateLegacy(src, dst *sql.DB, dry bool, out io.Writer) error {
	// A baseline model supplies SQLite metadata even when the target is Postgres.
	model, err := db.Open(":memory:")
	if err != nil {
		return err
	}
	defer model.Close()
	tables, err := orderedTables(model)
	if err != nil {
		return err
	}
	sourceTables, err := orderedTables(src)
	if err != nil {
		return err
	}
	sourceSet := map[string]bool{}
	targetSet := map[string]bool{}
	for _, table := range tables {
		targetSet[table] = true
	}
	for _, table := range sourceTables {
		if table == "julius_schema_version" {
			return errors.New("source is already a typed baseline, expected final legacy schema")
		}
		if skipped[table] {
			fmt.Fprintf(out, "skip %s\n", table)
			continue
		}
		if !targetSet[table] {
			return fmt.Errorf("unmapped source table %s", table)
		}
		sourceSet[table] = true
	}
	// Only untouched baseline seed categories may be replaced.
	postgres := !strings.Contains(fmt.Sprintf("%T", dst.Driver()), "sqlite")
	for _, table := range tables {
		if table == "goose_db_version" || table == "julius_schema_version" {
			continue
		}
		if !sourceSet[table] {
			return fmt.Errorf("missing legacy table %s; first upgrade with the old binary", table)
		}
		var count int
		if err := dst.QueryRow("SELECT COUNT(*) FROM " + quote(table)).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			if table != "categories" {
				return fmt.Errorf("target table %s is not empty", table)
			}
			if err := checkSeeds(model, dst); err != nil {
				return err
			}
		}
	}
	ctx := context.Background()
	sourceTx, err := src.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer sourceTx.Rollback()
	var dryTx *sql.Tx
	if dry {
		dryTx, err = dst.Begin()
		if err != nil {
			return err
		}
		defer dryTx.Rollback()
	}
	var failures []error
	for _, table := range tables {
		if table == "goose_db_version" || table == "julius_schema_version" {
			continue
		}
		meta, err := columns(model, table)
		if err != nil {
			return err
		}
		tx := dryTx
		if !dry {
			tx, err = dst.Begin()
			if err != nil {
				return err
			}
		}
		n, errs := copyTable(sourceTx, tx, table, meta, postgres, dry)
		if len(errs) != 0 {
			if !dry {
				tx.Rollback()
				return errors.Join(errs...)
			}
			failures = append(failures, errs...)
		}
		if !dry {
			if err := tx.Commit(); err != nil {
				return fmt.Errorf("commit %s: %w", table, err)
			}
		}
		fmt.Fprintf(out, "%s: %d rows converted\n", table, n)
	}
	if len(failures) != 0 {
		return errors.Join(failures...)
	}
	var target transactions.Querier = dst
	if dry {
		target = dryTx
	}
	if err := validate(ctx, sourceTx, target, tables, out); err != nil {
		return err
	}
	if dry {
		fmt.Fprintln(out, "dry-run: zero errors; imported data rolled back")
	} else {
		fmt.Fprintln(out, "copy complete; counts and balances match")
	}
	return nil
}

func checkSeeds(model, dst *sql.DB) error {
	read := func(conn *sql.DB) (map[string]string, error) {
		rows, err := conn.Query(`SELECT id,name,kind,is_active,created_at,updated_at,icon,color FROM categories`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		result := map[string]string{}
		for rows.Next() {
			var id, name, kind, created, updated, icon, color string
			var active bool
			if err := rows.Scan(types.ID(&id), &name, &kind, types.Bool(&active), types.Time(&created), types.Time(&updated), &icon, &color); err != nil {
				return nil, err
			}
			result[id] = fmt.Sprintf("%q", []any{name, kind, active, created, updated, icon, color})
		}
		return result, rows.Err()
	}
	want, err := read(model)
	if err != nil {
		return err
	}
	got, err := read(dst)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(got, want) {
		return errors.New("target categories differ from baseline seeds; use a fresh target")
	}
	return nil
}

func copyTable(src *sql.Tx, tx *sql.Tx, table string, meta []column, postgres, dry bool) (int, []error) {
	fail := func(err error) (int, []error) { return 0, []error{fmt.Errorf("%s: %w", table, err)} }
	rows, err := src.Query("SELECT * FROM " + quote(table))
	if err != nil {
		return fail(err)
	}
	defer rows.Close()
	names, err := rows.Columns()
	if err != nil {
		return fail(err)
	}
	indexes := map[string]int{}
	for i, name := range names {
		indexes[name] = i
	}
	known := map[string]bool{}
	for _, c := range meta {
		known[c.name] = true
	}
	for _, name := range names {
		if !known[name] {
			return fail(fmt.Errorf("unmapped column %s", name))
		}
	}
	var insertCols, placeholders []string
	var selected []column
	for _, c := range meta {
		if _, ok := indexes[c.name]; !ok {
			if c.required && c.fallback == nil {
				return fail(fmt.Errorf("new NOT NULL column %s has no default", c.name))
			}
			continue // The baseline default fills new columns.
		}
		selected = append(selected, c)
		insertCols = append(insertCols, quote(c.name))
		placeholders = append(placeholders, "?")
	}
	if table == "categories" {
		if _, err := tx.Exec("DELETE FROM categories"); err != nil {
			return fail(err)
		}
	}
	query := "INSERT INTO " + quote(table) + " (" + strings.Join(insertCols, ",") + ") VALUES (" + strings.Join(placeholders, ",") + ")"
	raw := make([]any, len(names))
	scan := make([]any, len(raw))
	for i := range raw {
		scan[i] = &raw[i]
	}
	var failures []error
	count := 0
	for rows.Next() {
		if err := rows.Scan(scan...); err != nil {
			return fail(err)
		}
		var identity []string
		for _, c := range selected {
			if c.primary != 0 {
				identity = append(identity, fmt.Sprintf("%s=%v", c.name, raw[indexes[c.name]]))
			}
		}
		sort.Strings(identity)
		label := strings.Join(identity, ",")
		if label == "" {
			label = fmt.Sprintf("row=%d", count+1)
		}
		values := make([]any, len(selected))
		invalid := false
		for i, c := range selected {
			v := raw[indexes[c.name]]
			values[i], err = convert(table+"."+c.name, v, postgres)
			if err == nil && c.kind == "INTEGER" && v != nil && columnKinds[table+"."+c.name] == "" {
				if _, ok := v.(int64); !ok {
					err = errors.New("expected integer")
				}
			}
			if err == nil && c.required && v == nil {
				err = errors.New("NULL in required column")
			}
			if err != nil {
				failures = append(failures, fmt.Errorf("%s [%s] column %s value %q: %w", table, label, c.name, fmt.Sprint(v), err))
				invalid = true
			}
		}
		if invalid {
			if !dry {
				return count, failures
			}
			continue
		}
		if dry {
			if _, err := tx.Exec("SAVEPOINT legacy_row"); err != nil {
				return fail(err)
			}
		}
		if _, err := tx.Exec(query, values...); err != nil {
			failures = append(failures, fmt.Errorf("%s [%s] values %q: %w", table, label, fmt.Sprint(raw), err))
			if !dry {
				return count, failures
			}
			if _, err := tx.Exec("ROLLBACK TO SAVEPOINT legacy_row"); err != nil {
				return fail(err)
			}
		} else {
			count++
		}
		if dry {
			if _, err := tx.Exec("RELEASE SAVEPOINT legacy_row"); err != nil {
				return fail(err)
			}
		}
	}
	if err := rows.Err(); err != nil {
		failures = append(failures, err)
	}
	return count, failures
}

func validate(ctx context.Context, src, dst transactions.Querier, tables []string, out io.Writer) error {
	for _, table := range tables {
		if table == "goose_db_version" || table == "julius_schema_version" {
			continue
		}
		var a, b int
		query := "SELECT COUNT(*) FROM " + quote(table)
		if err := src.QueryRowContext(ctx, query).Scan(&a); err != nil {
			return err
		}
		if err := dst.QueryRowContext(ctx, query).Scan(&b); err != nil {
			return err
		}
		fmt.Fprintf(out, "validate %s: source=%d target=%d\n", table, a, b)
		if a != b {
			return fmt.Errorf("%s row count mismatch: %d != %d", table, a, b)
		}
	}
	rows, err := src.QueryContext(ctx, "SELECT id FROM financial_accounts ORDER BY id")
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		a, err := transactions.AccountBalance(ctx, src, id)
		if err != nil {
			return err
		}
		b, err := transactions.AccountBalance(ctx, dst, id)
		if err != nil {
			return err
		}
		if (a == nil) != (b == nil) {
			return fmt.Errorf("account %s balance NULL mismatch", id)
		}
		if a != nil {
			left, err := types.ParseDecimal(*a)
			if err != nil {
				return err
			}
			right, err := types.ParseDecimal(*b)
			if err != nil {
				return err
			}
			if !left.Equal(right) {
				return fmt.Errorf("account %s balance mismatch: %s != %s", id, left, right)
			}
			fmt.Fprintf(out, "validate account %s balance: source=%s target=%s\n", id, left, right)
		}
		// Validate all stored movements, including pending and soft-deleted entries.
		sum := func(q transactions.Querier) (decimal.Decimal, error) {
			rows, err := q.QueryContext(ctx, `SELECT amount_in_account_currency FROM financial_transactions WHERE account_id=? UNION ALL SELECT amount FROM account_balance_adjustments WHERE account_id=?`, id, id)
			if err != nil {
				return decimal.Zero, err
			}
			defer rows.Close()
			total := decimal.Zero
			for rows.Next() {
				var value any
				if err := rows.Scan(&value); err != nil {
					return total, err
				}
				if value == nil {
					continue
				}
				d, err := types.ParseDecimal(value)
				if err != nil {
					return total, err
				}
				total = total.Add(d)
			}
			return total, rows.Err()
		}
		left, err := sum(src)
		if err != nil {
			return err
		}
		right, err := sum(dst)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "validate account %s movement sum: source=%s target=%s\n", id, left, right)
		if !left.Equal(right) {
			return fmt.Errorf("account %s movement sum mismatch", id)
		}
	}
	return nil
}
