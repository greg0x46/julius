package types

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

type scanner func(any) error

func (s scanner) Scan(value any) error { return s(value) }

// Time and Date normalize driver values before assigning existing store destinations.
func Time(dest any) sql.Scanner { return temporal(dest, ParseTime, TimeLayout) }
func Date(dest any) sql.Scanner { return temporal(dest, ParseDate, DateLayout) }

func temporal(dest any, parse func(any) (time.Time, error), layout string) sql.Scanner {
	return scanner(func(value any) error {
		if isNull(value) {
			return assignNull(dest)
		}
		t, err := parse(value)
		if err != nil {
			return err
		}
		switch d := dest.(type) {
		case *time.Time:
			*d = t
		case **time.Time:
			*d = &t
		case *sql.NullTime:
			*d = sql.NullTime{Time: t, Valid: true}
		default:
			return assignText(dest, t.Format(layout))
		}
		return nil
	})
}

func ID(dest any) sql.Scanner {
	return scanner(func(value any) error {
		if isNull(value) {
			return assignNull(dest)
		}
		s, err := ParseID(value)
		if err != nil {
			return err
		}
		return assignText(dest, s)
	})
}

func Bool(dest any) sql.Scanner {
	return scanner(func(value any) error {
		if isNull(value) {
			return assignNull(dest)
		}
		b, err := ParseBool(value)
		if err != nil {
			return err
		}
		switch d := dest.(type) {
		case *bool:
			*d = b
		case **bool:
			*d = &b
		case *sql.NullBool:
			*d = sql.NullBool{Bool: b, Valid: true}
		case *int:
			*d = 0
			if b {
				*d = 1
			}
		default:
			return fmt.Errorf("unsupported boolean destination %T", dest)
		}
		return nil
	})
}

func Decimal(dest any) sql.Scanner {
	return scanner(func(value any) error {
		if isNull(value) {
			return assignNull(dest)
		}
		d, err := ParseDecimal(value)
		if err != nil {
			return err
		}
		switch target := dest.(type) {
		case *decimal.Decimal:
			*target = d
		case **decimal.Decimal:
			*target = &d
		default:
			s := d.String()
			if raw, err := text(value); err == nil {
				s = raw
			}
			return assignText(dest, s)
		}
		return nil
	})
}

func assignText(dest any, value string) error {
	switch d := dest.(type) {
	case *string:
		*d = value
	case **string:
		*d = &value
	case *sql.NullString:
		*d = sql.NullString{String: value, Valid: true}
	default:
		return fmt.Errorf("unsupported text destination %T", dest)
	}
	return nil
}

func assignNull(dest any) error {
	switch d := dest.(type) {
	case **string:
		*d = nil
	case **time.Time:
		*d = nil
	case **bool:
		*d = nil
	case **decimal.Decimal:
		*d = nil
	case *sql.NullString:
		*d = sql.NullString{}
	case *sql.NullTime:
		*d = sql.NullTime{}
	case *sql.NullBool:
		*d = sql.NullBool{}
	default:
		return fmt.Errorf("null database value for %T", dest)
	}
	return nil
}
