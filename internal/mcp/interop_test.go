package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/db/dbtest"
)

func TestPythonClientInterop(t *testing.T) {
	python := os.Getenv("JULIUS_MCP_TEST_PYTHON")
	if python == "" {
		t.Skip("JULIUS_MCP_TEST_PYTHON not set; see docs/mcp.md")
	}
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		store, _ := setup(t, conn)
		srv := httptest.NewUnstartedServer(nil)
		defer srv.Close()
		base := "http://" + srv.Listener.Addr().String()
		handler, err := NewHandler(true, auth.Config{PublicURL: base}, store)
		if err != nil {
			t.Fatal(err)
		}
		srv.Config.Handler = handler
		srv.Start()
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, python, "testdata/client.py")
		cmd.Env = append(os.Environ(), "JULIUS_MCP_TEST_URL="+base, "PYTHONDONTWRITEBYTECODE=1")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Python MCP client: %v\n%s", err, output)
		}
		t.Log(string(output))
	})
}

func TestInspectorClientInterop(t *testing.T) {
	inspector := os.Getenv("JULIUS_MCP_TEST_INSPECTOR")
	if inspector == "" {
		t.Skip("JULIUS_MCP_TEST_INSPECTOR not set; see docs/mcp.md")
	}
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		store, token := setup(t, conn)
		handler, err := NewHandler(true, testConfig, store)
		if err != nil {
			t.Fatal(err)
		}
		srv := httptest.NewServer(handler)
		defer srv.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, inspector, "--cli", srv.URL+"/mcp", "--transport", "http", "--method", "tools/list", "--header", "Authorization: Bearer "+token)
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("Inspector discovery: %v", err)
		}
		var result struct {
			Tools []json.RawMessage `json:"tools"`
		}
		if err := json.Unmarshal(output, &result); err != nil || len(result.Tools) != 51 {
			t.Fatalf("Inspector discovery: %s (%v)", output, err)
		}
	})
}
