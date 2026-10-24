package main

import (
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDemo(t *testing.T) {
	for value, want := range map[string]bool{"": false, "  ": false, "0": false, "false": false, "1": true, "true": true, " TRUE ": true} {
		if got, err := parseDemo(value); err != nil || got != want {
			t.Errorf("parseDemo(%q) = %v, %v; want %v", value, got, err, want)
		}
	}
	// A typo must not silently leave a public instance closed or open.
	if _, err := parseDemo("yes please"); err == nil {
		t.Fatal("parseDemo accepted a value that is not a boolean")
	}
}

func TestDemoSeedCommandFillsAFreshDatabaseOnce(t *testing.T) {
	t.Setenv("JULIUS_PUBLIC_URL", "https://demo.example")
	t.Setenv("JULIUS_MASTER_KEY", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	t.Setenv("JULIUS_MASTER_KEY_FILE", "")
	path := filepath.Join(t.TempDir(), "demo.db")

	if err := demoCommand([]string{"seed", "-db", path}); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	if err := demoCommand([]string{"seed", "-db", path}); err == nil || !strings.Contains(err.Error(), "already has an owner") {
		t.Fatalf("second seed error = %v, want a refusal to touch a database that has an owner", err)
	}
}

func TestDemoCommandRejectsBadUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"reset"}, {"seed", "extra"}} {
		if err := demoCommand(args); err == nil {
			t.Errorf("demoCommand(%v) succeeded, want a usage error", args)
		}
	}
}
