package settings

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/text/currency"
)

const KeyReferenceCurrency = "finance.reference_currency"

func ValidateReferenceCurrency(code string) error {
	_, err := currency.ParseISO(code)
	if err != nil || code != strings.ToUpper(code) || code == "XXX" {
		return fmt.Errorf("Escolha uma moeda de referência válida.")
	}
	return nil
}

func ReferenceCurrency(ctx context.Context, q Querier) (string, error) {
	value, ok, err := Get(ctx, q, KeyReferenceCurrency, nil)
	if err != nil {
		return "", err
	}
	if !ok {
		return "BRL", nil
	}
	return value, ValidateReferenceCurrency(value)
}

const KeyTimezone = "finance.timezone"

// DefaultTimezone matches the quote refresh default.
const DefaultTimezone = "America/Sao_Paulo"

var errInvalidTimezone = errors.New("Escolha um fuso horário válido.")

// LoadTimezone resolves an IANA zone name. "" and "Local" are rejected:
// time.LoadLocation maps them to UTC and the process zone, so they would make
// calendar days depend on the host.
func LoadTimezone(name string) (*time.Location, error) {
	if name == "" || name == "Local" {
		return nil, errInvalidTimezone
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, errInvalidTimezone
	}
	return loc, nil
}

func ValidateTimezone(name string) error {
	_, err := LoadTimezone(name)
	return err
}

// Timezone is the zone every calendar day ("today", card cycles, month
// buckets) is read in.
func Timezone(ctx context.Context, q Querier) (string, error) {
	value, ok, err := Get(ctx, q, KeyTimezone, nil)
	if err != nil {
		return "", err
	}
	if !ok {
		return DefaultTimezone, nil
	}
	return value, ValidateTimezone(value)
}

// Location is Timezone resolved to a *time.Location.
func Location(ctx context.Context, q Querier) (*time.Location, error) {
	name, err := Timezone(ctx, q)
	if err != nil {
		return nil, err
	}
	return LoadTimezone(name)
}
