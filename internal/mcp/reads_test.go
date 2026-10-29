package mcp

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestReadResultSizeLimit(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"description":"` + strings.Repeat("x", 8<<20) + `"}`))
	})
	result, _, err := runRead(context.Background(), handler, "GET", nil, nil, "", nil, false)
	if err == nil || result != nil || !strings.Contains(err.Error(), "narrow") {
		t.Fatalf("oversized read must fail explicitly, not truncate: result=%+v error=%v", result, err)
	}
}
