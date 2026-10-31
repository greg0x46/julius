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

func TestTimezone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.db")
	conn, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	name, err := settings.Timezone(ctx, conn)
	if err != nil || name != "America/Sao_Paulo" {
		t.Fatalf("default = %q, %v", name, err)
	}
	loc, err := settings.Location(ctx, conn)
	if err != nil || loc.String() != "America/Sao_Paulo" {
		t.Fatalf("default location = %v, %v", loc, err)
	}
	for _, valid := range []string{"America/Sao_Paulo", "UTC", "Asia/Tokyo", "Pacific/Kiritimati"} {
		if err := settings.ValidateTimezone(valid); err != nil {
			t.Errorf("%s: %v", valid, err)
		}
	}
	// "" and "Local" would silently mean UTC and the process zone.
	for _, invalid := range []string{"", "Local", " UTC", "Mars/Olympus", "../etc/passwd", "America/"} {
		if err := settings.ValidateTimezone(invalid); err == nil {
			t.Errorf("accepted %q", invalid)
		}
	}
	if err := settings.Set(ctx, conn, settings.KeyTimezone, "Asia/Tokyo", false, nil); err != nil {
		t.Fatal(err)
	}
	conn.Close()
	conn, err = db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if loc, err = settings.Location(ctx, conn); err != nil || loc.String() != "Asia/Tokyo" {
		t.Fatalf("after reopen = %v, %v", loc, err)
	}
	if err := settings.Set(ctx, conn, settings.KeyTimezone, "Local", false, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := settings.Timezone(ctx, conn); err == nil {
		t.Fatal("a stored invalid zone must not be served")
	}
}
