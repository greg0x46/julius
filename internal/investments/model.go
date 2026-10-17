// Package investments owns the local investment ledger. Provider holdings
// stay in financial_investments; manual holdings, their custody cash, goals
// and the links that explain a bank movement live here.
package investments

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/shopspring/decimal"
)

// Querier is implemented by *sql.DB and *sql.Tx. Keeping this package on the
// standard database shape lets reporting packages consume its helpers without
// a dependency in the other direction.
type Querier interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

const DateLayout = "2006-01-02"

type AccountKind string

const (
	AccountKindManual     AccountKind = "manual"
	AccountKindIntegrated AccountKind = "integrated"
)

type PositionSource string

const (
	PositionSourceManual PositionSource = "manual"
	PositionSourceSynced PositionSource = "synced"
)

type ValuationBasis string

const (
	ValuationBasisManualValuation ValuationBasis = "manual_valuation"
	ValuationBasisCostBasis       ValuationBasis = "cost_basis"
	ValuationBasisProviderBalance ValuationBasis = "provider_balance"
	// ValuationBasisMarketQuote is a manual holding valued at its asset's
	// latest price in the quote series, derived on read.
	ValuationBasisMarketQuote ValuationBasis = "market_quote"
)

type OperationKind string

const (
	OperationInitialBalance OperationKind = "initial_balance"
	OperationDeposit        OperationKind = "deposit"
	OperationWithdrawal     OperationKind = "withdrawal"
	OperationBuy            OperationKind = "buy"
	OperationSell           OperationKind = "sell"
	OperationIncome         OperationKind = "income"
	OperationFee            OperationKind = "fee"
	OperationTax            OperationKind = "tax"
	OperationValuation      OperationKind = "valuation"
	OperationTransferOut    OperationKind = "transfer_out"
	OperationTransferIn     OperationKind = "transfer_in"
)

func (k OperationKind) Valid() bool {
	switch k {
	case OperationInitialBalance, OperationDeposit, OperationWithdrawal,
		OperationBuy, OperationSell, OperationIncome, OperationFee,
		OperationTax, OperationValuation, OperationTransferOut, OperationTransferIn:
		return true
	default:
		return false
	}
}

type Account struct {
	ID                 string
	Name               string
	Kind               AccountKind
	CurrencyCode       *string
	SourceID           *string
	SourceDisplayName  *string
	FinancialAccountID *string
	Active             bool
	// CashBalance is the local ledger balance. LinkedCashBalance is the
	// provider's authoritative balance when a manual custody account is
	// linked to a financial account.
	CashBalance       decimal.Decimal
	LinkedCashBalance *decimal.Decimal
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type Portfolio struct {
	ID           string
	Name         string
	TargetAmount *decimal.Decimal
	TargetDate   *time.Time
	Notes        *string
	CurrentValue decimal.Decimal
	Progress     *decimal.Decimal
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Asset identifies an economic instrument independently from its custody.
// CanonicalKey is internal identity material (for example ticker:BTC); API
// callers use ID and cannot accidentally create one asset per account.
type Asset struct {
	ID           string
	CanonicalKey string
	Name         string
	Ticker       *string
	AssetType    string
	AssetClass   AssetClass
	CurrencyCode string
	// QuoteSource stores the market ("b3" or "crypto"); QuoteSymbol stores
	// its provider-neutral symbol ("PETR4" or "BTC"). Only relevant for manual
	// positions of this asset — a synced position already carries its own
	// provider quote and never reads these.
	QuoteSource *string
	QuoteSymbol *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type AssetInput struct {
	Name         string
	Ticker       *string
	AssetType    string
	AssetClass   AssetClass
	CurrencyCode string
	QuoteSource  *string
	QuoteSymbol  *string
}

type Position struct {
	ID          string
	Source      PositionSource
	AccountID   string
	AssetID     *string
	PortfolioID *string
	Name        string
	Ticker      *string
	AssetType   string
	Quantity    decimal.Decimal
	AverageCost *decimal.Decimal
	// TotalCost is the exact cost basis of the units held. AverageCost is
	// derived from it for display; gains must subtract TotalCost, not
	// Quantity × AverageCost, or a non-terminating unit cost drifts.
	TotalCost          *decimal.Decimal
	CurrentValue       decimal.Decimal
	CurrentUnitPrice   *decimal.Decimal
	ValuedOn           *time.Time
	ValuationBasis     ValuationBasis
	CurrencyCode       *string
	Closed             bool
	LinkedInvestmentID *string
	Notes              *string
	CreatedAt          *time.Time
	UpdatedAt          *time.Time
}

type Operation struct {
	ID         string
	AccountID  string
	PositionID *string
	TransferID *string
	Kind       OperationKind
	OccurredOn time.Time
	Amount     decimal.Decimal
	Quantity   *decimal.Decimal
	UnitPrice  *decimal.Decimal
	Fees       decimal.Decimal
	Taxes      decimal.Decimal
	Notes      *string
	Source     string
	IsEditable bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Reconciliation struct {
	ID                               string
	OperationID                      string
	FinancialTransactionID           *string
	FinancialInvestmentTransactionID *string
	Amount                           decimal.Decimal
	CreatedAt                        time.Time
}

type AccountInput struct {
	Name               string
	FinancialAccountID *string
	// Active is the "still in use" flag the edit form toggles. nil keeps the
	// current value, so a rename does not have to know the flag.
	Active *bool
}

type PortfolioInput struct {
	Name         string
	TargetAmount *decimal.Decimal
	TargetDate   *time.Time
	Notes        *string
}

type PositionInput struct {
	AccountID       string
	AssetID         *string
	Name            string
	Ticker          *string
	AssetType       string
	PortfolioID     *string
	InitialQuantity decimal.Decimal
	InitialUnitCost decimal.Decimal
	// InitialValue opens a holding the caller only knows the amount of, such
	// as a fixed income application with no unit count.
	InitialValue *decimal.Decimal
	OccurredOn   time.Time
	Notes        *string
}

type PositionUpdate struct {
	Name        string
	Ticker      *string
	AssetType   string
	PortfolioID *string
	Notes       *string
}

type OperationInput struct {
	AccountID  string
	PositionID *string
	Kind       OperationKind
	OccurredOn time.Time
	Amount     decimal.Decimal
	Quantity   *decimal.Decimal
	UnitPrice  *decimal.Decimal
	Fees       decimal.Decimal
	Taxes      decimal.Decimal
	Notes      *string
}

type TransferInput struct {
	SourcePositionID      string
	DestinationPositionID string
	Quantity              decimal.Decimal
	OccurredOn            time.Time
	Notes                 *string
}

type ReconciliationInput struct {
	// OperationID may be empty when FinancialInvestmentTransactionID is set:
	// the provider movement then becomes the pivot through a derived
	// source='synced' operation on its integrated custody.
	OperationID                      string
	FinancialTransactionID           *string
	FinancialInvestmentTransactionID *string
	Amount                           decimal.Decimal
}

type SummaryAccount struct {
	AccountID    string
	Name         string
	Kind         AccountKind
	CurrentValue decimal.Decimal
	CashBalance  decimal.Decimal
}

type SummaryPortfolio struct {
	PortfolioID  *string
	Name         string
	CurrentValue decimal.Decimal
	TargetAmount *decimal.Decimal
	Progress     *decimal.Decimal
}

type Summary struct {
	CurrencyCode   string
	TotalValue     decimal.Decimal
	ManualValue    decimal.Decimal
	SyncedValue    decimal.Decimal
	CashBalance    decimal.Decimal
	UnrealizedGain decimal.Decimal
	Portfolios     []SummaryPortfolio
	Accounts       []SummaryAccount
}

// ManualReportingEntry is a dated reporting-only movement. Amount is signed:
// deposits/income are positive; withdrawals, fees and taxes are negative.
// It never changes the cash curve, which is why consumers must keep it
// distinct from actual financial_transactions.
type ManualReportingEntry struct {
	ID         string
	AccountID  string
	Kind       OperationKind
	OccurredOn time.Time
	Amount     decimal.Decimal
}

// MonthlyMovement is one calendar month (YYYY-MM) of gross local investment flows.
// Contributions and Withdrawals are principal, never income or expense.
// RealizedGain (negative for a loss) is the net proceeds of local sales minus
// the all-in cost basis written off, so it is net of purchase and sale fees
// and taxes and is not part of Income. Synced holdings have no cost basis:
// their sales are not covered and a zero is not "no gain".
type MonthlyMovement struct {
	Month         string
	Contributions decimal.Decimal
	Withdrawals   decimal.Decimal
	Income        decimal.Decimal
	Fees          decimal.Decimal
	Taxes         decimal.Decimal
	RealizedGain  decimal.Decimal
}

type ManualReporting struct {
	Income decimal.Decimal
	Fees   decimal.Decimal
	Taxes  decimal.Decimal
}

var (
	ErrNotFound                  = errors.New("investment record not found")
	ErrAccountNotFound           = errors.New("investment account not found")
	ErrPortfolioNotFound         = errors.New("investment portfolio not found")
	ErrPositionNotFound          = errors.New("investment position not found")
	ErrAssetNotFound             = errors.New("investment asset not found")
	ErrAssetAlreadyExists        = errors.New("investment asset already exists")
	ErrAssetHasPositions         = errors.New("investment asset has positions")
	ErrPositionAlreadyExists     = errors.New("investment position already exists for account and asset")
	ErrTransferAtomic            = errors.New("investment transfer must be changed as one atomic pair")
	ErrOperationNotFound         = errors.New("investment operation not found")
	ErrReconciliationNotFound    = errors.New("investment reconciliation not found")
	ErrInvalidInput              = errors.New("invalid investment input")
	ErrUnsupportedCurrency       = errors.New("manual investment records are in reais")
	ErrIntegratedReadOnly        = errors.New("integrated investment data is read-only")
	ErrNotManual                 = errors.New("investment record is not manual")
	ErrNegativeCash              = errors.New("investment operation would make cash negative")
	ErrNegativePosition          = errors.New("investment operation would make a position negative")
	ErrPositionHasOperations     = errors.New("investment position has operations")
	ErrAccountHasPositions       = errors.New("investment account has positions")
	ErrAccountHasOperations      = errors.New("investment account has operations")
	ErrFinancialAccountLinked    = errors.New("financial account is already linked to another investment account")
	ErrReconciliationConflict    = errors.New("investment reconciliation exceeds available amount")
	ErrReconciliationDuplicate   = errors.New("investment reconciliation is already linked")
	ErrInvalidReconciliationLink = errors.New("invalid investment reconciliation link")
)

func Day(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
