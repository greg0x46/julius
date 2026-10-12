package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/greg0x46/julius/internal/automation"
	"github.com/greg0x46/julius/internal/categories"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/money"
)

// statementCounts counts every row a confirmation may write.
func statementCounts(t *testing.T, conn *sql.DB) map[string]int {
	t.Helper()
	out := map[string]int{}
	for name, query := range map[string]string{
		"transactions":       `SELECT COUNT(*) FROM financial_transactions`,
		"raw imports":        `SELECT COUNT(*) FROM raw_imports`,
		"file runs":          `SELECT COUNT(*) FROM sync_runs WHERE run_type = 'file_import'`,
		"statement imports":  `SELECT COUNT(*) FROM statement_imports`,
		"normalization":      `SELECT COUNT(*) FROM normalization_events`,
		"category events":    `SELECT COUNT(*) FROM transaction_category_events`,
		"category decisions": `SELECT COUNT(*) FROM transaction_category_decisions`,
		"inclusion events":   `SELECT COUNT(*) FROM transaction_inclusion_events`,
		"data sources":       `SELECT COUNT(*) FROM data_sources`,
		"accounts":           `SELECT COUNT(*) FROM financial_accounts`,
	} {
		out[name] = countWhere(t, conn, query)
	}
	return out
}

// Confirming a file again, or a file whose period overlaps one already
// imported, adds only its audit records: one run and one statement import,
// no transaction, decision or balance change.
func TestStatementImportReplayIsIdempotent(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		accountID, counts := importStatementFile(t, conn, validFlash, map[string]string{"new_account_name": "Flash refeições"})
		if counts.New != 2 {
			t.Fatalf("first import %+v", counts)
		}
		balance, asOf := accountBalance(t, conn, accountID)
		fields := map[string]string{"account_id": accountID}
		for _, c := range []struct {
			name     string
			file     string
			dups     int
			decision string
		}{
			{"same file", validFlash, 2, ""},
			{"same file again", validFlash, 2, ""},
			{"overlapping period", flashFile(rowCafe), 1, "ignore"},
		} {
			before := statementCounts(t, conn)
			if c.decision != "" {
				fields["ambiguous_line_2"] = c.decision
			}
			_, counts := importStatementFile(t, conn, c.file, fields)
			delete(fields, "ambiguous_line_2")
			if counts.New != 0 || counts.Duplicate != c.dups {
				t.Fatalf("%s: counts %+v", c.name, counts)
			}
			want := before
			want["file runs"]++
			want["raw imports"]++
			want["statement imports"]++
			if got := statementCounts(t, conn); !reflect.DeepEqual(got, want) {
				t.Fatalf("%s: rows %v, want %v", c.name, got, want)
			}
			if n := countWhere(t, conn, `SELECT COUNT(*) FROM sync_runs WHERE run_type = 'file_import' AND transactions_inserted <> 0`); n != 1 {
				t.Fatalf("%s: %d runs report inserted transactions, want only the first", c.name, n)
			}
			if b, a := accountBalance(t, conn, accountID); b != balance || a != asOf {
				t.Fatalf("%s: balance %v as of %v, want %v as of %v", c.name, b, a, balance, asOf)
			}
		}
	})
}

// Identity is (account, line content, ordinal among identical lines): two
// identical lines are two movements, and a later file with a third identical
// line imports exactly that one.
func TestStatementIdenticalLinesAreNotCollapsed(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		twice := flashFile(rowCafe, rowCafe)
		accountID, counts := importStatementFile(t, conn, twice, map[string]string{"new_account_name": "Flash refeições"})
		if counts.New != 2 {
			t.Fatalf("counts %+v", counts)
		}
		if n := countWhere(t, conn, `SELECT COUNT(DISTINCT external_id) FROM financial_transactions WHERE account_id = ?`, accountID); n != 2 {
			t.Fatalf("%d distinct identities, want 2", n)
		}
		fields := map[string]string{"account_id": accountID}
		if _, counts = importStatementFile(t, conn, twice, fields); counts.New != 0 || counts.Duplicate != 2 {
			t.Fatalf("reimport %+v", counts)
		}
		fields["ambiguous_line_2"] = "ignore"
		fields["ambiguous_line_3"] = "ignore"
		if _, counts = importStatementFile(t, conn, flashFile(rowCafe, rowCafe, rowCafe), fields); counts.New != 1 || counts.Duplicate != 2 {
			t.Fatalf("three identical lines %+v", counts)
		}
		if n := countWhere(t, conn, `SELECT COUNT(*) FROM financial_transactions WHERE account_id = ?`, accountID); n != 3 {
			t.Fatalf("%d transactions, want 3", n)
		}
	})
}

// Flash records time only to the minute. A later statement can contain a
// different purchase with the same description, amount and payment method;
// the resulting balance distinguishes it from a replay of the first purchase.
func TestStatementSameTextDifferentBalanceIsNotSilentlyDeduplicated(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		firstFile := flashFile(rowCafe)
		accountID, counts := importStatementFile(t, conn, firstFile, map[string]string{"new_account_name": "Flash refeições"})
		if counts.New != 1 {
			t.Fatalf("first import %+v", counts)
		}
		fields := map[string]string{"account_id": accountID}
		secondFile := flashFile(`09/09/2026,12:00,Café,"-R$ 10,00",Cartão,"R$ 80,00"`)
		preview := previewStatementFile(t, conn, secondFile, fields)
		if preview.Counts.Ambiguous != 1 || preview.Counts.Duplicate != 0 || !preview.hasWarning(warnBalanceDiffers) {
			t.Fatalf("same-looking purchase with different balance: %+v", preview)
		}
		fields["ambiguous_line_2"] = "import"
		if _, counts = importStatementFile(t, conn, secondFile, fields); counts.New != 1 || counts.Duplicate != 0 {
			t.Fatalf("second purchase %+v", counts)
		}
		delete(fields, "ambiguous_line_2")
		if n := countWhere(t, conn, `SELECT COUNT(DISTINCT external_id) FROM financial_transactions WHERE account_id=? AND description='Café'`, accountID); n != 2 {
			t.Fatalf("%d distinct purchases, want 2", n)
		}
		for _, file := range []string{firstFile, secondFile} {
			if _, counts = importStatementFile(t, conn, file, fields); counts.New != 0 || counts.Duplicate != 1 {
				t.Fatalf("replay %+v", counts)
			}
		}
		if n := countWhere(t, conn, `SELECT COUNT(*) FROM financial_transactions WHERE account_id=?`, accountID); n != 2 {
			t.Fatalf("%d rows after both replays, want 2", n)
		}
	})
}

func TestStatementSameBalanceFromAnotherFileNeedsLineDecision(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		accountID, counts := importStatementFile(t, conn, validFlash, map[string]string{"new_account_name": "Flash refeições"})
		if counts.New != 2 {
			t.Fatalf("first import %+v", counts)
		}
		file := flashFile(rowCafe)
		fields := map[string]string{"account_id": accountID}
		preview := previewStatementFile(t, conn, file, fields)
		if preview.Counts.Ambiguous != 1 || preview.Counts.Duplicate != 0 || !preview.hasWarning(warnOtherFileMatch) {
			t.Fatalf("cross-file preview %+v", preview)
		}
		requireProblem(t, confirmStatementFile(t, conn, file, preview, fields), 422, "ambiguous-row")
		if n := countWhere(t, conn, `SELECT COUNT(*) FROM financial_transactions WHERE account_id=?`, accountID); n != 2 {
			t.Fatalf("missing decision changed %d rows", n)
		}
		fields["ambiguous_line_2"] = "ignore"
		if _, counts = importStatementFile(t, conn, file, fields); counts.New != 0 || counts.Duplicate != 1 {
			t.Fatalf("ignored match %+v", counts)
		}
		fields["ambiguous_line_2"] = "import"
		stale := previewStatementFile(t, conn, file, map[string]string{"account_id": accountID})
		if _, counts = importStatementFile(t, conn, file, fields); counts.New != 1 || counts.Duplicate != 0 {
			t.Fatalf("distinct same-balance purchase %+v", counts)
		}
		requireProblem(t, confirmStatementFile(t, conn, file, stale, fields), 409, "preview-changed")
		if n := countWhere(t, conn, `SELECT COUNT(*) FROM financial_transactions WHERE account_id=?`, accountID); n != 3 {
			t.Fatalf("%d rows after explicit import, want 3", n)
		}
		delete(fields, "ambiguous_line_2")
		if _, counts = importStatementFile(t, conn, file, fields); counts.New != 0 || counts.Duplicate != 1 {
			t.Fatalf("replay of explicitly imported file %+v", counts)
		}
	})
}

func TestStatementLegacyIdentityWithMatchingBalanceStillReplays(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		file := flashFile(rowCafe)
		accountID, _ := importStatementFile(t, conn, file, map[string]string{"new_account_name": "Flash refeições"})
		base := digest(cafeOccurredAt, "café", "-10.00", "cartão")
		legacyID := digest("statement-v1", accountID, base, "1")
		if _, err := conn.Exec(`UPDATE financial_transactions SET external_id=? WHERE account_id=?`, legacyID, accountID); err != nil {
			t.Fatal(err)
		}
		if _, counts := importStatementFile(t, conn, file, map[string]string{"account_id": accountID}); counts.New != 0 || counts.Duplicate != 1 {
			t.Fatalf("legacy replay %+v", counts)
		}
		if n := countWhere(t, conn, `SELECT COUNT(*) FROM financial_transactions WHERE account_id=?`, accountID); n != 1 {
			t.Fatalf("%d rows after legacy replay", n)
		}
	})
}

// Identity is scoped to the account: the same file in two accounts yields two
// independent sets of rows.
func TestStatementSameFileInTwoAccountsIsNotMerged(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		first, counts := importStatementFile(t, conn, validFlash, map[string]string{"new_account_name": "Conta A"})
		if counts.New != 2 {
			t.Fatalf("first %+v", counts)
		}
		second, counts := importStatementFile(t, conn, validFlash, map[string]string{"new_account_name": "Conta B"})
		if counts.New != 2 || counts.Duplicate != 0 || first == second {
			t.Fatalf("second %+v (accounts %s, %s)", counts, first, second)
		}
		if n := countWhere(t, conn, `SELECT COUNT(DISTINCT external_id) FROM financial_transactions`); n != 4 {
			t.Fatalf("%d distinct identities, want 4", n)
		}
	})
}

// A Pluggy row and a manual row that look like a file row are only warnings:
// the file row is still imported and neither existing row is touched.
func TestStatementLookalikeRowsOnlyWarn(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		accountID := newFileAccount(t, conn)
		insertPluggyStatementRow(t, conn, "Café", "-10", cafeOccurredAt)
		insertManualStatementRow(t, conn, accountID, "Café", "-10.00", cafeOccurredAt)
		existing := func() [][]string {
			result, err := conn.Query(`SELECT id, account_id, description, amount, origin, updated_at, COALESCE(deleted_at, '') FROM financial_transactions WHERE occurred_at = ? ORDER BY id`, cafeOccurredAt)
			if err != nil {
				t.Fatal(err)
			}
			defer result.Close()
			var out [][]string
			for result.Next() {
				row := make([]string, 7)
				if err := result.Scan(&row[0], &row[1], &row[2], &row[3], &row[4], &row[5], &row[6]); err != nil {
					t.Fatal(err)
				}
				out = append(out, row)
			}
			return out
		}
		before := existing()
		if len(before) != 2 {
			t.Fatalf("setup rows %v", before)
		}

		fields := map[string]string{"account_id": accountID}
		file := flashFile(rowCafe)
		p := previewStatementFile(t, conn, file, fields)
		if p.Counts.New != 1 || !p.hasWarning(warnPluggyMatch) || !p.hasWarning(warnManualMatch) {
			t.Fatalf("preview %+v", p)
		}
		rr := confirmStatementFile(t, conn, file, p, fields)
		if rr.Code != 201 {
			t.Fatalf("confirm %d: %s", rr.Code, rr.Body.String())
		}
		after := existing()
		if len(after) != 3 {
			t.Fatalf("%d rows at the instant, want the two existing ones plus the file row", len(after))
		}
		kept := 0
		for _, row := range after {
			for _, old := range before {
				if reflect.DeepEqual(row, old) {
					kept++
				}
			}
		}
		if kept != 2 {
			t.Fatalf("existing rows changed: %v -> %v", before, after)
		}
	})
}

// A failure while confirming rolls everything back, including rows already
// inserted earlier in the same file; the same upload then succeeds as a
// whole, and once only.
func TestStatementConfirmFailureLeavesNothingAndRetrySucceeds(t *testing.T) {
	for _, existingAccount := range []bool{false, true} {
		name := "new account"
		if existingAccount {
			name = "existing account"
		}
		t.Run(name, func(t *testing.T) {
			dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
				ctx := context.Background()
				category, err := categories.Create(ctx, conn, "Lanches", money.Expense, "tag", "#000000")
				if err != nil {
					t.Fatal(err)
				}
				// The rule matches the second line only, so the fault fires
				// after the first line is already in the transaction.
				if _, err := automation.Create(ctx, conn, automation.Write{
					Name: "Café é lanche", IsActive: true, LogicOperator: automation.LogicAnd,
					Conditions: []automation.Condition{{Field: automation.FieldDescription, Operator: automation.OperatorEquals, Value: "Café"}},
					Actions:    []automation.ActionWrite{{Type: automation.ActionSetCategory, CategoryID: &category.ID}},
				}); err != nil {
					t.Fatal(err)
				}
				fields := map[string]string{"new_account_name": "Flash refeições"}
				previewFields := map[string]string{}
				if existingAccount {
					accountID := newFileAccount(t, conn)
					fields = map[string]string{"account_id": accountID}
					previewFields = fields
				}
				file := flashFile(rowDeposit, rowCafe)
				p := previewStatementFile(t, conn, file, previewFields)
				before := statementCounts(t, conn)
				var balance, asOf sql.NullString
				if existingAccount {
					balance, asOf = accountBalance(t, conn, fields["account_id"])
				}

				drop := dbtest.FailInserts(t, conn, "transaction_category_events", `NEW.origin = 'rule'`)
				captureLog(t)
				requireProblem(t, confirmStatementFile(t, conn, file, p, fields), 503, "import-unavailable")
				if got := statementCounts(t, conn); !reflect.DeepEqual(got, before) {
					t.Fatalf("failed confirmation left rows: %v, want %v", got, before)
				}
				if existingAccount {
					if b, a := accountBalance(t, conn, fields["account_id"]); b != balance || a != asOf {
						t.Fatalf("failed confirmation moved the balance to %v as of %v", b, a)
					}
				}
				drop()

				rr := confirmStatementFile(t, conn, file, p, fields)
				if rr.Code != 201 {
					t.Fatalf("retry %d: %s", rr.Code, rr.Body.String())
				}
				got := statementCounts(t, conn)
				if got["transactions"] != before["transactions"]+2 || got["statement imports"] != before["statement imports"]+1 {
					t.Fatalf("retry rows %v, before %v", got, before)
				}
				if n := countWhere(t, conn, `SELECT COUNT(*) FROM transaction_category_decisions WHERE origin = 'rule' AND category_id = ?`, category.ID); n != 1 {
					t.Fatalf("%d rule decisions after retry, want 1", n)
				}
				var out struct {
					AccountID string `json:"account_id"`
				}
				if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
					t.Fatal(err)
				}
				if _, counts := importStatementFile(t, conn, file, map[string]string{"account_id": out.AccountID}); counts.New != 0 || counts.Duplicate != 2 {
					t.Fatalf("replay after retry %+v", counts)
				}
			})
		})
	}
}
