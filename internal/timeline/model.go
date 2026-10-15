// Package timeline builds the single financial timeline series every
// presentation layer (the Home's projection and period totals) consumes —
// see .specs/motores-de-dominio.md section 6. Nothing downstream recomputes
// totals locally: BuildSeries is the one place that merges sources into
// Entries, so reconciliation across presentation layers is guaranteed by
// construction rather than by convention.
//
// The three sources and the tiers they produce:
//
//   - real transactions — Realizado, or Confirmado for a credit-card
//     purchase re-dated to its bill's due date;
//   - scenarios — Confirmado for a payable plan's installment (a real
//     debt/receivable with a planned date), Hipotético for a standalone
//     "what if" the caller explicitly selected;
//   - recurring commitments — Projetado, and only for occurrences that no
//     real transaction has reconciled; a reconciled one is already in the
//     series as its real transaction.
//
// This package knows nothing of payables: a plan's link to one stays
// encapsulated in scenarios (Scenario.PayableID/Kind).
package timeline

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/reporting"
	timelinetypes "github.com/greg0x46/julius/internal/timeline/types"
)

// These aliases keep the public timeline API stable while allowing the
// projections package to use the same vocabulary without importing this
// package back and creating a cycle.
type Tier = timelinetypes.Tier
type CertaintyTier = timelinetypes.CertaintyTier

const (
	TierRealizado  = timelinetypes.TierRealizado
	TierConfirmado = timelinetypes.TierConfirmado
	TierProjetado  = timelinetypes.TierProjetado
	TierHipotetico = timelinetypes.TierHipotetico
)

type SourceKind = timelinetypes.SourceKind

const (
	SourceReal        = timelinetypes.SourceReal
	SourceInvestment  = timelinetypes.SourceInvestment
	SourceRecurring   = timelinetypes.SourceRecurring
	SourcePayablePlan = timelinetypes.SourcePayablePlan
	SourceScenario    = timelinetypes.SourceScenario
)

// Entry is one atomic cash-flow item — the one shape every consumer reads,
// never a second parallel representation.
type Entry struct {
	OriginalCurrency         string
	OriginalReportableAmount *decimal.Decimal
	ReportingStatus          string
	ReportingIssue           *reporting.Issue
	Date                     time.Time
	Description              string
	Amount                   decimal.Decimal  // signed: positive = inflow, negative = outflow
	ReportableAmount         *decimal.Decimal // signed income/expense amount; nil uses Amount
	CategoryID               *string          // nil means "Sem categoria" — never dropped from totals
	CategoryName             string
	Tier                     CertaintyTier
	Source                   SourceKind
	SourceRefID              string
	EventKey                 string  // stable identity for projected events; real events use transaction:<id>
	ScenarioID               *string // set for every projected Scenario event
}

// DayPoint is the running balance at the end of one calendar day.
type DayPoint struct {
	Date       time.Time
	Balance    decimal.Decimal
	Inflow     decimal.Decimal
	Outflow    decimal.Decimal
	LowestTier CertaintyTier
}

// Series is BuildSeries' one output — every presentation layer reads from
// this, never recomputing totals independently.
type Series struct {
	Totals           []reporting.Totals
	Reporting        reporting.Summary
	BalanceReporting reporting.Summary
	Points           []DayPoint
	Entries          []Entry
	StartingBalance  decimal.Decimal
	LowestBalance    DayPoint   // minimum from reference onward, or the window's minimum for a wholly historical series
	FirstNegative    *time.Time // first negative point from reference onward; nil for wholly historical windows
}
