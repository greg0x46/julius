package mcp

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/greg0x46/julius/internal/httpapi"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxReadPageSize = 200

// Collection pages retain the API's order and never paginate financial totals.
type readPage struct {
	Page     int `json:"page,omitempty" jsonschema:"Page number, starting at 1 (default 1, maximum 1000000)."`
	PageSize int `json:"page_size,omitempty" jsonschema:"Items per page (default 50, maximum 200)."`
}

func (p *readPage) validate() error {
	if p.Page == 0 {
		p.Page = 1
	}
	if p.PageSize == 0 {
		p.PageSize = 50
	}
	if p.Page < 1 || p.Page > 1000000 || p.PageSize < 1 || p.PageSize > maxReadPageSize {
		return errors.New("page must be 1..1000000 and page_size 1..200")
	}
	return nil
}

type billsInput struct {
	readPage
	AccountID string `json:"account_id" jsonschema:"Account identifier from list_accounts."`
}

type spendingInput struct {
	readPage
	Timezone       string `json:"timezone" jsonschema:"IANA timezone, for example America/Sao_Paulo."`
	Month          string `json:"month,omitempty" jsonschema:"YYYY-MM; choose month, a date range, or period=all."`
	DateFrom       string `json:"date_from,omitempty" jsonschema:"Inclusive calendar day YYYY-MM-DD; requires date_to."`
	DateTo         string `json:"date_to,omitempty" jsonschema:"Inclusive calendar day YYYY-MM-DD; requires date_from."`
	Period         string `json:"period,omitempty" jsonschema:"Use all for all dates; do not combine with month or date range."`
	Classification string `json:"classification" jsonschema:"outflow for spending or inflow for income."`
}

type readResult struct {
	Status     int             `json:"status"`
	Data       any             `json:"data"`
	Pagination *readPagination `json:"pagination,omitempty"`
}

type readPagination struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	TotalItems int `json:"total_items"`
	TotalPages int `json:"total_pages"`
}

func registerReadTools(server *sdk.Server, conn *sql.DB) {
	handlers := httpapi.FinancialReadHandlers(conn)
	tool := func(name, description string) *sdk.Tool {
		return &sdk.Tool{Name: name, Description: description + " Results contain HTTP status and the API payload in data. Pages contain at most 200 items. Money uses exact decimal strings; null monetary values mean unavailable, never zero. No owner data is changed.", Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true}}
	}
	for _, name := range []string{"list_accounts", "list_categories"} {
		description := "List accounts and cards with native currency balances, credit limits and closing-day metadata."
		if name == "list_categories" {
			description = "List the category catalog, including inactive categories, kinds and display metadata."
		}
		sdk.AddTool(server, tool(name, description), func(ctx context.Context, _ *sdk.CallToolRequest, input readPage) (*sdk.CallToolResult, readResult, error) {
			if err := input.validate(); err != nil {
				return nil, readResult{}, err
			}
			return runRead(ctx, handlers[name], "GET", nil, nil, "", &input, false)
		})
	}
	sdk.AddTool(server, tool("list_card_bills", "List recorded card bills for an account. The open bill is represented by the account balance, not a fabricated bill. Due and closing dates retain the REST timestamp representation of calendar days."), func(ctx context.Context, _ *sdk.CallToolRequest, input billsInput) (*sdk.CallToolResult, readResult, error) {
		if err := input.validate(); err != nil {
			return nil, readResult{}, err
		}
		if strings.TrimSpace(input.AccountID) == "" {
			return nil, readResult{}, errors.New("account_id is required")
		}
		return runRead(ctx, handlers["list_card_bills"], "GET", nil, nil, input.AccountID, &input.readPage, false)
	})
	sdk.AddTool(server, tool("search_transactions", "Search transactions using the REST query filters. timezone is required; group_by is none, day, week, month or year. Date ranges are inclusive YYYY-MM-DD pairs. Defaults: page=1, page_size=50, group_by=none. Includes ignored, pending and unclassified rows with explicit eligibility; totals cover all matching rows, not just the page. Native currency totals and consolidated reporting coverage/FX are retained."), func(ctx context.Context, _ *sdk.CallToolRequest, input httpapi.TransactionReadQuery) (*sdk.CallToolResult, readResult, error) {
		page := readPage{Page: input.Page, PageSize: input.PageSize}
		if err := page.validate(); err != nil {
			return nil, readResult{}, err
		}
		input.Page, input.PageSize = page.Page, page.PageSize
		if input.GroupBy == "" {
			input.GroupBy = "none"
		}
		switch input.GroupBy {
		case "none", "day", "week", "month", "year":
		default:
			return nil, readResult{}, errors.New("invalid group_by")
		}
		body, err := json.Marshal(input)
		if err != nil {
			return nil, readResult{}, err
		}
		return runRead(ctx, handlers["search_transactions"], "POST", body, nil, "", nil, false)
	})
	sdk.AddTool(server, tool("spending_by_category", "Review real spending or income by category for a month, inclusive date range or all dates. No hypothetical scenarios are included. Paginated items share totals for the whole period. Per-currency totals, reporting status, missing FX and null consolidated totals are retained; reference-currency valuation includes explicit FX provenance."), func(ctx context.Context, _ *sdk.CallToolRequest, input spendingInput) (*sdk.CallToolResult, readResult, error) {
		if err := input.validate(); err != nil {
			return nil, readResult{}, err
		}
		params := url.Values{"timezone": {input.Timezone}, "classification": {input.Classification}}
		for key, value := range map[string]string{"month": input.Month, "date_from": input.DateFrom, "date_to": input.DateTo, "period": input.Period} {
			if value != "" {
				params.Set(key, value)
			}
		}
		return runRead(ctx, handlers["spending_by_category"], "GET", nil, params, "", &input.readPage, true)
	})
}

// The adapter calls only allowlisted handlers, never the router or a URL supplied
// by the client. Raw JSON preserves decimal strings and explicit nulls.
func runRead(ctx context.Context, handler http.HandlerFunc, method string, body []byte, params url.Values, accountID string, page *readPage, report bool) (*sdk.CallToolResult, readResult, error) {
	req, err := http.NewRequestWithContext(ctx, method, "/?"+params.Encode(), bytes.NewReader(body))
	if err != nil {
		return nil, readResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", accountID)
	response := &readResponse{header: make(http.Header)}
	handler.ServeHTTP(response, req)
	if response.overflow {
		return nil, readResult{}, errors.New("result exceeds 8 MiB; narrow the date range or filters")
	}
	result := readResult{Status: response.status, Data: json.RawMessage(response.body.Bytes())}
	if result.Status >= 400 {
		return &sdk.CallToolResult{IsError: true}, result, nil
	}
	if page != nil {
		data := result.Data.(json.RawMessage)
		var fields map[string]json.RawMessage
		if report {
			if err := json.Unmarshal(data, &fields); err != nil {
				return nil, readResult{}, errors.New("invalid report response")
			}
			data = fields["items"]
		}
		var items []json.RawMessage
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, readResult{}, errors.New("invalid collection response")
		}
		total := len(items)
		start := min((page.Page-1)*page.PageSize, total)
		end := min(start+page.PageSize, total)
		data, _ = json.Marshal(items[start:end])
		if report {
			fields["items"] = data
			data, _ = json.Marshal(fields)
		}
		result.Data = data
		result.Pagination = &readPagination{Page: page.Page, PageSize: page.PageSize, TotalItems: total, TotalPages: (total + page.PageSize - 1) / page.PageSize}
	}
	return nil, result, nil
}

type readResponse struct {
	header   http.Header
	status   int
	body     bytes.Buffer
	overflow bool
}

func (r *readResponse) Header() http.Header { return r.header }
func (r *readResponse) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
}
func (r *readResponse) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	if r.body.Len()+len(data) > 8<<20 {
		r.overflow = true
		return 0, errors.New("read result too large")
	}
	return r.body.Write(data)
}
