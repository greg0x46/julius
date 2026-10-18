package pluggy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Config is the subset of app config the adapter needs — the settings
// package (phase 4's credential storage) supplies ClientID/ClientSecret
// decrypted from the settings table instead of environment variables.
type Config struct {
	BaseURL            string
	ClientID           string
	ClientSecret       string
	ItemID             string
	ConnectTimeout     time.Duration
	ReadTimeout        time.Duration
	MaxRateLimitWait   time.Duration
	MaxAttempts        int
	MaxPagesPerAccount int
}

// DefaultConfig fills in the same defaults as Settings' field defaults in
// the Python reference.
func DefaultConfig() Config {
	return Config{
		BaseURL:            "https://api.pluggy.ai",
		ConnectTimeout:     5 * time.Second,
		ReadTimeout:        30 * time.Second,
		MaxRateLimitWait:   60 * time.Second,
		MaxAttempts:        3,
		MaxPagesPerAccount: 1000,
	}
}

// Adapter mirrors PluggyAdapter.
type Adapter struct {
	config Config
	writer RawEnvelopeWriter
	client *http.Client
	sleep  func(context.Context, time.Duration) error

	apiKey      string
	institution *string
}

// NewAdapter builds an Adapter. httpClient defaults to one with the
// configured timeouts when nil; tests pass one pointed at an httptest server.
func NewAdapter(config Config, writer RawEnvelopeWriter, httpClient *http.Client) *Adapter {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: config.ConnectTimeout + config.ReadTimeout}
	}
	return &Adapter{config: config, writer: writer, client: httpClient, sleep: sleepContext}
}

// maxResponseBytes bounds any provider response body held in memory.
const maxResponseBytes = 32 << 20

// sleepContext waits d or until ctx ends, whichever comes first.
func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// readBody reads at most maxResponseBytes; a larger body is an error.
func readBody(r io.Reader) ([]byte, bool) {
	content, _ := io.ReadAll(io.LimitReader(r, maxResponseBytes+1))
	return content, len(content) <= maxResponseBytes
}

func (a *Adapter) authenticate(ctx context.Context) error {
	body, err := json.Marshal(map[string]string{
		"clientId":     a.config.ClientID,
		"clientSecret": a.config.ClientSecret,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.config.BaseURL+"/auth", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return &ProviderError{Code: "provider_unavailable", Stage: StageAuth, SafeMessage: "Financial provider is temporarily unavailable"}
	}
	defer resp.Body.Close()
	content, withinLimit := readBody(resp.Body)
	if !withinLimit {
		return &ProviderError{Code: "invalid_provider_payload", Stage: StageAuth, SafeMessage: "Financial provider authentication response was invalid"}
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return &ProviderError{Code: "invalid_provider_credentials", Stage: StageAuth, SafeMessage: "Financial provider credentials are invalid"}
	}
	if resp.StatusCode >= 400 {
		return &ProviderError{Code: "provider_unavailable", Stage: StageAuth, SafeMessage: "Financial provider authentication failed"}
	}

	var payload struct {
		APIKey string `json:"apiKey"`
	}
	if err := json.Unmarshal(content, &payload); err != nil || payload.APIKey == "" {
		return &ProviderError{Code: "invalid_provider_payload", Stage: StageAuth, SafeMessage: "Financial provider authentication response was invalid"}
	}
	a.apiKey = payload.APIKey
	return nil
}

func (a *Adapter) ensureAuthenticated(ctx context.Context) (string, error) {
	if a.apiKey == "" {
		if err := a.authenticate(ctx); err != nil {
			return "", err
		}
	}
	return a.apiKey, nil
}

func retryWait(resp *http.Response) time.Duration {
	if v := resp.Header.Get("retry-after"); v != "" {
		if seconds, err := strconv.ParseFloat(v, 64); err == nil && seconds >= 0 {
			return time.Duration(seconds * float64(time.Second))
		}
	}
	if v := resp.Header.Get("ratelimit-reset"); v != "" {
		if unix, err := strconv.ParseFloat(v, 64); err == nil {
			wait := time.Until(time.Unix(int64(unix), 0))
			if wait < 0 {
				wait = 0
			}
			return wait
		}
	}
	return 250 * time.Millisecond
}

// read mirrors PluggyAdapter._read: authenticate once, retry on network
// errors/429/5xx up to MaxAttempts, and persist every response that actually
// came back from the wire (network-level failures that never got a response
// are not — there is nothing to persist) before trusting its body.
func (a *Adapter) read(
	ctx context.Context, scope Scope, path string, stage FailureStage, pageSequence int, externalAccountID *string,
) (map[string]any, string, error) {
	apiKey, err := a.ensureAuthenticated(ctx)
	if err != nil {
		return nil, "", err
	}

	var lastNetErr error
	for attempt := 1; attempt <= a.config.MaxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.config.BaseURL+path, nil)
		if err != nil {
			return nil, "", err
		}
		req.Header.Set("X-API-KEY", apiKey)

		resp, err := a.client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, "", ctx.Err()
			}
			lastNetErr = err
			if attempt < a.config.MaxAttempts && isRetryableNetError(err) {
				if err := a.sleep(ctx, time.Duration(attempt)*250*time.Millisecond); err != nil {
					return nil, "", err
				}
				continue
			}
			return nil, "", &ProviderError{
				Code: "provider_unavailable", Stage: stage, SafeMessage: "Financial provider is temporarily unavailable",
				ExternalAccountID: externalAccountID,
			}
		}

		content, withinLimit := readBody(resp.Body)
		resp.Body.Close()
		if !withinLimit {
			return nil, "", &ProviderError{
				Code: "invalid_provider_payload", Stage: stage, SafeMessage: "The provider returned data that could not be processed safely",
				ExternalAccountID: externalAccountID,
			}
		}

		rawImportID, writeErr := a.writer.Write(ctx, RawResponseEnvelope{
			Scope: scope, Method: http.MethodGet, Path: req.URL.String(), StatusCode: resp.StatusCode,
			Headers: flattenHeader(resp.Header), Content: content, PageSequence: pageSequence,
			RequestAttempt: attempt, ExternalAccountID: externalAccountID, ReceivedAt: time.Now().UTC(),
		})
		if writeErr != nil {
			return nil, "", fmt.Errorf("persist raw import: %w", writeErr)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			wait := retryWait(resp)
			if attempt < a.config.MaxAttempts && wait <= a.config.MaxRateLimitWait {
				if err := a.sleep(ctx, wait); err != nil {
					return nil, "", err
				}
				continue
			}
			return nil, "", &ProviderError{
				Code: "provider_rate_limited", Stage: stage, SafeMessage: "Financial provider rate limit was exceeded",
				RawImportID: &rawImportID, ExternalAccountID: externalAccountID,
			}
		}
		if resp.StatusCode >= 500 {
			if attempt < a.config.MaxAttempts {
				if err := a.sleep(ctx, time.Duration(attempt)*250*time.Millisecond); err != nil {
					return nil, "", err
				}
				continue
			}
			return nil, "", &ProviderError{
				Code: "provider_unavailable", Stage: stage, SafeMessage: "Financial provider is temporarily unavailable",
				RawImportID: &rawImportID, ExternalAccountID: externalAccountID,
			}
		}
		if resp.StatusCode >= 400 {
			return nil, "", &ProviderError{
				Code: "provider_request_failed", Stage: stage, SafeMessage: "Financial provider rejected a read request",
				RawImportID: &rawImportID, ExternalAccountID: externalAccountID,
			}
		}

		payload, err := decodeJSON(content)
		if err != nil {
			var mapErr *MappingError
			if errors.As(err, &mapErr) {
				return nil, "", &ProviderError{
					Code: mapErr.Code, Stage: stage, SafeMessage: mapErr.SafeMessage,
					RawImportID: &rawImportID, ExternalAccountID: externalAccountID,
				}
			}
			return nil, "", err
		}
		return payload, rawImportID, nil
	}
	return nil, "", fmt.Errorf("retry loop exhausted: %w", lastNetErr)
}

func isRetryableNetError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr)
}

func flattenHeader(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		if len(v) > 0 {
			out[k] = v[0]
		}
	}
	return out
}

// GetSource mirrors PluggyAdapter.get_source, including
// _interpret_item_status's checks that turn a login-error or
// no-safely-updated-product item into a ProviderError instead of proceeding.
func (a *Adapter) GetSource(ctx context.Context) (SourceSnapshot, string, error) {
	payload, rawImportID, err := a.read(ctx, ScopeItem, "/items/"+url.PathEscape(a.config.ItemID), StageItem, 1, nil)
	if err != nil {
		return SourceSnapshot{}, "", err
	}
	source, err := mapSource(payload, a.config.ItemID)
	if err != nil {
		var mapErr *MappingError
		errors.As(err, &mapErr)
		return SourceSnapshot{}, "", &ProviderError{Code: mapErr.Code, Stage: StageItem, SafeMessage: mapErr.SafeMessage, RawImportID: &rawImportID}
	}
	source, err = interpretItemStatus(source, payload, rawImportID)
	if err != nil {
		return SourceSnapshot{}, "", err
	}
	a.institution = source.InstitutionName
	return source, rawImportID, nil
}

func interpretItemStatus(source SourceSnapshot, payload map[string]any, rawImportID string) (SourceSnapshot, error) {
	status := strField2(source.ProviderStatus)
	execution := strField2(source.ProviderExecutionStatus)
	if status == "LOGIN_ERROR" || execution == "INVALID_CREDENTIALS" {
		return SourceSnapshot{}, &ProviderError{
			Code: "item_invalid_credentials", Stage: StageItem,
			SafeMessage: "The financial connection requires updated credentials", RawImportID: &rawImportID,
		}
	}
	if status == "OUTDATED" && execution != "PARTIAL_SUCCESS" {
		return SourceSnapshot{}, &ProviderError{
			Code: "item_unavailable", Stage: StageItem,
			SafeMessage: "The financial connection has no safely updated products", RawImportID: &rawImportID,
		}
	}
	if execution != "PARTIAL_SUCCESS" {
		source.SafeProducts = map[string]bool{"ACCOUNTS": true, "TRANSACTIONS": true, "INVESTMENTS": true}
		return source, nil
	}
	safe := safeProducts(payload["statusDetail"])
	if len(safe) == 0 {
		return SourceSnapshot{}, &ProviderError{
			Code: "item_unavailable", Stage: StageItem,
			SafeMessage: "The financial connection has no safely updated products", RawImportID: &rawImportID,
		}
	}
	source.Warnings = append(source.Warnings, "provider_partial_result")
	source.SafeProducts = safe
	return source, nil
}

func strField2(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func safeProducts(statusDetail any) map[string]bool {
	safe := make(map[string]bool)
	switch detail := statusDetail.(type) {
	case map[string]any:
		for product, v := range detail {
			if isSuccessStatus(v) {
				safe[strings.ToUpper(product)] = true
			}
		}
	case []any:
		for _, item := range detail {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			product, _ := entry["product"].(string)
			if product != "" && isSuccessStatus(entry["status"]) {
				safe[strings.ToUpper(product)] = true
			}
		}
	}
	return safe
}

func isSuccessStatus(v any) bool {
	var status string
	switch value := v.(type) {
	case string:
		status = value
	case map[string]any:
		status, _ = value["status"].(string)
	}
	return status == "SUCCESS" || status == "UPDATED"
}

// GetAccounts mirrors PluggyAdapter.get_accounts.
func (a *Adapter) GetAccounts(ctx context.Context) (AccountsPage, error) {
	payload, rawImportID, err := a.read(ctx, ScopeAccounts, "/accounts?itemId="+url.QueryEscape(a.config.ItemID), StageAccounts, 1, nil)
	if err != nil {
		return AccountsPage{}, err
	}
	accounts, rejections, err := mapAccountsPage(payload, a.institution)
	if err != nil {
		var mapErr *MappingError
		errors.As(err, &mapErr)
		return AccountsPage{}, &ProviderError{Code: mapErr.Code, Stage: StageAccounts, SafeMessage: mapErr.SafeMessage, RawImportID: &rawImportID}
	}
	return AccountsPage{RawImportID: rawImportID, Accounts: accounts, Rejections: rejections}, nil
}

// GetInvestments mirrors GetAccounts for the Investments product.
func (a *Adapter) GetInvestments(ctx context.Context) (InvestmentsPage, error) {
	payload, rawImportID, err := a.read(ctx, ScopeInvestments, "/investments?itemId="+url.QueryEscape(a.config.ItemID), StageInvestments, 1, nil)
	if err != nil {
		return InvestmentsPage{}, err
	}
	investments, rejections, err := mapInvestmentsPage(payload)
	if err != nil {
		var mapErr *MappingError
		errors.As(err, &mapErr)
		return InvestmentsPage{}, &ProviderError{Code: mapErr.Code, Stage: StageInvestments, SafeMessage: mapErr.SafeMessage, RawImportID: &rawImportID}
	}
	return InvestmentsPage{RawImportID: rawImportID, Investments: investments, Rejections: rejections}, nil
}

// pageCapExceeded is what a page-numbered read answers with when it runs out
// of budget before the provider runs out of pages.
//
// Returning what was collected so far would be worse than failing: the caller
// has no way to tell a complete history from half of one, and half a history
// is exactly what makes a yield read as a loss (see applyYield's
// historico_incompleto). MaxPagesPerAccount exists to bound a runaway
// provider, not to quietly define how much history is enough, so hitting it
// is a provider problem to surface — with the account/investment named, so
// raising the cap is an informed decision rather than a guess.
func pageCapExceeded(stage FailureStage, externalID, rawImportID string, maxPages int) *ProviderError {
	return &ProviderError{
		Code:  "provider_pagination_exhausted",
		Stage: stage,
		SafeMessage: fmt.Sprintf(
			"Provider reported more than %d pages of history; refusing to report a truncated one", maxPages),
		RawImportID:       &rawImportID,
		ExternalAccountID: &externalID,
	}
}

// GetInvestmentTransactions fetches one investment's transaction history.
// The endpoint pages by number in the response body (page/totalPages), the
// same envelope as /bills and unlike /v2/transactions' cursor — so this
// loops a page counter like GetBills does.
//
// It used to read page 1 only, on the belief that the endpoint was
// unpaginated. Every holding seen so far fits in one page, so nothing was
// lost yet, but a history past the page size would have been truncated in
// silence: no error, no rejection, just half a history.
func (a *Adapter) GetInvestmentTransactions(ctx context.Context, externalInvestmentID string) (InvestmentTransactionsPage, error) {
	var allTransactions []InvestmentTransactionSnapshot
	var allRejections []RejectedRecord
	var firstRawImportID string

	for pageNumber := 1; pageNumber <= a.config.MaxPagesPerAccount; pageNumber++ {
		path := "/investments/" + url.PathEscape(externalInvestmentID) + "/transactions?page=" + strconv.Itoa(pageNumber)
		payload, rawImportID, err := a.read(ctx, ScopeInvestmentTransactions, path, StageInvestmentTransactions, pageNumber, &externalInvestmentID)
		if err != nil {
			return InvestmentTransactionsPage{}, err
		}
		if pageNumber == 1 {
			firstRawImportID = rawImportID
		}
		transactions, rejections, marker, err := mapInvestmentTransactionsPage(payload, externalInvestmentID)
		if err != nil {
			var mapErr *MappingError
			errors.As(err, &mapErr)
			return InvestmentTransactionsPage{}, &ProviderError{
				Code: mapErr.Code, Stage: StageInvestmentTransactions, SafeMessage: mapErr.SafeMessage,
				RawImportID: &rawImportID, ExternalAccountID: &externalInvestmentID,
			}
		}
		allTransactions = append(allTransactions, transactions...)
		allRejections = append(allRejections, rejections...)
		if marker.isLast() {
			return InvestmentTransactionsPage{
				RawImportID: firstRawImportID, Transactions: allTransactions, Rejections: allRejections,
			}, nil
		}
	}
	return InvestmentTransactionsPage{}, pageCapExceeded(
		StageInvestmentTransactions, externalInvestmentID, firstRawImportID, a.config.MaxPagesPerAccount)
}

// GetBills fetches every closed/overdue bill for one credit card account.
// Unlike /v2/transactions' cursor pagination, Pluggy's /bills endpoint pages
// by number (page/totalPages in the response body), so this loops a plain
// page counter instead of following a cursor.
func (a *Adapter) GetBills(ctx context.Context, externalAccountID string) (BillsPage, error) {
	var allBills []BillSnapshot
	var allRejections []RejectedRecord
	var firstRawImportID string

	for pageNumber := 1; pageNumber <= a.config.MaxPagesPerAccount; pageNumber++ {
		path := "/bills?accountId=" + url.QueryEscape(externalAccountID) + "&page=" + strconv.Itoa(pageNumber)
		payload, rawImportID, err := a.read(ctx, ScopeBills, path, StageBills, pageNumber, &externalAccountID)
		if err != nil {
			return BillsPage{}, err
		}
		if pageNumber == 1 {
			firstRawImportID = rawImportID
		}
		bills, rejections, marker, err := mapBillsPage(payload)
		if err != nil {
			var mapErr *MappingError
			errors.As(err, &mapErr)
			return BillsPage{}, &ProviderError{
				Code: mapErr.Code, Stage: StageBills, SafeMessage: mapErr.SafeMessage,
				RawImportID: &rawImportID, ExternalAccountID: &externalAccountID,
			}
		}
		allBills = append(allBills, bills...)
		allRejections = append(allRejections, rejections...)
		if marker.isLast() {
			return BillsPage{RawImportID: firstRawImportID, Bills: allBills, Rejections: allRejections}, nil
		}
	}
	return BillsPage{}, pageCapExceeded(StageBills, externalAccountID, firstRawImportID, a.config.MaxPagesPerAccount)
}

// IterTransactionPages mirrors PluggyAdapter.iter_transaction_pages: handle
// is called once per page in order; returning an error from handle aborts
// pagination and propagates that error.
func (a *Adapter) IterTransactionPages(ctx context.Context, externalAccountID string, handle func(TransactionsPage) error) error {
	var cursor *string
	seenCursors := make(map[string]bool)

	for pageSequence := 1; pageSequence <= a.config.MaxPagesPerAccount; pageSequence++ {
		path := "/v2/transactions?accountId=" + url.QueryEscape(externalAccountID)
		if cursor != nil {
			path += "&after=" + url.QueryEscape(*cursor)
		}
		payload, rawImportID, err := a.read(ctx, ScopeTransactions, path, StageTransactions, pageSequence, &externalAccountID)
		if err != nil {
			return err
		}
		transactions, rejections, err := mapTransactionsPage(payload, externalAccountID)
		if err != nil {
			var mapErr *MappingError
			errors.As(err, &mapErr)
			return &ProviderError{Code: mapErr.Code, Stage: StageTransactions, SafeMessage: mapErr.SafeMessage, RawImportID: &rawImportID, ExternalAccountID: &externalAccountID}
		}
		nextCursor, err := extractCursor(payload["next"], externalAccountID)
		if err != nil {
			var mapErr *MappingError
			errors.As(err, &mapErr)
			return &ProviderError{Code: mapErr.Code, Stage: StageTransactions, SafeMessage: mapErr.SafeMessage, RawImportID: &rawImportID, ExternalAccountID: &externalAccountID}
		}

		if err := handle(TransactionsPage{RawImportID: rawImportID, Transactions: transactions, Rejections: rejections, NextCursor: nextCursor}); err != nil {
			return err
		}
		if nextCursor == nil {
			return nil
		}
		if seenCursors[*nextCursor] {
			return &ProviderError{
				Code: "invalid_provider_payload", Stage: StageTransactions,
				SafeMessage: "Provider returned a repeated pagination cursor",
				RawImportID: &rawImportID, ExternalAccountID: &externalAccountID,
			}
		}
		seenCursors[*nextCursor] = true
		cursor = nextCursor
	}
	return &ProviderError{
		Code: "invalid_provider_payload", Stage: StageTransactions,
		SafeMessage: "Provider exceeded the safe pagination limit", ExternalAccountID: &externalAccountID,
	}
}
