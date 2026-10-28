package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/mcp"
)

func TestMCPRouteIsolation(t *testing.T) {
	disabled, err := mcp.NewHandler(false, auth.Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Even an open/demo API fallback cannot claim the reserved MCP paths.
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	handler := serverHandler(api, disabled)
	for _, path := range []string{"/mcp", "/mcp/", "/mcp/anything", "/oauth/authorize", "/oauth/token", "/.well-known/oauth-authorization-server", "/.well-known/oauth-protected-resource/mcp", "/api/categories", "/"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		want := 204
		if path != "/" && path != "/api/categories" {
			want = 404
		}
		if w.Code != want {
			t.Fatalf("%s: %d, want %d", path, w.Code, want)
		}
	}
}

func TestServeStartupAndShutdown(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	release := make(chan struct{})
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		w.WriteHeader(204)
	}), ReadHeaderTimeout: time.Second}
	done := make(chan error, 1)
	go func() { done <- serve(ctx, server, listener) }()
	response := make(chan error, 1)
	go func() {
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get("http://" + listener.Addr().String())
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
		response <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("server did not start")
	}
	cancel()
	select {
	case err := <-done:
		close(release)
		t.Fatalf("shutdown did not drain request: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-response; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not finish")
	}
	if _, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second); err == nil {
		t.Fatal("listener still open")
	}
}

func TestServeStartupFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listener.Close()
	done := make(chan error, 1)
	go func() { done <- serve(context.Background(), &http.Server{}, listener) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("startup failure was ignored")
		}
	case <-time.After(time.Second):
		t.Fatal("startup failure did not stop lifecycle goroutine")
	}
}
