package db

import (
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
)

// A generation marker distinguishes the new sequence from legacy version 1.
func guardSchema(conn *sql.DB, dialect goose.Dialect) error {
	query := `SELECT name FROM sqlite_schema WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`
	if dialect == goose.DialectPostgres {
		query = `SELECT tablename FROM pg_tables WHERE schemaname = current_schema()`
	}
	rows, err := conn.Query(query)
	if err != nil {
		return fmt.Errorf("inspect schema generation: %w", err)
	}
	var marker, applicationTables bool
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			rows.Close()
			return fmt.Errorf("inspect schema generation: %w", err)
		}
		marker = marker || table == "julius_schema_version"
		applicationTables = applicationTables || table != "goose_db_version"
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return fmt.Errorf("inspect schema generation: %w", err)
	}
	if !marker {
		if applicationTables {
			return fmt.Errorf("unsupported legacy database schema: this binary requires schema generation 1; keep the database unchanged and use the legacy binary until it has been converted")
		}
		return nil
	}
	var generation int
	if err := conn.QueryRow(`SELECT generation FROM julius_schema_version WHERE id = 1`).Scan(&generation); err != nil {
		return fmt.Errorf("read schema generation: %w", err)
	}
	if generation != 1 {
		return fmt.Errorf("unsupported database schema generation %d: this binary requires generation 1", generation)
	}
	return nil
}
