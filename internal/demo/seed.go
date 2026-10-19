// Package demo fills an empty database with synthetic data for the public demo
// instance. It drives the real HTTP handler in process, so every row passes
// the same validation and multi-engine transactions as a user's own write.
package demo

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/httpapi"
	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/settings"
)

// ErrNotEmpty is returned when the database already has an owner: seeding only
// ever fills a fresh database and never removes anything.
var ErrNotEmpty = errors.New("demo: the database already has an owner; seed only fills a fresh database")

// Options configures Seed. Now anchors every date, so the data always looks
// current; Master is the instance's own key, never shared with another one.
type Options struct {
	Master    []byte
	PublicURL string
	Now       time.Time
}

// Summary counts what Seed created.
type Summary struct {
	Accounts, Transactions, Transfers, Recurring, Payables, Goals, Rules, Positions, Operations int
}

// Seed initializes the owner with an unrecoverable random password (the demo
// is open: authentication is switched off and stays off), disables the quote
// refresh and then creates the sample data.
func Seed(ctx context.Context, conn *sql.DB, opts Options) (Summary, error) {
	var owners int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM auth_owner`).Scan(&owners); err != nil {
		return Summary{}, fmt.Errorf("demo: check owner: %w", err)
	}
	if owners != 0 {
		return Summary{}, ErrNotEmpty
	}
	password := make([]byte, 32)
	if _, err := rand.Read(password); err != nil {
		return Summary{}, err
	}
	if err := auth.NewStore(conn).Initialize(ctx, "demo@example.test", hex.EncodeToString(password), opts.Master, nil); err != nil {
		return Summary{}, fmt.Errorf("demo: initialize owner: %w", err)
	}
	if err := settings.SetAuthenticationEnabled(ctx, conn, false); err != nil {
		return Summary{}, fmt.Errorf("demo: disable authentication: %w", err)
	}
	quotes := settings.DefaultQuoteRefreshSettings()
	quotes.Enabled = false
	if _, err := settings.InitializeQuoteRefresh(ctx, conn, quotes); err != nil {
		return Summary{}, fmt.Errorf("demo: disable quote refresh: %w", err)
	}

	// The server fills the catalog on every start; positions refer to it.
	if _, err := investments.SeedInvestmentAssets(ctx, conn); err != nil {
		return Summary{}, fmt.Errorf("demo: seed asset catalog: %w", err)
	}

	handler := httpapi.NewServer(conn, noFiles{}, settings.NewSecrets(opts.Master), auth.Config{PublicURL: opts.PublicURL, Demo: true}, nil)
	s := &seeder{ctx: ctx, conn: conn, handler: handler, origin: opts.PublicURL, today: day(opts.Now), rng: &lcg{state: 42}}
	if err := s.run(); err != nil {
		return s.summary, err
	}
	return s.summary, nil
}

// noFiles is the empty frontend: the seeder only calls /api.
type noFiles struct{}

func (noFiles) Open(string) (fs.File, error) { return nil, fs.ErrNotExist }

type seeder struct {
	ctx     context.Context
	conn    *sql.DB
	handler http.Handler
	origin  string
	today   time.Time
	rng     *lcg
	summary Summary
	err     error
}

// call sends one API request and returns the decoded JSON object (or array).
// The first failure is kept and every later call is skipped, so the data
// builders read as straight-line code.
func (s *seeder) call(method, path string, body any) map[string]any {
	if s.err != nil {
		return nil
	}
	var reader *bytes.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			s.err = err
			return nil
		}
		reader = bytes.NewReader(payload)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader).WithContext(s.ctx)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", s.origin)
	req.Header.Set("X-Julius-Request", "1")
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	if rec.Code < 200 || rec.Code > 299 {
		s.err = fmt.Errorf("demo: %s %s: %d %s", method, path, rec.Code, bytes.TrimSpace(rec.Body.Bytes()))
		return nil
	}
	var out map[string]any
	if rec.Body.Len() > 0 && rec.Body.Bytes()[0] == '{' {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			s.err = fmt.Errorf("demo: %s %s: decode: %w", method, path, err)
			return nil
		}
	}
	return out
}

// list reads a JSON array response, or the {"items": [...]} wrapper.
func (s *seeder) list(path string) []map[string]any {
	if s.err != nil {
		return nil
	}
	req := httptest.NewRequest(http.MethodGet, path, nil).WithContext(s.ctx)
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		s.err = fmt.Errorf("demo: GET %s: %d %s", path, rec.Code, bytes.TrimSpace(rec.Body.Bytes()))
		return nil
	}
	var out []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err == nil {
		return out
	}
	var wrapped struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &wrapped); err != nil {
		s.err = fmt.Errorf("demo: GET %s: decode: %w", path, err)
	}
	return wrapped.Items
}

// id returns the "id" of a created resource, or "" after a failure.
func id(resource map[string]any) string {
	value, _ := resource["id"].(string)
	return value
}

// lcg is a tiny deterministic generator: the same day always yields the same
// data, and the data does not depend on the standard library's generator.
type lcg struct{ state uint64 }

func (g *lcg) next() uint64 {
	g.state = g.state*6364136223846793005 + 1442695040888963407
	return g.state >> 33
}

// between returns a value in [lo, hi] cents-exact, as a decimal string.
func (g *lcg) between(lo, hi int) string {
	cents := lo*100 + int(g.next()%uint64((hi-lo)*100+1))
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}

func day(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func date(t time.Time) string { return t.Format("2006-01-02") }
