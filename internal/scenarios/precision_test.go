package scenarios

import (
	"encoding/json"
	"github.com/shopspring/decimal"
	"os"
	"testing"
)

func TestInstallmentPrecision(t *testing.T) {
	data, err := os.ReadFile("../../testdata/money_precision.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases struct {
		Shares []struct {
			Value, Share string
			Divisor      int
		}
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases.Shares {
		value := decimal.RequireFromString(c.Value)
		shares := splitEvenly(value, c.Divisor)
		if !shares[0].Equal(decimal.RequireFromString(c.Share)) {
			t.Errorf("first share of %s = %s, want %s", value, shares[0], c.Share)
		}
		total := decimal.Zero
		for _, share := range shares {
			total = total.Add(share)
		}
		if !total.Equal(value) {
			t.Errorf("shares total %s, want %s", total, value)
		}
	}
}
