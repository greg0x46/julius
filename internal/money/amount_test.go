package money

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

func amt(t *testing.T, value, currency string) Amount {
	t.Helper()
	a, err := NewAmount(decimal.RequireFromString(value), currency)
	if err != nil {
		t.Fatalf("NewAmount(%s, %s): %v", value, currency, err)
	}
	return a
}

func TestAmountAddSubRejectCurrencyMismatch(t *testing.T) {
	brl, usd := amt(t, "200", "BRL"), amt(t, "100", "USD")
	for name, op := range map[string]func(Amount) (Amount, error){"Add": brl.Add, "Sub": brl.Sub} {
		got, err := op(usd)
		if !errors.Is(err, ErrCurrencyMismatch) {
			t.Fatalf("%s err = %v, want ErrCurrencyMismatch", name, err)
		}
		if got != (Amount{}) {
			t.Errorf("%s returned %+v alongside the error, want zero Amount", name, got)
		}
		if msg := err.Error(); !strings.Contains(msg, "BRL") || !strings.Contains(msg, "USD") {
			t.Errorf("%s error %q should name both currencies", name, msg)
		}
	}
}

func TestAmountAddSubSameCurrency(t *testing.T) {
	cases := []struct {
		a, b         Amount
		sum, diff    string
		wantCurrency string
	}{
		{amt(t, "200", "BRL"), amt(t, "50.25", "BRL"), "250.25", "149.75", "BRL"},
		{amt(t, "10", "USD"), amt(t, "15", "USD"), "25", "-5", "USD"},
	}
	for _, c := range cases {
		sum, err := c.a.Add(c.b)
		if err != nil || sum.Value.String() != c.sum || sum.Currency != c.wantCurrency {
			t.Errorf("%v + %v = %+v, %v; want %s %s", c.a, c.b, sum, err, c.sum, c.wantCurrency)
		}
		diff, err := c.a.Sub(c.b)
		if err != nil || diff.Value.String() != c.diff || diff.Currency != c.wantCurrency {
			t.Errorf("%v - %v = %+v, %v; want %s %s", c.a, c.b, diff, err, c.diff, c.wantCurrency)
		}
	}
}

func TestAmountZeroAndPrecision(t *testing.T) {
	zero, err := amt(t, "0", "BRL").Add(amt(t, "0", "BRL"))
	if err != nil || !zero.Value.IsZero() {
		t.Errorf("0 BRL + 0 BRL = %+v, %v", zero, err)
	}
	if _, err := amt(t, "0", "USD").Add(amt(t, "0", "BRL")); !errors.Is(err, ErrCurrencyMismatch) {
		t.Errorf("0 USD + 0 BRL err = %v, want ErrCurrencyMismatch even at zero", err)
	}
	sum, _ := amt(t, "0.1", "BRL").Add(amt(t, "0.2", "BRL"))
	if !sum.Value.Equal(decimal.RequireFromString("0.3")) {
		t.Errorf("0.1 + 0.2 = %s, want exactly 0.3", sum.Value)
	}
	sum, _ = amt(t, "0.00000001", "BRL").Add(amt(t, "0.12345678", "BRL"))
	if sum.Value.String() != "0.12345679" {
		t.Errorf("8-place sum = %s", sum.Value)
	}
	sum, _ = amt(t, "999999999999999999.99", "BRL").Add(amt(t, "0.01", "BRL"))
	if sum.Value.String() != "1000000000000000000" {
		t.Errorf("large sum = %s", sum.Value)
	}
}

func TestParseCurrency(t *testing.T) {
	s := func(v string) *string { return &v }
	cases := []struct {
		in   *string
		want error
	}{
		{nil, ErrMissingCurrency},
		{s(""), ErrMissingCurrency},
		{s("  "), ErrMissingCurrency},
		{s("br"), ErrInvalidCurrency},
		{s("BRLL"), ErrInvalidCurrency},
		{s("brl"), ErrInvalidCurrency},
		{s("B1L"), ErrInvalidCurrency},
		{s("R$"), ErrInvalidCurrency},
		{s(" BRL"), ErrInvalidCurrency},
		{s("ABC"), ErrInvalidCurrency},
		{s("ZZZ"), ErrInvalidCurrency},
		{s("BRL"), nil},
		{s("USD"), nil},
		{s("EUR"), nil},
	}
	for _, c := range cases {
		got, err := ParseCurrency(c.in)
		label := "<nil>"
		if c.in != nil {
			label = *c.in
		}
		if c.want == nil {
			if err != nil || got != *c.in {
				t.Errorf("ParseCurrency(%q) = %q, %v; want %q", label, got, err, *c.in)
			}
			continue
		}
		if !errors.Is(err, c.want) {
			t.Errorf("ParseCurrency(%q) err = %v, want %v", label, err, c.want)
		}
	}
	for _, bad := range []string{"", "brl", "XX"} {
		if _, err := NewAmount(decimal.NewFromInt(1), bad); err == nil {
			t.Errorf("NewAmount(1, %q) accepted an invalid currency", bad)
		}
	}
}

func TestBalancesKeepCurrenciesSeparate(t *testing.T) {
	b := Balances{}
	for _, a := range []Amount{amt(t, "200", "BRL"), amt(t, "100", "USD"), amt(t, "50", "BRL")} {
		if err := b.Add(a); err != nil {
			t.Fatalf("Add(%+v): %v", a, err)
		}
	}
	if got := b.Get("BRL").String(); got != "250" {
		t.Errorf("BRL = %s, want 250", got)
	}
	if got := b.Get("USD").String(); got != "100" {
		t.Errorf("USD = %s, want 100", got)
	}
	if got := b.Get("EUR"); !got.IsZero() {
		t.Errorf("EUR = %s, want 0", got)
	}
	if got := b.Currencies(); !reflect.DeepEqual(got, []string{"BRL", "USD"}) {
		t.Errorf("Currencies = %v", got)
	}
	if got := (Balances{}).Currencies(); len(got) != 0 {
		t.Errorf("empty Currencies = %v", got)
	}
}
