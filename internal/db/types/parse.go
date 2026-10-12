// Package types decodes logical database values returned by SQLite and Postgres.
package types

import (
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/greg0x46/julius/internal/dates"
	"github.com/shopspring/decimal"
)

const TimeLayout = "2006-01-02T15:04:05.000000000Z"
const DateLayout = "2006-01-02"

func text(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	case sql.NullString:
		if v.Valid {
			return v.String, nil
		}
	}
	return "", fmt.Errorf("expected database text, got %T", value)
}

// ParseTime treats timestamps without an offset as UTC.
func ParseTime(value any) (time.Time, error) {
	switch v := value.(type) {
	case time.Time:
		return v.UTC(), nil
	case sql.NullTime:
		if v.Valid {
			return v.Time.UTC(), nil
		}
		return time.Time{}, fmt.Errorf("null timestamp")
	}
	s, err := text(value)
	if err != nil {
		return time.Time{}, err
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999Z07:00", "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05.999999999-07", "2006-01-02 15:04:05.999999999 -0700 MST", "2006-01-02 15:04:05.999999999", "2006-01-02T15:04:05.999999999"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid database timestamp %q", s)
}

// ParseDate preserves the calendar day of native date values.
func ParseDate(value any) (time.Time, error) {
	switch v := value.(type) {
	case time.Time:
		return dates.Day(v), nil
	case sql.NullTime:
		if v.Valid {
			return dates.Day(v.Time), nil
		}
		return time.Time{}, fmt.Errorf("null date")
	}
	s, err := text(value)
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(DateLayout, s)
}

func ParseUUID(value any) (uuid.UUID, error) {
	switch v := value.(type) {
	case uuid.UUID:
		return v, nil
	case [16]byte:
		return uuid.UUID(v), nil
	case []byte:
		if len(v) == 16 {
			return uuid.FromBytes(v)
		}
	}
	s, err := text(value)
	if err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(s)
}

// ParseID also accepts opaque provider and legacy identifiers.
func ParseID(value any) (string, error) {
	switch v := value.(type) {
	case uuid.UUID, [16]byte:
		id, err := ParseUUID(v)
		return id.String(), err
	}
	return text(value)
}

func ParseBool(value any) (bool, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case int64:
		return v != 0, nil
	case int:
		return v != 0, nil
	case sql.NullBool:
		if v.Valid {
			return v.Bool, nil
		}
	case sql.NullInt64:
		if v.Valid {
			return v.Int64 != 0, nil
		}
	case string:
		return strconv.ParseBool(v)
	case []byte:
		return strconv.ParseBool(string(v))
	}
	return false, fmt.Errorf("invalid database boolean %T", value)
}

func ParseDecimal(value any) (decimal.Decimal, error) {
	switch v := value.(type) {
	case decimal.Decimal:
		return v, nil
	case int64:
		return decimal.NewFromInt(v), nil
	case int:
		return decimal.NewFromInt(int64(v)), nil
	}
	s, err := text(value)
	if err != nil {
		return decimal.Zero, err
	}
	return decimal.NewFromString(s)
}

func ParseNullTime(value any) (*time.Time, error) {
	if isNull(value) {
		return nil, nil
	}
	t, err := ParseTime(value)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func ParseNullDecimal(value any) (*decimal.Decimal, error) {
	if isNull(value) {
		return nil, nil
	}
	d, err := ParseDecimal(value)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func isNull(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case sql.NullString:
		return !v.Valid
	case sql.NullTime:
		return !v.Valid
	case sql.NullBool:
		return !v.Valid
	case sql.NullInt64:
		return !v.Valid
	}
	return false
}
