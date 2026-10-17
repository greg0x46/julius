package settings_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/settings"
)

func TestReferenceCurrency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.db")
	conn, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	code, err := settings.ReferenceCurrency(ctx, conn)
	if err != nil || code != "BRL" {
		t.Fatalf("default = %q, %v", code, err)
	}
	for _, valid := range []string{"BRL", "USD", "EUR", "JPY"} {
		if err := settings.ValidateReferenceCurrency(valid); err != nil {
			t.Errorf("%s: %v", valid, err)
		}
	}
	for _, invalid := range []string{"", "usd", " USD", "ZZZ", "XXX", "US", "USDD"} {
		if err := settings.ValidateReferenceCurrency(invalid); err == nil {
			t.Errorf("accepted %q", invalid)
		}
	}
	if err := settings.Set(ctx, conn, settings.KeyReferenceCurrency, "USD", false, nil); err != nil {
		t.Fatal(err)
	}
	conn.Close()
	conn, err = db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	code, err = settings.ReferenceCurrency(ctx, conn)
	if err != nil || code != "USD" {
		t.Fatalf("after reopen = %q, %v", code, err)
	}
}
