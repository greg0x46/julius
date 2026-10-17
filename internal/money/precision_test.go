package money_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/greg0x46/julius/internal/money"
	"github.com/shopspring/decimal"
)

func TestSharedMoneyPrecision(t *testing.T) {
	data, err := os.ReadFile("../../testdata/money_precision.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases struct {
		Rounding []struct{ Value, Rounded string }
		Sums     []struct {
			Values []string
			Total  string
		}
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases.Rounding {
		if got := dec(c.Value).Round(2).StringFixed(2); got != c.Rounded {
			t.Errorf("round(%s) = %s, want %s", c.Value, got, c.Rounded)
		}
		original := dec(c.Value)
		encoded := money.CanonicalDecimal(original)
		if !dec(encoded).Equal(original) || dec(encoded).Exponent() != original.Exponent() {
			t.Errorf("canonical round trip lost precision: %s -> %s", c.Value, encoded)
		}
	}
	for _, c := range cases.Sums {
		total := decimal.Zero
		for _, v := range c.Values {
			total = total.Add(dec(v))
		}
		if !total.Equal(dec(c.Total)) {
			t.Errorf("sum(%v) = %s, want %s", c.Values, total, c.Total)
		}
	}
}
