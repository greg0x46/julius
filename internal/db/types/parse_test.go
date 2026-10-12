package types

import (
	"database/sql"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"testing"
	"time"
)

func TestParseTime(t *testing.T) {
	want := time.Date(2026, 10, 11, 12, 13, 14, 123456789, time.UTC)
	for _, v := range []any{want, want.In(time.FixedZone("offset", -10800)), sql.NullTime{Time: want, Valid: true}, want.Format(TimeLayout), []byte(want.Format(TimeLayout)), sql.NullString{String: want.Format(TimeLayout), Valid: true}, "2026-10-11T09:13:14.123456789-03:00", "2026-10-11 12:13:14.123456789Z", "2026-10-11 09:13:14.123456789-03:00", "2026-10-11 09:13:14.123456789-03", "2026-10-11 12:13:14.123456789 +0000 UTC", "2026-10-11 12:13:14.123456789", "2026-10-11T12:13:14.123456789"} {
		got, err := ParseTime(v)
		if err != nil || !got.Equal(want) || got.Location() != time.UTC {
			t.Errorf("ParseTime(%#v) = %v, %v", v, got, err)
		}
	}
	for _, v := range []any{nil, sql.NullTime{}, sql.NullString{}, true, 1, "", "2026-02-30T12:00:00Z", "2026-10-11", "garbage"} {
		if _, err := ParseTime(v); err == nil {
			t.Errorf("accepted %#v", v)
		}
	}
}
func TestParseDate(t *testing.T) {
	want := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	native := time.Date(2026, 10, 11, 23, 30, 0, 0, time.FixedZone("offset", -10800))
	for _, v := range []any{native, sql.NullTime{Time: native, Valid: true}, "2026-10-11", []byte("2026-10-11"), sql.NullString{String: "2026-10-11", Valid: true}} {
		got, err := ParseDate(v)
		if err != nil || got != want {
			t.Errorf("ParseDate(%#v) = %v, %v", v, got, err)
		}
	}
	for _, v := range []any{nil, sql.NullTime{}, sql.NullString{}, 1, "", "2026-02-30", "2026-10-11T00:00:00Z"} {
		if _, err := ParseDate(v); err == nil {
			t.Errorf("accepted %#v", v)
		}
	}
}
func TestParseUUID(t *testing.T) {
	want := uuid.MustParse("b438a2c8-6c7d-4e94-8e2f-5c230900671a")
	for _, v := range []any{want, [16]byte(want), want[:], want.String(), []byte(want.String()), sql.NullString{String: want.String(), Valid: true}} {
		got, err := ParseUUID(v)
		if err != nil || got != want {
			t.Errorf("ParseUUID(%#v) = %v, %v", v, got, err)
		}
	}
	for _, v := range []any{nil, sql.NullString{}, 1, "", "bad", []byte{1}} {
		if _, err := ParseUUID(v); err == nil {
			t.Errorf("accepted %#v", v)
		}
	}
}
func TestParseID(t *testing.T) {
	for _, s := range []string{"integrated:source", "provider-1", "opaque-sixteen!!x", ""} {
		for _, v := range []any{s, []byte(s), sql.NullString{String: s, Valid: true}} {
			got, err := ParseID(v)
			if err != nil || got != s {
				t.Errorf("ParseID(%#v) = %q, %v", v, got, err)
			}
		}
	}
	id := uuid.New()
	for _, v := range []any{id, [16]byte(id)} {
		got, err := ParseID(v)
		if err != nil || got != id.String() {
			t.Errorf("ParseID(%#v) = %q, %v", v, got, err)
		}
	}
	for _, v := range []any{nil, sql.NullString{}, true, 1} {
		if _, err := ParseID(v); err == nil {
			t.Errorf("accepted %#v", v)
		}
	}
}
func TestParseBool(t *testing.T) {
	for _, tc := range []struct {
		v    any
		want bool
	}{{true, true}, {false, false}, {int64(1), true}, {int64(0), false}, {int64(-1), true}, {1, true}, {0, false}, {sql.NullBool{Bool: true, Valid: true}, true}, {sql.NullInt64{Int64: 1, Valid: true}, true}, {"true", true}, {"false", false}, {"t", true}, {"f", false}, {"1", true}, {"0", false}, {[]byte("TRUE"), true}, {[]byte("FALSE"), false}} {
		got, err := ParseBool(tc.v)
		if err != nil || got != tc.want {
			t.Errorf("ParseBool(%#v) = %v, %v", tc.v, got, err)
		}
	}
	for _, v := range []any{nil, sql.NullBool{}, sql.NullInt64{}, 1.0, "", "yes", "2", []byte("bad")} {
		if _, err := ParseBool(v); err == nil {
			t.Errorf("accepted %#v", v)
		}
	}
}
func TestParseDecimal(t *testing.T) {
	for _, tc := range []struct {
		v    any
		want string
	}{{"12345678901234567890.123456789", "12345678901234567890.123456789"}, {[]byte("-12.34"), "-12.34"}, {int64(42), "42"}, {42, "42"}, {decimal.RequireFromString("12.34"), "12.34"}, {sql.NullString{String: "0.01", Valid: true}, "0.01"}} {
		got, err := ParseDecimal(tc.v)
		if err != nil || got.String() != tc.want {
			t.Errorf("ParseDecimal(%#v) = %v, %v", tc.v, got, err)
		}
	}
	for _, v := range []any{nil, sql.NullString{}, true, 1.1, "", "bad"} {
		if _, err := ParseDecimal(v); err == nil {
			t.Errorf("accepted %#v", v)
		}
	}
}
func TestNullableParsers(t *testing.T) {
	for _, v := range []any{nil, sql.NullString{}, sql.NullTime{}, sql.NullBool{}, sql.NullInt64{}} {
		if got, err := ParseNullTime(v); got != nil || err != nil {
			t.Errorf("ParseNullTime(%#v) = %v, %v", v, got, err)
		}
		if got, err := ParseNullDecimal(v); got != nil || err != nil {
			t.Errorf("ParseNullDecimal(%#v) = %v, %v", v, got, err)
		}
	}
	if got, err := ParseNullTime("2026-10-11T00:00:00Z"); err != nil || got == nil {
		t.Fatal(got, err)
	}
	if got, err := ParseNullDecimal("0"); err != nil || got == nil {
		t.Fatal(got, err)
	}
	if _, err := ParseNullTime("bad"); err == nil {
		t.Fatal("accepted invalid timestamp")
	}
	if _, err := ParseNullDecimal("bad"); err == nil {
		t.Fatal("accepted invalid decimal")
	}
}
