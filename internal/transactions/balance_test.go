package transactions_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/transactions"
)

// TestCashOnHandExcludesCreditAccounts covers the reason this figure is not
// just "sum every balance": a credit card's balance is debt owed, counted on
// the liability side by CreditCardTransactionTotal instead. An account whose
// provider never reported an account_type is cash, not credit.
func TestCashOnHandExcludesCreditAccounts(t *testing.T) {
	f := newFixture(t)
	f.addAccount(account{AccountType: strp("BANK"), CurrencyCode: strp("BRL"), Balance: strp("1500.00")})
	f.addAccount(account{AccountType: nil, CurrencyCode: strp("BRL"), Balance: strp("250.50")})
	f.addAccount(account{AccountType: strp("CREDIT"), CurrencyCode: strp("BRL"), Balance: strp("-900.00")})

	total, err := transactions.CashOnHandIn(context.Background(), f.conn, nil, "BRL")
	if err != nil {
		t.Fatalf("CashOnHandIn: %v", err)
	}
	if total.String() != "1750.5" {
		t.Errorf("CashOnHandIn = %s, want 1750.5 (the CREDIT account's balance is debt, not cash)", total)
	}
}

// TestCashOnHandScopedToAccountIDsFiltersUntypedAccounts guards the SQL
// precedence trap the query's comment calls out: without the parentheses
// around the account_type OR, an account with a NULL account_type binds to
// the wrong side of the AND and escapes the id filter entirely.
func TestCashOnHandScopedToAccountIDsFiltersUntypedAccounts(t *testing.T) {
	f := newFixture(t)
	wanted := f.addAccount(account{AccountType: strp("BANK"), CurrencyCode: strp("BRL"), Balance: strp("100.00")})
	f.addAccount(account{AccountType: nil, CurrencyCode: strp("BRL"), Balance: strp("9999.00")})

	total, err := transactions.CashOnHandIn(context.Background(), f.conn, []string{wanted}, "BRL")
	if err != nil {
		t.Fatalf("CashOnHandIn: %v", err)
	}
	if total.String() != "100" {
		t.Errorf("CashOnHandIn = %s, want 100 — the untyped account is outside the id filter", total)
	}
}

// TestCashOnHandIgnoresAccountsWithoutBalance pins that an account the
// provider has not reported a balance for contributes nothing, rather than
// failing the whole sum on an unparseable empty string.
func TestCashOnHandIgnoresAccountsWithoutBalance(t *testing.T) {
	f := newFixture(t)
	f.addAccount(account{AccountType: strp("BANK"), CurrencyCode: strp("BRL"), Balance: strp("42.00")})
	f.addAccount(account{AccountType: strp("BANK"), CurrencyCode: strp("BRL"), Balance: nil})

	total, err := transactions.CashOnHandIn(context.Background(), f.conn, nil, "BRL")
	if err != nil {
		t.Fatalf("CashOnHandIn: %v", err)
	}
	if total.String() != "42" {
		t.Errorf("CashOnHandIn = %s, want 42", total)
	}
}

// TestCashOnHandIsZeroWithNoAccounts pins the empty case: no rows is zero,
// not an error — the timeline anchors on this before any sync has run.
func TestCashOnHandIsZeroWithNoAccounts(t *testing.T) {
	f := newFixture(t)
	total, err := transactions.CashOnHandIn(context.Background(), f.conn, nil, "BRL")
	if err != nil {
		t.Fatalf("CashOnHandIn: %v", err)
	}
	if !total.IsZero() {
		t.Errorf("CashOnHandIn = %s, want 0", total)
	}
	byCurrency, err := transactions.CashOnHandByCurrency(context.Background(), f.conn, nil)
	if err != nil || len(byCurrency) != 0 {
		t.Errorf("CashOnHandByCurrency = %v, %v; want empty", byCurrency, err)
	}
}

// TestCashOnHandKeepsCurrenciesSeparate is the bug this split exists for:
// BRL 200 and USD 100 must never become 300 of anything.
func TestCashOnHandKeepsCurrenciesSeparate(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.addAccount(account{AccountType: strp("BANK"), CurrencyCode: strp("BRL"), Balance: strp("150.00")})
	f.addAccount(account{AccountType: strp("BANK"), CurrencyCode: strp("BRL"), Balance: strp("50.00")})
	f.addAccount(account{AccountType: strp("BANK"), CurrencyCode: strp("USD"), Balance: strp("100.00")})

	byCurrency, err := transactions.CashOnHandByCurrency(ctx, f.conn, nil)
	if err != nil {
		t.Fatalf("CashOnHandByCurrency: %v", err)
	}
	if got := byCurrency.Currencies(); !reflect.DeepEqual(got, []string{"BRL", "USD"}) {
		t.Fatalf("currencies = %v, want [BRL USD]", got)
	}
	if byCurrency.Get("BRL").String() != "200" || byCurrency.Get("USD").String() != "100" {
		t.Errorf("CashOnHandByCurrency = %v, want BRL 200 and USD 100", byCurrency)
	}

	for currency, want := range map[string]string{"BRL": "200", "USD": "100", "EUR": "0"} {
		got, err := transactions.CashOnHandIn(ctx, f.conn, nil, currency)
		if err != nil {
			t.Fatalf("CashOnHandIn(%s): %v", currency, err)
		}
		if got.String() != want {
			t.Errorf("CashOnHandIn(%s) = %s, want %s", currency, got, want)
		}
	}
}

// TestCashOnHandInRejectsInvalidRequestedCurrency pins that the scalar
// accessor validates the currency it is asked for.
func TestCashOnHandInRejectsInvalidRequestedCurrency(t *testing.T) {
	f := newFixture(t)
	for _, currency := range []string{"", "brl", "XX"} {
		if _, err := transactions.CashOnHandIn(context.Background(), f.conn, nil, currency); err == nil {
			t.Errorf("CashOnHandIn(%q) accepted an invalid currency", currency)
		}
	}
}

// TestCashOnHandFailsOnMissingOrInvalidAccountCurrency pins the fail-closed
// rule: a cash account with a balance but no usable currency is an error
// naming the account, never silently treated as BRL.
func TestCashOnHandFailsOnMissingOrInvalidAccountCurrency(t *testing.T) {
	cases := []struct {
		name     string
		currency *string
		want     error
	}{
		{"null", nil, money.ErrMissingCurrency},
		{"unknown", strp("ABC"), money.ErrInvalidCurrency},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			f.addAccount(account{AccountType: strp("BANK"), CurrencyCode: strp("BRL"), Balance: strp("10")})
			bad := f.addAccount(account{AccountType: strp("BANK"), CurrencyCode: c.currency, Balance: strp("5")})

			_, err := transactions.CashOnHandByCurrency(context.Background(), f.conn, nil)
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
			if !strings.Contains(err.Error(), bad) {
				t.Errorf("err %q does not name account %s", err, bad)
			}
			if _, err := transactions.CashOnHandIn(context.Background(), f.conn, nil, "BRL"); !errors.Is(err, c.want) {
				t.Errorf("CashOnHandIn err = %v, want %v", err, c.want)
			}
		})
	}
}

// TestCashOnHandSkipsCurrencyCheckForExcludedAccounts pins that accounts
// filtered out before currency is looked at — CREDIT, NULL balance, or
// outside the accountIDs scope — never trigger the currency error.
func TestCashOnHandSkipsCurrencyCheckForExcludedAccounts(t *testing.T) {
	f := newFixture(t)
	wanted := f.addAccount(account{AccountType: strp("BANK"), CurrencyCode: strp("BRL"), Balance: strp("10")})
	f.addAccount(account{AccountType: strp("CREDIT"), CurrencyCode: nil, Balance: strp("-50")})
	f.addAccount(account{AccountType: strp("BANK"), CurrencyCode: nil, Balance: nil})

	total, err := transactions.CashOnHandIn(context.Background(), f.conn, nil, "BRL")
	if err != nil || total.String() != "10" {
		t.Fatalf("CashOnHandIn = %s, %v; want 10", total, err)
	}

	f.addAccount(account{AccountType: strp("BANK"), CurrencyCode: strp("ABC"), Balance: strp("99")})
	total, err = transactions.CashOnHandIn(context.Background(), f.conn, []string{wanted}, "BRL")
	if err != nil || total.String() != "10" {
		t.Fatalf("scoped CashOnHandIn = %s, %v; want 10 (bad account is out of scope)", total, err)
	}
}

// TestCashOnHandPreservesDecimalPrecision guards against any float path.
func TestCashOnHandPreservesDecimalPrecision(t *testing.T) {
	f := newFixture(t)
	f.addAccount(account{AccountType: strp("BANK"), CurrencyCode: strp("BRL"), Balance: strp("0.10")})
	f.addAccount(account{AccountType: strp("BANK"), CurrencyCode: strp("BRL"), Balance: strp("0.20")})
	f.addAccount(account{AccountType: strp("BANK"), CurrencyCode: strp("USD"), Balance: strp("1.005")})
	f.addAccount(account{AccountType: strp("BANK"), CurrencyCode: strp("EUR"), Balance: strp("0")})

	byCurrency, err := transactions.CashOnHandByCurrency(context.Background(), f.conn, nil)
	if err != nil {
		t.Fatalf("CashOnHandByCurrency: %v", err)
	}
	if got := byCurrency.Get("BRL").String(); got != "0.3" {
		t.Errorf("BRL = %s, want exactly 0.3", got)
	}
	if got := byCurrency.Get("USD").String(); got != "1.005" {
		t.Errorf("USD = %s, want 1.005", got)
	}
	if got, ok := byCurrency["EUR"]; !ok || !got.IsZero() {
		t.Errorf("EUR = %s (present=%v), want a zero entry", got, ok)
	}
}
