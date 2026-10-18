package categories_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/greg0x46/julius/internal/categories"
	"github.com/greg0x46/julius/internal/money"
)

func TestApplyRuleDoesNotOverwriteManualSiblingInstallment(t *testing.T) {
	conn := newTestDB(t)
	ctx := context.Background()
	accountID, rawImportID := newTestAccount(t, conn)
	d1 := "Loja X 1/3"
	m1 := installmentMetadata(t, "1234", 1, 3)
	tx1 := insertTransactionOnAccount(t, conn, accountID, rawImportID, "", &d1, &m1)
	if _, err := categories.AssignManual(ctx, conn, tx1, seededExpenseCategory); err != nil {
		t.Fatal(err)
	}
	other, err := categories.Create(ctx, conn, "Outra", money.Expense, "", "")
	if err != nil {
		t.Fatal(err)
	}
	d2 := "Loja X 2/3"
	m2 := installmentMetadata(t, "1234", 2, 3)
	tx2 := insertTransactionOnAccount(t, conn, accountID, rawImportID, "", &d2, &m2)

	got, changed, err := categories.ApplyRule(ctx, conn, tx2, other.ID)
	if err != nil || !changed || got.CategoryID != other.ID {
		t.Fatalf("ApplyRule primary: %+v changed=%v err=%v", got, changed, err)
	}
	sibling, err := getDecision(t, conn, tx1)
	if err != nil {
		t.Fatal(err)
	}
	if sibling.categoryID != seededExpenseCategory || sibling.origin != "manual" {
		t.Fatalf("manual sibling decision overwritten: %+v", sibling)
	}
}

// categoryFailingQuerier fails category lookups with a non-ErrNoRows error.
type categoryFailingQuerier struct {
	categories.Querier
	broken *sql.DB
}

func (q categoryFailingQuerier) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	if strings.Contains(query, "FROM categories") {
		return q.broken.QueryRowContext(ctx, query, args...)
	}
	return q.Querier.QueryRowContext(ctx, query, args...)
}

func TestCategoryLookupDBErrorIsNotCategoryInvalid(t *testing.T) {
	conn := newTestDB(t)
	ctx := context.Background()
	txID := insertTransaction(t, conn, nil)
	broken := newTestDB(t)
	if err := broken.Close(); err != nil {
		t.Fatal(err)
	}
	q := categoryFailingQuerier{Querier: conn, broken: broken}

	_, err := categories.AssignManualWithQuerier(ctx, q, txID, seededExpenseCategory)
	if err == nil || errors.Is(err, categories.ErrCategoryInvalid) {
		t.Fatalf("AssignManualWithQuerier error = %v, want a DB error", err)
	}
	_, _, err = categories.ApplyRuleWithQuerier(ctx, q, txID, seededExpenseCategory)
	if err == nil || errors.Is(err, categories.ErrCategoryInvalid) {
		t.Fatalf("ApplyRuleWithQuerier error = %v, want a DB error", err)
	}
}
