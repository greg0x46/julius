package money

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
)

// BRL is the currency every BRL-denominated aggregate (net worth, timeline)
// asks for explicitly.
const BRL = "BRL"

var (
	ErrMissingCurrency  = errors.New("currency code is missing")
	ErrInvalidCurrency  = errors.New("currency code is invalid")
	ErrCurrencyMismatch = errors.New("currency mismatch")
)

// Amount is a decimal value tagged with its currency. Arithmetic only
// combines amounts of the same currency; there is no conversion here.
type Amount struct {
	Value    decimal.Decimal
	Currency string
}

// NewAmount builds an Amount, rejecting a missing or malformed currency.
func NewAmount(value decimal.Decimal, currency string) (Amount, error) {
	code, err := ParseCurrency(&currency)
	if err != nil {
		return Amount{}, err
	}
	return Amount{Value: value, Currency: code}, nil
}

func (a Amount) Add(b Amount) (Amount, error) {
	if a.Currency != b.Currency {
		return Amount{}, mismatch(a, b)
	}
	return Amount{Value: a.Value.Add(b.Value), Currency: a.Currency}, nil
}

func (a Amount) Sub(b Amount) (Amount, error) {
	if a.Currency != b.Currency {
		return Amount{}, mismatch(a, b)
	}
	return Amount{Value: a.Value.Sub(b.Value), Currency: a.Currency}, nil
}

func mismatch(a, b Amount) error {
	return fmt.Errorf("%w: %s and %s", ErrCurrencyMismatch, a.Currency, b.Currency)
}

// ParseCurrency accepts exactly an uppercase ISO 4217 code. It never
// defaults to BRL and never normalizes case.
func ParseCurrency(code *string) (string, error) {
	if code == nil || strings.TrimSpace(*code) == "" {
		return "", ErrMissingCurrency
	}
	c := *code
	if len(c) != 3 {
		return "", fmt.Errorf("%w: %q", ErrInvalidCurrency, c)
	}
	for i := 0; i < len(c); i++ {
		if c[i] < 'A' || c[i] > 'Z' {
			return "", fmt.Errorf("%w: %q", ErrInvalidCurrency, c)
		}
	}
	if _, ok := iso4217[c]; !ok {
		return "", fmt.Errorf("%w: unknown code %q", ErrInvalidCurrency, c)
	}
	return c, nil
}

// Balances holds one total per currency; totals are never mixed.
type Balances map[string]decimal.Decimal

// Add folds a into the total for its currency.
func (b Balances) Add(a Amount) error {
	current := Amount{Value: b[a.Currency], Currency: a.Currency}
	sum, err := current.Add(a)
	if err != nil {
		return err
	}
	b[a.Currency] = sum.Value
	return nil
}

// Get returns the total for currency, zero if there is none.
func (b Balances) Get(currency string) decimal.Decimal {
	if v, ok := b[currency]; ok {
		return v
	}
	return decimal.Zero
}

// Currencies lists the currencies present, sorted.
func (b Balances) Currencies() []string {
	codes := make([]string, 0, len(b))
	for c := range b {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	return codes
}
