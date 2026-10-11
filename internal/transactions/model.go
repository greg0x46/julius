// Package transactions ports app/transactions/query.py (the filtered,
// paginated, grouped transaction listing with running totals) and the
// manual inclusion/exclusion half of app/transactions/inclusion.py. Category
// assignment lives in package categories; classification, effective money,
// and eligibility rules live in package money.
package transactions

import (
	"context"
	"database/sql"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/reporting"
)

// Querier is satisfied by both *sql.DB and *sql.Tx.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Filters mirrors TransactionFilters. DateFrom/DateTo only take effect as a
// pair — matching the reference, setting just one has no effect.
type Filters struct {
	Origin         *string
	SourceProvider *string
	CardBalance    bool
	CreditCard     bool
	DateFrom       *money.Date
	DateTo         *money.Date
	Description    *string
	// AccountIDs, Institutions, CategoryIDs and ProviderStatuses are each
	// "any of" sets: an empty slice means "no filter", a non-empty one keeps
	// a transaction that matches any of its values. Across fields the
	// filter is still an AND.
	AccountIDs       []string
	Institutions     []string
	CategoryIDs      []string
	Classification   *money.Classification
	ProviderStatuses []string
	AmountMin        *decimal.Decimal
	AmountMax        *decimal.Decimal
	Uncategorized    bool
}

// QueryRequest mirrors TransactionQuery.
type QueryRequest struct {
	Timezone string
	GroupBy  money.GroupBy
	Page     int
	PageSize int
	Filters  Filters
}

type AccountSummary struct {
	ID           string
	Name         *string
	Institution  *string
	CurrencyCode *string
}

type EffectiveMoneyView struct {
	Value        string
	CurrencyCode string
	Source       money.Source
}

type Inclusion struct {
	State     money.InclusionState
	ChangedAt *time.Time
	Origin    string
	RuleName  *string
}

type InternalCategory struct {
	ID        string
	Name      string
	Kind      money.CategoryKind
	IsActive  bool
	Icon      string
	Color     string
	Origin    string
	ChangedAt time.Time
}

type TotalsEligibility struct {
	Included bool
	Reason   *money.EligibilityReason
}

// MovesCash reports whether this transaction moved real money in or out of
// the account it sits on, which is not the same question as whether it counts
// toward income/expense totals.
//
// The two answers diverge on exactly one case: a transaction categorized as a
// transfer between the user's own accounts is kept out of the totals (both
// legs would otherwise count the same money twice) while still being a real
// withdrawal from the origin account — possibly to an account this app does
// not track at all.
//
// Every consumer that reconstructs or projects a *balance* must filter on
// this instead of on Included. Consumers that report income/expense flows —
// Totals, SpendingByCategory, the financial report — must keep using
// Included. See money.MovedCash for the rule itself.
func (e TotalsEligibility) MovesCash() bool {
	return e.Included || money.MovedCash(e.Reason)
}

// CardInfo is the subset of Pluggy's opaque credit_card_metadata JSON that's
// useful for display: which card a purchase was made on, and — for
// installment purchases — which parcela this row is.
type CardInfo struct {
	Number            string
	InstallmentNumber *int
	TotalInstallments *int
}

// Item mirrors TransactionItem: one row of the query result, with every
// derived field (classification, effective money, eligibility, group key)
// already resolved so the HTTP layer never has to re-run domain logic.
type Item struct {
	ReportingDate            *time.Time
	Reporting                reporting.Conversion
	ID                       string
	ExternalID               string
	Origin                   string
	SourceProvider           *string
	OccurredAt               *time.Time
	Description              *string
	Account                  AccountSummary
	SourceCategory           *string
	InternalCategory         *InternalCategory
	MovementType             *string
	ProviderStatus           *string
	Classification           money.Classification
	Amount                   *string
	CurrencyCode             *string
	AmountInAccountCurrency  *string
	EffectiveMoney           *EffectiveMoneyView
	InvestmentTransferAmount string
	ReportableAmount         *string
	Card                     *CardInfo
	Inclusion                Inclusion
	TotalsEligibility        TotalsEligibility
	GroupKey                 string
}

type CurrencyTotals = reporting.Totals

type Group struct {
	Reporting      reporting.Summary
	Key            string
	Kind           money.PeriodKind
	StartDate      *money.Date
	EndDate        *money.Date
	ItemCount      int
	PageItemCount  int
	HasItemsBefore bool
	HasItemsAfter  bool
	Totals         []CurrencyTotals
}

type Page struct {
	Number     int
	Size       int
	TotalItems int
	TotalPages int
}

type AccountOption struct {
	ID          string
	Name        *string
	Institution *string
}

type CategoryOption struct {
	ID       string
	Name     string
	Kind     money.CategoryKind
	IsActive bool
	Icon     string
	Color    string
}

type AvailableFilters struct {
	Accounts     []AccountOption
	Institutions []string
	Categories   []CategoryOption
}

// Result mirrors TransactionQueryResult.
type Result struct {
	Reporting        reporting.Summary
	ConfirmedAt      time.Time
	StoredTotal      int
	Page             Page
	Items            []Item
	Totals           []CurrencyTotals
	Groups           []Group
	AvailableFilters AvailableFilters
}
