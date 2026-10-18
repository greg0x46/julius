package settings

import (
	"context"
	"fmt"
	"strings"

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
