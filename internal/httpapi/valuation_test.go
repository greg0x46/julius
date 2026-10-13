package httpapi

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/exchange"
)

type unavailableFX struct{}

func (unavailableFX) Name() string { return "synthetic" }
func (unavailableFX) Rate(context.Context, string, string, time.Time) (exchange.ProviderRate, error) {
	return exchange.ProviderRate{}, errors.New("offline")
}

func TestReferenceValuationRequiresFX(t *testing.T) {
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	fx := exchange.New(conn, unavailableFX{})
	_, err = valuationRate(context.Background(), fx, "USD", time.Now().UTC())
	if !errors.Is(err, exchange.ErrUnavailable) {
		t.Fatalf("missing FX = %v", err)
	}
	rate, err := valuationRate(context.Background(), fx, "BRL", time.Now().UTC())
	if err != nil || rate.Source != exchange.SourceIdentity {
		t.Fatalf("BRL regression = %+v, %v", rate, err)
	}
}
