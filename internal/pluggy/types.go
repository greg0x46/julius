// Package pluggy ports app/providers/base.py, app/providers/pluggy.py, and
// app/sync/mappings.py: the provider-agnostic snapshot types, the Pluggy
// HTTP adapter (auth, retry, rate limiting, pagination), and the JSON
// mapping from Pluggy's wire format into those snapshots. Kept as one
// package because the three are tightly coupled (the adapter calls the
// mapper directly, as the reference does) and splitting them further would
// only add import ceremony.
package pluggy

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

// Scope is which part of a sync run a raw HTTP response belongs to.
type Scope string

const (
	ScopeItem                   Scope = "item"
	ScopeAccounts               Scope = "accounts"
	ScopeTransactions           Scope = "transactions"
	ScopeInvestments            Scope = "investments"
	ScopeInvestmentTransactions Scope = "investment_transactions"
	ScopeBills                  Scope = "bills"
)

// FailureStage mirrors the sync_failures.stage CHECK constraint.
type FailureStage string

const (
	StageAuth                   FailureStage = "auth"
	StageItem                   FailureStage = "item"
	StageAccounts               FailureStage = "accounts"
	StageAccount                FailureStage = "account"
	StageTransactions           FailureStage = "transactions"
	StageInvestments            FailureStage = "investments"
	StageInvestment             FailureStage = "investment"
	StageInvestmentTransactions FailureStage = "investment_transactions"
	StageBills                  FailureStage = "bills"
	StageNormalize              FailureStage = "normalize"
	StageInterrupted            FailureStage = "interrupted"
	StageWorkerUnavailable      FailureStage = "worker_unavailable"
)

// SourceSnapshot mirrors SourceSnapshot: the Pluggy "Item" (a bank
// connection), reduced to what sync needs to decide whether it's safe to
// import from.
type SourceSnapshot struct {
	ExternalItemID          string
	InstitutionName         *string
	ProviderStatus          *string
	ProviderExecutionStatus *string
	ProviderUpdatedAt       *time.Time
	// Warnings are non-fatal issues (e.g. a partial-success item) recorded
	// as sync_failures rather than aborting the run.
	Warnings []string
	// SafeProducts is which of ACCOUNTS/TRANSACTIONS are safe to read this
	// run — a partial-success item may have refreshed only one.
	SafeProducts map[string]bool
}

// AccountSnapshot mirrors AccountSnapshot.
type AccountSnapshot struct {
	ExternalID           string
	Institution          *string
	Name                 *string
	Number               *string
	AccountType          *string
	AccountSubtype       *string
	Balance              *decimal.Decimal
	CreditLimit          *decimal.Decimal
	AvailableCreditLimit *decimal.Decimal
	BalanceCloseDate     *time.Time
	BalanceDueDate       *time.Time
	CurrencyCode         *string
	ProviderUpdatedAt    *time.Time
}

// TransactionSnapshot mirrors TransactionSnapshot. PaymentData,
// CreditCardMetadata, and Merchant are stored as opaque JSON — nothing in
// this app's domain logic (money/categories/transactions/debts) reads their
// nested fields, so unlike the reference's fully-typed nested dataclasses,
// this is a deliberate pass-through: validated only enough to confirm it's
// an object or null, re-serialized canonically for the change-detection
// hash, and stored verbatim for display.
type TransactionSnapshot struct {
	ExternalID                  string
	ExternalAccountID           string
	Description                 *string
	DescriptionRaw              *string
	Amount                      *decimal.Decimal
	AmountInAccountCurrency     *decimal.Decimal
	BalanceAfter                *decimal.Decimal
	CurrencyCode                *string
	OccurredAt                  *time.Time
	ProviderStatus              *string
	MovementType                *string
	SourceCategory              *string
	SourceCategoryID            *string
	ProviderCode                *string
	PaymentData                 []byte // canonical JSON object or nil
	CreditCardMetadata          []byte
	Merchant                    []byte
	OperationType               *string
	OperationTypeAdditionalInfo *string
	OperationCategory           *string
	ProviderID                  *string
	ProviderOrder               *int
	ProviderCreatedAt           *time.Time
	ProviderUpdatedAt           *time.Time
}

// InvestmentSnapshot mirrors Pluggy's Investment resource: a holding (fund,
// fixed income, equity, ...) reduced to what sync needs to detect changes.
type InvestmentSnapshot struct {
	ExternalID           string
	InvestmentType       *string // "type": MUTUAL_FUND, FIXED_INCOME, EQUITY, SECURITY, COE, ETF
	Subtype              *string
	Name                 *string
	Balance              *decimal.Decimal
	CurrencyCode         *string
	Number               *string
	Owner                *string
	TaxNumber            *string
	DueDate              *time.Time
	Issuer               *string
	IssuerCode           *string
	Rate                 *decimal.Decimal
	RateType             *string
	FixedAnnualRate      *decimal.Decimal
	AnnualRate           *decimal.Decimal
	LastTwelveMonthsRate *decimal.Decimal
	Quantity             *decimal.Decimal
	Value                *decimal.Decimal
	Amount               *decimal.Decimal
	AmountProfit         *decimal.Decimal
	AmountWithdrawal     *decimal.Decimal
	// AmountOriginal is the principal originally applied. Taxes is Pluggy's
	// "taxes" (IR provisioned) and Taxes2 its "taxes2" (IOF, regressive,
	// nonzero only in the holding's first 30 days) — both empirically
	// verified against real Nubank FIXED_INCOME data; Pluggy never populates
	// any of the three for EQUITY.
	AmountOriginal *decimal.Decimal
	Taxes          *decimal.Decimal
	Taxes2         *decimal.Decimal
	AsOfDate       *time.Time
	// IssueDate, PurchaseDate and IssuerCNPJ identify a fixed income title
	// when the provider sends no code/isin: two holdings with the same
	// issuer, rate, issue and due date carry the same unit price (PU), so
	// they are the same instrument. See investments.FixedIncomeCode.
	IssueDate         *time.Time
	PurchaseDate      *time.Time
	IssuerCNPJ        *string
	ProviderUpdatedAt *time.Time
	ISIN              *string
	Code              *string
	ProviderStatus    *string
}

// InvestmentTransactionSnapshot mirrors one movement (buy/sell/dividend/...)
// within an investment's history.
type InvestmentTransactionSnapshot struct {
	ExternalID           string
	ExternalInvestmentID string
	MovementType         *string // "type": BUY/SELL/... — the provider's own label, kept for display
	Direction            *string // normalized from "movementType": "inflow" | "outflow"
	Quantity             *decimal.Decimal
	Value                *decimal.Decimal
	Amount               *decimal.Decimal
	OccurredAt           *time.Time
	TradeDate            *time.Time
}

// InvestmentsPage is one page of mapped investments (Pluggy returns
// investments unpaginated, so there is only ever one).
type InvestmentsPage struct {
	RawImportID string
	Investments []InvestmentSnapshot
	Rejections  []RejectedRecord
}

// InvestmentTransactionsPage is the mapped transaction history for a single
// investment, accumulated across every page the provider returned.
type InvestmentTransactionsPage struct {
	RawImportID  string
	Transactions []InvestmentTransactionSnapshot
	Rejections   []RejectedRecord
}

// BillSnapshot mirrors one entry from Pluggy's Bill entity: a credit card
// invoice/fatura. Pluggy only returns bills that have already closed or gone
// overdue — the currently open bill is never listed, so this is always
// historical/settled data.
type BillSnapshot struct {
	ExternalID           string
	DueDate              *time.Time
	ClosingDate          *time.Time
	TotalAmount          *decimal.Decimal
	CurrencyCode         *string
	MinimumPaymentAmount *decimal.Decimal
}

// BillsPage is every mapped bill for a single credit card account — GetBills
// already collapses Pluggy's page/totalPages pagination into one slice.
type BillsPage struct {
	RawImportID string
	Bills       []BillSnapshot
	Rejections  []RejectedRecord
}

// RawResponseEnvelope mirrors RawResponseEnvelope: a raw HTTP response as it
// must be persisted to raw_imports before any of it is trusted.
type RawResponseEnvelope struct {
	Scope             Scope
	Method            string
	Path              string
	StatusCode        int
	Headers           map[string]string
	Content           []byte
	PageSequence      int
	RequestAttempt    int
	ExternalAccountID *string
	ReceivedAt        time.Time
}

// RejectedRecord mirrors RejectedRecord: one record within a page that
// failed to map, recorded as a sync_failure without aborting the rest of
// the page.
type RejectedRecord struct {
	EntityType  string // "account" | "transaction" | "investment" | "investment_transaction"
	ExternalID  *string
	Code        string
	SafeMessage string
}

// AccountsPage is one page of mapped accounts (Pluggy returns accounts
// unpaginated, so there is only ever one).
type AccountsPage struct {
	RawImportID string
	Accounts    []AccountSnapshot
	Rejections  []RejectedRecord
}

// TransactionsPage is one page of mapped transactions for a single account.
type TransactionsPage struct {
	RawImportID  string
	Transactions []TransactionSnapshot
	Rejections   []RejectedRecord
	NextCursor   *string
}

// ProviderError mirrors ProviderError: every error this package raises is
// one of these, carrying the sync_failures fields needed to record it.
type ProviderError struct {
	Code              string
	Stage             FailureStage
	SafeMessage       string
	RawImportID       *string
	ExternalAccountID *string
}

func (e *ProviderError) Error() string { return e.SafeMessage }

// RawEnvelopeWriter mirrors RawEnvelopeWriter: persists a raw response
// before it is trusted, returning the raw_imports row id.
type RawEnvelopeWriter interface {
	Write(ctx context.Context, envelope RawResponseEnvelope) (rawImportID string, err error)
}
