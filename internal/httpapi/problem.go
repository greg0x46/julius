// Package httpapi is the net/http layer: it decodes requests, calls the
// domain packages (money/categories/transactions), and encodes responses in
// the same JSON shape frontend/src/api/contracts.ts expects, so the existing
// React frontend needs no changes to talk to this server.
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/greg0x46/julius/internal/reporting"
)

// Problem mirrors web/schemas.py's ProblemResponse (RFC 7807-flavored error
// body), served with the application/problem+json content type the
// frontend's client.ts specifically checks for.
type Problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
}

func writeProblem(w http.ResponseWriter, status int, kind, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Problem{
		Type:   "/problems/" + kind,
		Title:  title,
		Status: status,
		Detail: detail,
	})
}

func writeReportingProblem(w http.ResponseWriter, err error) bool {
	var incomplete *reporting.IncompleteError
	if !errors.As(err, &incomplete) {
		return false
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusServiceUnavailable)
	_ = json.NewEncoder(w).Encode(struct {
		Problem
		Reporting reporting.Summary  `json:"reporting"`
		Totals    []reporting.Totals `json:"totals"`
	}{Problem{Type: "/problems/reporting-fx-unavailable", Title: "Total indisponível", Status: 503, Detail: incomplete.Error()}, incomplete.Coverage, incomplete.Totals})
	return true
}
