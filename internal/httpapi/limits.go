package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/shopspring/decimal"
)

// Input limits for user-authored money and dates. The REST handlers and the
// MCP write previews share them, so both entry points accept the same values.
// They do not apply to provider-synced data or to investment quantities and
// unit prices, which keep their own precision.
const (
	// minInputYear and maxInputYear bound a user-typed calendar day
	// (1900-01-01 through 2100-12-31): wide enough for any real record,
	// narrow enough that a mistyped year cannot reach the daily series.
	minInputYear = 1900
	maxInputYear = 2100

	// A money amount is below 10^13 in absolute value and has at most 4
	// decimal places.
	maxMoneyIntegerDigits = 13
	maxMoneyDecimals      = 4
	// maxMoneyScale and maxMoneyBits reject absurd exponents and coefficients
	// before any arithmetic: a decimal like 1e-1000000 must not make the
	// comparison below allocate or loop.
	maxMoneyScale = 64
	maxMoneyBits  = 300
)

const (
	invalidAmountDetail = "Use valores com no máximo 4 casas decimais e abaixo de 10 trilhões."
	invalidDateDetail   = "Use datas entre 1900 e 2100."
	rangeTooLongDetail  = "O período não pode passar de 100 anos."
)

var (
	errInvalidAmount = errors.New(invalidAmountDetail)
	errInvalidDate   = errors.New(invalidDateDetail)
	errRangeTooLong  = errors.New(rangeTooLongDetail)
)

var maxMoneyAbs = decimal.New(1, maxMoneyIntegerDigits)

// validInputDate reports whether the calendar day of t is within
// [1900-01-01, 2100-12-31].
func validInputDate(t time.Time) bool {
	return t.Year() >= minInputYear && t.Year() <= maxInputYear
}

// validMoneyAmount reports whether d is below 10^13 in absolute value and has
// at most 4 decimal places (trailing zeros do not count). Zero is valid here;
// callers that forbid it check separately.
func validMoneyAmount(d decimal.Decimal) bool {
	if exp := d.Exponent(); exp < -maxMoneyScale || exp > maxMoneyIntegerDigits || d.Coefficient().BitLen() > maxMoneyBits {
		return false
	}
	return d.Truncate(maxMoneyDecimals).Equal(d) && d.Abs().LessThan(maxMoneyAbs)
}

// withinInputLimits applies the amount and date limits of a manual money
// movement (a transaction, or one leg of a transfer). Malformed values are
// left to toManualInput's own checks.
func (req manualTransactionRequest) withinInputLimits() bool {
	if !validMoneyAmount(req.Amount) {
		return false
	}
	day, err := time.Parse("2006-01-02", req.OccurredAt)
	return err != nil || validInputDate(day)
}

// writeInvestmentRangeTooLong keeps the investment handlers' problem type.
func writeInvestmentRangeTooLong(w http.ResponseWriter) {
	writeProblem(w, http.StatusUnprocessableEntity, "invalid-investment-input", "Dados de investimento inválidos", rangeTooLongDetail)
}
