// Command migrate-to-postgres is a one-shot data copier: it reads every row
// out of a SQLite julius database and re-inserts it into a Postgres
// database (already schema-migrated via internal/db, same as the server
// does on startup). Table order is derived from PRAGMA foreign_key_list so
// FK-referenced rows always land before the rows that reference them. Not
// wired into the server — run once via `go run`, then delete.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/db/types"
)

func main() {
	sqlitePath := flag.String("sqlite", db.DefaultPath(), "path to the source SQLite database")
	pgDSN := flag.String("postgres", "", "postgres://... DSN of the (schema-migrated) target database")
	flag.Parse()

	if *pgDSN == "" {
		log.Fatal("-postgres DSN is required")
	}

	src, err := db.Open(*sqlitePath)
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}
	defer src.Close()

	dst, err := db.Open(*pgDSN)
	if err != nil {
		log.Fatalf("open postgres: %v", err)
	}
	defer dst.Close()

	tables, err := orderedTables(src)
	if err != nil {
		log.Fatalf("determine table order: %v", err)
	}

	// Some migrations seed fixed-id rows (e.g. default categories) into a
	// freshly created schema; truncate everything first so the SQLite data
	// -- the source of truth, possibly edited since those defaults were
	// seeded -- doesn't collide with them on insert.
	truncateSQL := fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", strings.Join(tables, ", "))
	if _, err := dst.Exec(truncateSQL); err != nil {
		log.Fatalf("truncate target tables: %v", err)
	}

	for _, table := range tables {
		n, err := copyTable(src, dst, table)
		if err != nil {
			log.Fatalf("copy %s: %v", table, err)
		}
		log.Printf("%-40s %d rows", table, n)
	}
	log.Println("done")
}

// orderedTables lists application tables (i.e. everything but goose's own
// bookkeeping table) topologically sorted by foreign key so that a
// referenced table is always copied before the table referencing it.
func orderedTables(conn *sql.DB) ([]string, error) {
	rows, err := conn.Query("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name != 'goose_db_version'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var all []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		all = append(all, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	deps := map[string][]string{}
	for _, table := range all {
		fkRows, err := conn.Query(fmt.Sprintf("PRAGMA foreign_key_list(%q)", table))
		if err != nil {
			return nil, err
		}
		var refs []string
		for fkRows.Next() {
			var id, seq int
			var refTable, from, to string
			var onUpdate, onDelete, match string
			if err := fkRows.Scan(&id, &seq, &refTable, &from, &to, &onUpdate, &onDelete, &match); err != nil {
				fkRows.Close()
				return nil, err
			}
			if refTable != table {
				refs = append(refs, refTable)
			}
		}
		fkRows.Close()
		if err := fkRows.Err(); err != nil {
			return nil, err
		}
		deps[table] = refs
	}

	var ordered []string
	visited := map[string]bool{}
	var visit func(string) error
	visit = func(table string) error {
		if visited[table] {
			return nil
		}
		visited[table] = true
		for _, dep := range deps[table] {
			if err := visit(dep); err != nil {
				return err
			}
		}
		ordered = append(ordered, table)
		return nil
	}
	for _, table := range all {
		if err := visit(table); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

func copyTable(src, dst *sql.DB, table string) (int, error) {
	rows, err := src.Query(fmt.Sprintf("SELECT * FROM %q", table))
	if err != nil {
		return 0, fmt.Errorf("select: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return 0, err
	}

	placeholders := make([]string, len(cols))
	quotedCols := make([]string, len(cols))
	for i, c := range cols {
		placeholders[i] = "?"
		quotedCols[i] = c
	}
	insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", table, strings.Join(quotedCols, ", "), strings.Join(placeholders, ", "))
	targetRows, err := dst.Query(fmt.Sprintf("SELECT %s FROM %s LIMIT 0", strings.Join(quotedCols, ", "), table))
	if err != nil {
		return 0, fmt.Errorf("target columns: %w", err)
	}
	targetTypes, err := targetRows.ColumnTypes()
	targetRows.Close()
	if err != nil {
		return 0, fmt.Errorf("target column types: %w", err)
	}

	tx, err := dst.Begin()
	if err != nil {
		return 0, err
	}
	stmt, err := tx.Prepare(insertSQL)
	if err != nil {
		tx.Rollback()
		return 0, err
	}

	n := 0
	values := make([]any, len(cols))
	scanArgs := make([]any, len(cols))
	for i := range values {
		scanArgs[i] = &values[i]
	}
	for rows.Next() {
		if err := rows.Scan(scanArgs...); err != nil {
			stmt.Close()
			tx.Rollback()
			return 0, fmt.Errorf("scan row %d: %w", n, err)
		}
		for i, column := range targetTypes {
			if values[i] == nil || (column.DatabaseTypeName() != "BOOL" && column.DatabaseTypeName() != "BOOLEAN") {
				continue
			}
			values[i], err = types.ParseBool(values[i])
			if err != nil {
				stmt.Close()
				tx.Rollback()
				return 0, fmt.Errorf("convert row %d column %s: %w", n, cols[i], err)
			}
		}
		if _, err := stmt.Exec(values...); err != nil {
			stmt.Close()
			tx.Rollback()
			return 0, fmt.Errorf("insert row %d: %w", n, err)
		}
		n++
	}
	if err := rows.Err(); err != nil {
		stmt.Close()
		tx.Rollback()
		return 0, err
	}
	stmt.Close()
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
}
