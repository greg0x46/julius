package httpapi

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestValidMoneyAmount(t *testing.T) {
	for _, tc := range []struct {
		amount string
		want   bool
	}{
		{"0", true},
		{"1", true},
		{"-1", true},
		{"0.0001", true},
		{"-0.0001", true},
		{"123.4500", true},
		{"1.230000", true}, // trailing zeros are not decimal places
		{"9999999999999", true},
		{"-9999999999999", true},
		{"9999999999999.9999", true},
		{"-9999999999999.9999", true},
		{"10000000000000", false},
		{"-10000000000000", false},
		{"10000000000000.5", false},
		{"0.00001", false},
		{"-0.00001", false},
		{"0.005", true}, // three places is within the four allowed
		{"1.00001", false},
		{"-1E-7", false},
		{"1e12", true},
		{"1e13", false},
		{"1e400", false},
		{"-1e400", false},
		{"1e-400", false},
		{"0e5000", false},
		{"1e-9999", false},
		{"9e9999", false},
		{"123456789012345678901234567890.1234", false},
	} {
		d, err := decimal.NewFromString(tc.amount)
		if err != nil {
			t.Fatalf("%s: %v", tc.amount, err)
		}
		if got := validMoneyAmount(d); got != tc.want {
			t.Errorf("validMoneyAmount(%s) = %v, want %v", tc.amount, got, tc.want)
		}
	}
	if !validMoneyAmount(decimal.Decimal{}) {
		t.Error("the zero Decimal must be valid")
	}
}

func TestValidInputDate(t *testing.T) {
	day := func(year int, month time.Month, d int) time.Time {
		return time.Date(year, month, d, 12, 0, 0, 0, time.UTC)
	}
	for _, tc := range []struct {
		name string
		date time.Time
		want bool
	}{
		{"lower bound", day(1900, time.January, 1), true},
		{"upper bound", day(2100, time.December, 31), true},
		{"today", day(2026, time.October, 31), true},
		{"before lower bound", day(1899, time.December, 31), false},
		{"after upper bound", day(2101, time.January, 1), false},
		{"mistyped year", day(202, time.March, 1), false},
		{"year one", day(1, time.January, 1), false},
		{"year 9999", day(9999, time.December, 31), false},
	} {
		if got := validInputDate(tc.date); got != tc.want {
			t.Errorf("%s: validInputDate(%s) = %v, want %v", tc.name, tc.date.Format(time.DateOnly), got, tc.want)
		}
	}
}

func TestManualTransactionRequestToManualInputEnforcesLimits(t *testing.T) {
	valid := manualTransactionRequest{
		AccountID: "account", Description: "Feira", Amount: decimal.RequireFromString("-42.5"), OccurredAt: "2026-03-01",
	}
	if _, ok := valid.toManualInput(); !ok {
		t.Fatal("a valid request was rejected")
	}
	for _, tc := range []struct {
		name   string
		mutate func(*manualTransactionRequest)
	}{
		{"huge amount", func(r *manualTransactionRequest) { r.Amount = decimal.RequireFromString("1e400") }},
		{"at the integer cap", func(r *manualTransactionRequest) { r.Amount = decimal.RequireFromString("10000000000000") }},
		{"negative tiny exponent", func(r *manualTransactionRequest) { r.Amount = decimal.RequireFromString("-1E-7") }},
		{"sub-cent beyond four places", func(r *manualTransactionRequest) { r.Amount = decimal.RequireFromString("0.00005") }},
		{"year before 1900", func(r *manualTransactionRequest) { r.OccurredAt = "1899-12-31" }},
		{"mistyped year", func(r *manualTransactionRequest) { r.OccurredAt = "0202-03-01" }},
		{"year after 2100", func(r *manualTransactionRequest) { r.OccurredAt = "2101-01-01" }},
		{"year 9999", func(r *manualTransactionRequest) { r.OccurredAt = "9999-12-31" }},
	} {
		req := valid
		tc.mutate(&req)
		if _, ok := req.toManualInput(); ok {
			t.Errorf("%s: accepted", tc.name)
		}
	}
	for _, tc := range []struct {
		name   string
		mutate func(*manualTransactionRequest)
	}{
		{"largest amount", func(r *manualTransactionRequest) { r.Amount = decimal.RequireFromString("-9999999999999.9999") }},
		{"smallest amount", func(r *manualTransactionRequest) { r.Amount = decimal.RequireFromString("0.0001") }},
		{"first day", func(r *manualTransactionRequest) { r.OccurredAt = "1900-01-01" }},
		{"last day", func(r *manualTransactionRequest) { r.OccurredAt = "2100-12-31" }},
	} {
		req := valid
		tc.mutate(&req)
		if _, ok := req.toManualInput(); !ok {
			t.Errorf("%s: rejected", tc.name)
		}
	}
}
