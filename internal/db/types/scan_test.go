package types

import (
	"database/sql"
	"github.com/shopspring/decimal"
	"testing"
	"time"
)

func TestScanners(t *testing.T) {
	stamp := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	var s string
	var ns sql.NullString
	var ps *string
	for _, dest := range []any{&s, &ns, &ps} {
		if err := Time(dest).Scan(stamp); err != nil {
			t.Fatal(err)
		}
	}
	if s != stamp.Format(TimeLayout) || !ns.Valid || ps == nil || *ps != s {
		t.Fatal("timestamp text destinations", s, ns, ps)
	}
	var tm time.Time
	var pt *time.Time
	var nt sql.NullTime
	for _, dest := range []any{&tm, &pt, &nt} {
		if err := Date(dest).Scan(stamp); err != nil {
			t.Fatal(err)
		}
	}
	if tm.Hour() != 0 || pt == nil || !nt.Valid {
		t.Fatal("date destinations", tm, pt, nt)
	}
	var b bool
	var pb *bool
	var nb sql.NullBool
	var i int
	for _, dest := range []any{&b, &pb, &nb, &i} {
		if err := Bool(dest).Scan("true"); err != nil {
			t.Fatal(err)
		}
	}
	if !b || pb == nil || !*pb || !nb.Valid || !nb.Bool || i != 1 {
		t.Fatal("boolean destinations")
	}
	if err := Bool(&i).Scan(false); err != nil || i != 0 {
		t.Fatal(i, err)
	}
	var d decimal.Decimal
	var pd *decimal.Decimal
	for _, dest := range []any{&d, &pd, &s, &ns, &ps} {
		if err := Decimal(dest).Scan([]byte("12.30")); err != nil {
			t.Fatal(err)
		}
	}
	if d.String() != "12.3" || pd == nil || pd.String() != d.String() || s != "12.30" {
		t.Fatal("decimal destinations")
	}
	for _, dest := range []any{&s, &ns, &ps} {
		if err := ID(dest).Scan("integrated:source"); err != nil {
			t.Fatal(err)
		}
	}
	if s != "integrated:source" {
		t.Fatal(s)
	}
	for _, dest := range []any{&ns, &ps, &pt, &nt, &pb, &nb, &pd} {
		if err := Time(dest).Scan(nil); err != nil {
			t.Fatal(err)
		}
	}
	if ns.Valid || ps != nil || pt != nil || nt.Valid || pb != nil || nb.Valid || pd != nil {
		t.Fatal("null destinations were not cleared")
	}
}
func TestScannerErrors(t *testing.T) {
	var s string
	var b bool
	var d decimal.Decimal
	for _, sc := range []sql.Scanner{Time(&s), Date(&s), ID(&s), Bool(&b), Decimal(&d)} {
		if err := sc.Scan(struct{}{}); err == nil {
			t.Errorf("accepted invalid source")
		}
		if err := sc.Scan(nil); err == nil {
			t.Errorf("accepted null required value")
		}
	}
	for _, tc := range []struct {
		sc sql.Scanner
		v  any
	}{{Time(new(int)), time.Now()}, {Date(new(int)), time.Now()}, {ID(new(int)), "id"}, {Bool(&s), true}, {Decimal(new(int)), "1"}} {
		if err := tc.sc.Scan(tc.v); err == nil {
			t.Errorf("accepted unsupported destination")
		}
	}
}

func TestDateScannerStoredRepresentations(t *testing.T) {
	want := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	for _, input := range []any{"2026-10-11", []byte("2026-10-11"), "2026-10-11T12:34:56.000000000Z", []byte("2026-10-11T12:34:56Z"), sql.NullString{String: "2026-10-11T12:34:56Z", Valid: true}, "2026-10-11T23:30:00-03:00", time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)} {
		var got time.Time
		if err := Date(&got).Scan(input); err != nil {
			t.Fatalf("%T %v: %v", input, input, err)
		}
		expected := want
		if raw, ok := input.(string); ok && raw == "2026-10-11T23:30:00-03:00" {
			expected = want.AddDate(0, 0, 1)
		}
		if !got.Equal(expected) {
			t.Fatalf("%v date = %v, want %v", input, got, expected)
		}
		var text string
		if err := Date(&text).Scan(input); err != nil || text != expected.Format(DateLayout) {
			t.Fatalf("%v text = %q, error %v", input, text, err)
		}
	}
	for _, input := range []any{"", "2026-02-30", []byte("invalid"), 1} {
		if err := Date(new(time.Time)).Scan(input); err == nil {
			t.Fatalf("accepted invalid date %v", input)
		}
	}
}
