package db_test

import (
	"database/sql"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/greg0x46/julius/internal/db/dbtest"
)

type schemaTable struct {
	Columns     map[string]string
	Indexes     []string
	Constraints []string
}

func TestBaselineSchemaParity(t *testing.T) {
	sqlite := dbtest.OpenSQLite(t)
	postgres := dbtest.OpenPostgres(t)
	a := schemaSnapshot(t, sqlite, false)
	b := schemaSnapshot(t, postgres, true)
	if len(a) != 45 {
		t.Fatalf("got %d tables, want 44 application tables and a generation marker", len(a))
	}
	for name, table := range a {
		other, ok := b[name]
		if !ok {
			t.Errorf("Postgres is missing table %s", name)
			continue
		}
		if !reflect.DeepEqual(table, other) {
			t.Errorf("schema differs for %s:\nSQLite: %#v\nPostgres: %#v", name, table, other)
		}
	}
	for name := range b {
		if _, ok := a[name]; !ok {
			t.Errorf("SQLite is missing table %s", name)
		}
	}
}

func schemaSnapshot(t *testing.T, conn *sql.DB, pg bool) map[string]schemaTable {
	t.Helper()
	query := `SELECT name FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name <> 'goose_db_version'`
	if pg {
		query = `SELECT tablename FROM pg_tables WHERE schemaname=current_schema() AND tablename <> 'goose_db_version'`
	}
	names := queryStrings(t, conn, query)
	result := map[string]schemaTable{}
	for _, name := range names {
		table := schemaTable{Columns: map[string]string{}}
		if pg {
			rows, err := conn.Query(`SELECT column_name, data_type, is_nullable FROM information_schema.columns WHERE table_schema=current_schema() AND table_name=?`, name)
			if err != nil {
				t.Fatal(err)
			}
			for rows.Next() {
				var col, typ, nullable string
				if err := rows.Scan(&col, &typ, &nullable); err != nil {
					t.Fatal(err)
				}
				table.Columns[col] = logicalType(typ) + ":" + nullable
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			rows.Close()
			table.Indexes = queryStrings(t, conn, `SELECT pg_get_indexdef(i.indexrelid) FROM pg_index i JOIN pg_class c ON c.oid=i.indrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=current_schema() AND c.relname=?`, name)
			table.Constraints = queryStrings(t, conn, `SELECT pg_get_constraintdef(co.oid) FROM pg_constraint co JOIN pg_class c ON c.oid=co.conrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=current_schema() AND c.relname=? AND co.contype IN ('c','f')`, name)
		} else {
			rows, err := conn.Query(`PRAGMA table_info("` + name + `")`)
			if err != nil {
				t.Fatal(err)
			}
			for rows.Next() {
				var id, nn, pk int
				var col, typ string
				var def any
				if err := rows.Scan(&id, &col, &typ, &nn, &def, &pk); err != nil {
					t.Fatal(err)
				}
				nullable := "YES"
				if nn != 0 || pk != 0 {
					nullable = "NO"
				}
				table.Columns[col] = logicalType(typ) + ":" + nullable
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			rows.Close()
			var definition string
			if err := conn.QueryRow(`SELECT sql FROM sqlite_schema WHERE name=? AND type='table'`, name).Scan(&definition); err != nil {
				t.Fatal(err)
			}
			table.Constraints = extractChecks(definition)
			rows, err = conn.Query(`PRAGMA foreign_key_list("` + name + `")`)
			if err != nil {
				t.Fatal(err)
			}
			for rows.Next() {
				var id, seq int
				var parent, from, to, onUpdate, onDelete, match string
				if err := rows.Scan(&id, &seq, &parent, &from, &to, &onUpdate, &onDelete, &match); err != nil {
					t.Fatal(err)
				}
				table.Constraints = append(table.Constraints, fmt.Sprintf("FOREIGN KEY (%s) REFERENCES %s(%s) ON DELETE %s", from, parent, to, onDelete))
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			rows.Close()
			rows, err = conn.Query(`PRAGMA index_list("` + name + `")`)
			if err != nil {
				t.Fatal(err)
			}
			type index struct {
				name   string
				unique int
			}
			var indexes []index
			for rows.Next() {
				var seq, unique, partial int
				var idx, origin string
				if err := rows.Scan(&seq, &idx, &unique, &origin, &partial); err != nil {
					t.Fatal(err)
				}
				indexes = append(indexes, index{idx, unique})
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			rows.Close()
			for _, idx := range indexes {
				var stmt sql.NullString
				if err := conn.QueryRow(`SELECT sql FROM sqlite_schema WHERE name=?`, idx.name).Scan(&stmt); err != nil {
					t.Fatal(err)
				}
				if stmt.Valid {
					table.Indexes = append(table.Indexes, stmt.String)
					continue
				}
				rows, err := conn.Query(`PRAGMA index_info("` + idx.name + `")`)
				if err != nil {
					t.Fatal(err)
				}
				var cols []string
				for rows.Next() {
					var seq, cid int
					var col string
					if err := rows.Scan(&seq, &cid, &col); err != nil {
						t.Fatal(err)
					}
					cols = append(cols, col)
				}
				if err := rows.Err(); err != nil {
					t.Fatal(err)
				}
				rows.Close()
				table.Indexes = append(table.Indexes, "CREATE UNIQUE INDEX auto ON "+name+" ("+strings.Join(cols, ",")+")")
			}
			// INTEGER PRIMARY KEY is the SQLite rowid, without a separate index.
			if strings.Contains(definition, "id INTEGER NOT NULL PRIMARY KEY") || strings.Contains(definition, "id INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT") {
				table.Indexes = append(table.Indexes, "CREATE UNIQUE INDEX auto ON "+name+" (id)")
			}
		}
		for i, s := range table.Indexes {
			table.Indexes[i] = normalizeSchemaSQL(s, true)
		}
		var checks []string
		for _, s := range table.Constraints {
			if normalized := normalizeSchemaSQL(s, false); normalized != "" {
				checks = append(checks, normalized)
			}
		}
		table.Constraints = checks
		sort.Strings(table.Indexes)
		sort.Strings(table.Constraints)
		result[name] = table
	}
	return result
}

func logicalType(s string) string {
	switch strings.ToLower(s) {
	case "text", "uuid", "numeric", "date", "timestamp with time zone":
		return "value"
	case "integer", "bigint":
		return "integer"
	case "boolean":
		return "integer"
	case "blob", "bytea":
		return "bytes"
	default:
		return s
	}
}

func queryStrings(t *testing.T, conn *sql.DB, query string, args ...any) []string {
	t.Helper()
	rows, err := conn.Query(query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var values []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		values = append(values, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return values
}

func extractChecks(s string) []string {
	var checks []string
	for {
		start := strings.Index(s, "CHECK (")
		if start < 0 {
			return checks
		}
		s = s[start+len("CHECK "):]
		depth, end := 0, 0
		quote := false
		for i, c := range s {
			if c == '\'' {
				quote = !quote
			}
			if quote {
				continue
			}
			if c == '(' {
				depth++
			}
			if c == ')' {
				depth--
				if depth == 0 {
					end = i + 1
					break
				}
			}
		}
		checks = append(checks, "CHECK "+s[:end])
		s = s[end:]
	}
}

var schemaTokens = regexp.MustCompile(`'(?:''|[^'])*'|[a-zA-Z_][a-zA-Z_0-9]*|<>|<=|>=|!=|[=+~><-]|[0-9]+`)

func normalizeSchemaSQL(s string, index bool) string {
	s = regexp.MustCompile(`\b[a-zA-Z_][a-zA-Z_0-9]*\.`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`::(?:text|numeric|integer)\b(?:\[\])?`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`(?i)=\s*ANY\s*\(\s*ARRAY\[`).ReplaceAllString(s, " IN (")
	s = regexp.MustCompile(`(?i)<>\s*ALL\s*\(\s*ARRAY\[`).ReplaceAllString(s, " NOT IN (")
	s = strings.ReplaceAll(s, "])", ")")
	s = strings.ReplaceAll(s, "BOTH FROM ", "")
	s = strings.ReplaceAll(s, "!=", "<>")
	s = regexp.MustCompile(`(\w+) BETWEEN (\d+) AND (\d+)`).ReplaceAllString(s, "$1 >= $2 AND $1 <= $3")
	if index {
		s = regexp.MustCompile(`(?i)(INDEX)\s+\w+\s+ON`).ReplaceAllString(s, "INDEX ON")
		s = strings.ReplaceAll(s, "USING btree ", "")
		s = strings.ReplaceAll(s, "NULLS LAST", "")
	}
	if strings.Contains(s, "'NaN'") {
		return ""
	} // Native NUMERIC also rejects non-finite values.
	if regexp.MustCompile(`CHECK \(is_\w+ IN \(0, 1\)\)`).MatchString(s) {
		return ""
	} // BOOLEAN supplies this domain in Postgres.
	// Exact SQLite sign checks are the equivalent of NUMERIC comparisons.
	if strings.Contains(s, "NOT GLOB '*[^0-9.-]*'") {
		col := regexp.MustCompile(`CHECK \(\((\w+)`).FindStringSubmatch(s)[1]
		op := ">"
		if strings.Contains(s, " OR ltrim(") {
			op = ">="
		}
		s = "CHECK (" + col + " " + op + " 0)"
	}
	s = strings.ReplaceAll(s, "GLOB '[A-Z][A-Z][A-Z]'", "~ '^[A-Z]{3}$'")
	var tokens []string
	for _, tok := range schemaTokens.FindAllString(s, -1) {
		if tok == "CAST" || tok == "AS" || tok == "INTEGER" {
			continue
		}
		if tok == "true" || tok == "TRUE" {
			tok = "1"
		}
		if tok == "false" || tok == "FALSE" {
			tok = "0"
		}
		if !strings.HasPrefix(tok, "'") {
			tok = strings.ToLower(tok)
		}
		tokens = append(tokens, tok)
	}
	return strings.Join(tokens, " ")
}

func TestBaselineNativeTypes(t *testing.T) {
	// Inventory classifications pin every column, including misleading names like captured_at.
	inventory, err := os.ReadFile("../../.specs/reorganizacao-schema/inventario.md")
	if err != nil {
		t.Fatal(err)
	}
	var table string
	expected := map[string]string{}
	for _, line := range strings.Split(string(inventory), "\n") {
		if m := regexp.MustCompile("^### `([^`]+)`").FindStringSubmatch(line); m != nil {
			table = m[1]
		}
		if m := regexp.MustCompile("^\\| `([^`]+)` \\|[^|]+\\|[^|]+\\| ([^|]+) \\|").FindStringSubmatch(line); m != nil && table != "goose_db_version" {
			kind := strings.TrimSpace(m[2])
			typ := map[string]string{"money": "numeric", "decimal (non-money)": "numeric", "instant": "timestamp with time zone", "calendar date": "date", "boolean": "boolean"}[kind]
			if typ != "" {
				expected[table+"."+m[1]] = typ
			}
		}
	}
	conn := dbtest.OpenPostgres(t)
	rows, err := conn.Query(`SELECT table_name,column_name,data_type FROM information_schema.columns WHERE table_schema=current_schema()`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var table, col, typ string
		if err := rows.Scan(&table, &col, &typ); err != nil {
			t.Fatal(err)
		}
		key := table + "." + col
		if want, ok := expected[key]; ok {
			if typ != want {
				t.Errorf("%s type=%s, want %s", key, typ, want)
			}
			delete(expected, key)
		}
		if col == "id" && typ == "text" && table != "investment_accounts" {
			t.Errorf("%s should use UUID", key)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(expected) != 0 {
		t.Errorf("missing typed columns: %v", expected)
	}
}
