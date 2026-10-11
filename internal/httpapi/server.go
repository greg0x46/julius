package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/ledger"
	"github.com/greg0x46/julius/internal/payables"
	"github.com/greg0x46/julius/internal/recurrences"
	"github.com/greg0x46/julius/internal/settings"
	"github.com/greg0x46/julius/internal/transactions"
)

// onIgnoredHook wires the "a transaction just became ignored" cleanup into
// every inclusion-changing path this package exposes (manual PUT,
// automation's apply-to-new and apply-retroactively), matching the reference
// calling unlink_if_present from those same places.
//
// All three links it drops rest on the same fact: a transaction the user
// excluded from the totals is no longer money the app counts, so it can
// neither pay down a payable, carry a recurring commitment's occurrence nor
// be the parcel that funded an investment operation. A detached occurrence
// is untouched — that decision is about the occurrence, not about any
// transaction.
func onIgnoredHook(ctx context.Context, q transactions.Querier, transactionID string) error {
	if err := payables.UnlinkIfPresent(ctx, q, transactionID); err != nil {
		return err
	}
	if err := recurrences.UnlinkIfPresent(ctx, q, transactionID); err != nil {
		return err
	}
	return investments.UnlinkTransactionIfPresent(ctx, q, transactionID)
}

// NewServer protects API routes with per-browser authentication. The separate
// secrets holder is used only to encrypt configuration, never to grant access.
func NewServer(db *sql.DB, frontend fs.FS, secrets *settings.Secrets, config auth.Config) http.Handler {
	if err := config.Validate(); err != nil {
		panic(err)
	}
	authentication := newAuthAPI(auth.NewStore(db), config)
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", handleHealth(db))

	mux.HandleFunc("POST /api/auth/login", authentication.login)
	mux.HandleFunc("GET /api/auth/session", authentication.session)
	mux.HandleFunc("POST /api/auth/logout", authentication.logout)
	mux.HandleFunc("PUT /api/auth/password", authentication.password)
	mux.HandleFunc("PUT /api/auth/config", authentication.updateConfig)
	mux.HandleFunc("PUT /api/settings/pluggy", handlePluggySettings(db, secrets))
	mux.HandleFunc("PUT /api/settings/quotes", handleQuotesSettings(db, secrets))

	mux.HandleFunc("GET /api/preferences", handleGetPreferences(db))
	mux.HandleFunc("PUT /api/preferences", handleUpdatePreferences(db))

	mux.HandleFunc("GET /api/categories", handleListCategories(db))
	mux.HandleFunc("POST /api/categories", handleCreateCategory(db))
	mux.HandleFunc("PATCH /api/categories/{id}", handleUpdateCategory(db))

	mux.HandleFunc("POST /api/transactions/query", handleQueryTransactions(db))
	mux.HandleFunc("GET /api/transactions/spending-by-category", handleSpendingByCategory(db))
	mux.HandleFunc("GET /api/transactions/category-breakdown", handleCategoryBreakdown(db))
	manual := &ledger.Service{DB: db, OnIgnored: onIgnoredHook}
	mux.HandleFunc("POST /api/transactions", handleCreateManualTransaction(manual))
	mux.HandleFunc("PUT /api/transactions/{id}", handleUpdateManualTransaction(manual))
	mux.HandleFunc("DELETE /api/transactions/{id}", handleDeleteManualTransaction(manual))
	mux.HandleFunc("PUT /api/transactions/{id}/inclusion", handleSetTransactionInclusion(db))
	mux.HandleFunc("PUT /api/transactions/{id}/category", handleSetTransactionCategory(db))
	mux.HandleFunc("GET /api/transactions/{id}/reconciliation", handleGetTransactionReconciliation(db))

	mux.HandleFunc("GET /api/data-sources", handleListDataSources(db))
	mux.HandleFunc("GET /api/data-sources/{id}", handleGetDataSource(db))
	mux.HandleFunc("POST /api/data-sources", handleCreateDataSource(db))
	mux.HandleFunc("PATCH /api/data-sources/{id}", handleUpdateDataSource(db))

	mux.HandleFunc("POST /api/sync-runs", handleCreateSyncRun(db))
	mux.HandleFunc("GET /api/sync-runs", handleListSyncRuns(db))
	mux.HandleFunc("GET /api/sync-runs/{id}", handleGetSyncRun(db))

	mux.HandleFunc("POST /api/statement-imports/preview", handleStatementPreview(db))
	mux.HandleFunc("POST /api/statement-imports", handleStatementConfirm(db))
	mux.HandleFunc("GET /api/statement-imports", handleStatementHistory(db))
	mux.HandleFunc("GET /api/statement-imports/accounts", handleStatementAccounts(db))
	mux.HandleFunc("GET /api/statement-imports/template", handleStatementTemplate())

	mux.HandleFunc("GET /api/accounts", handleListAccounts(db))
	mux.HandleFunc("GET /api/accounts/{id}", handleGetAccount(db))
	mux.HandleFunc("GET /api/accounts/{id}/cards", handleListAccountCards(db))
	mux.HandleFunc("GET /api/accounts/{id}/bills", handleListAccountBills(db))
	mux.HandleFunc("PUT /api/accounts/{id}/closing-day", handleSetAccountClosingDay(db))

	mux.HandleFunc("GET /api/investments", handleListInvestments(db))
	mux.HandleFunc("GET /api/investments/{id}", handleGetInvestment(db))
	mux.HandleFunc("GET /api/investments/{id}/transactions", handleListInvestmentTransactions(db))

	// The investment workspace: custody accounts, goals, manual positions,
	// their operations and the links that explain a bank line. It is a
	// sibling of /api/investments above, never a sub-collection of it: that
	// one answers what the provider reported about a holding, this one owns
	// the local ledger, and only the local one accepts writes.
	mux.HandleFunc("GET /api/investment-accounts", handleListInvestmentAccounts(db))
	mux.HandleFunc("POST /api/investment-accounts", handleCreateInvestmentAccount(db))
	mux.HandleFunc("PUT /api/investment-accounts/{id}", handleUpdateInvestmentAccount(db))
	mux.HandleFunc("DELETE /api/investment-accounts/{id}", handleDeleteInvestmentAccount(db))

	mux.HandleFunc("GET /api/investment-portfolios", handleListInvestmentPortfolios(db))
	mux.HandleFunc("POST /api/investment-portfolios", handleCreateInvestmentPortfolio(db))
	mux.HandleFunc("PUT /api/investment-portfolios/{id}", handleUpdateInvestmentPortfolio(db))
	mux.HandleFunc("DELETE /api/investment-portfolios/{id}", handleDeleteInvestmentPortfolio(db))

	mux.HandleFunc("GET /api/investment-assets", handleListInvestmentAssets(db))
	mux.HandleFunc("GET /api/investment-asset-classification", handleInvestmentAssetClassification)
	mux.HandleFunc("POST /api/investment-assets", handleCreateInvestmentAsset(db))
	mux.HandleFunc("PUT /api/investment-assets/{id}", handleUpdateInvestmentAsset(db))
	mux.HandleFunc("DELETE /api/investment-assets/{id}", handleDeleteInvestmentAsset(db))

	mux.HandleFunc("GET /api/investment-positions", handleListInvestmentPositions(db))
	mux.HandleFunc("GET /api/investment-positions/{id}", handleGetInvestmentPosition(db))
	mux.HandleFunc("GET /api/investment-positions/{id}/yield", handleGetInvestmentPositionYield(db))
	mux.HandleFunc("POST /api/investment-positions", handleCreateInvestmentPosition(db))
	mux.HandleFunc("PUT /api/investment-positions/{id}", handleUpdateInvestmentPosition(db))
	mux.HandleFunc("DELETE /api/investment-positions/{id}", handleDeleteInvestmentPosition(db))

	mux.HandleFunc("GET /api/investment-operations", handleListInvestmentOperations(db))
	mux.HandleFunc("POST /api/investment-operations/batch", handleCreateInvestmentOperations(db))
	mux.HandleFunc("POST /api/investment-operations", handleCreateInvestmentOperation(db))
	mux.HandleFunc("POST /api/investment-transfers", handleCreateInvestmentTransfer(db))
	mux.HandleFunc("DELETE /api/investment-transfers/{id}", handleDeleteInvestmentTransfer(db))
	mux.HandleFunc("PUT /api/investment-operations/{id}", handleUpdateInvestmentOperation(db))
	mux.HandleFunc("DELETE /api/investment-operations/{id}", handleDeleteInvestmentOperation(db))

	mux.HandleFunc("GET /api/investment-reconciliations", handleListInvestmentReconciliations(db))
	mux.HandleFunc("POST /api/investment-reconciliations", handleCreateInvestmentReconciliation(db))
	mux.HandleFunc("DELETE /api/investment-reconciliations/{id}", handleDeleteInvestmentReconciliation(db))

	mux.HandleFunc("GET /api/investment-summary", handleGetInvestmentSummary(db))
	mux.HandleFunc("GET /api/investment-monthly-movements", handleListInvestmentMonthlyMovements(db))

	mux.HandleFunc("GET /api/automation-rules/condition-options", handleListConditionOptions(db))
	mux.HandleFunc("GET /api/automation-rules", handleListAutomationRules(db))
	mux.HandleFunc("POST /api/automation-rules", handleCreateAutomationRule(db))
	mux.HandleFunc("PUT /api/automation-rules/{id}", handleUpdateAutomationRule(db))
	mux.HandleFunc("PATCH /api/automation-rules/{id}", handleSetAutomationRuleActive(db))
	mux.HandleFunc("DELETE /api/automation-rules/{id}", handleDeleteAutomationRule(db))

	mux.HandleFunc("GET /api/payables", handleListPayables(db))
	mux.HandleFunc("POST /api/payables", handleCreatePayable(db))
	mux.HandleFunc("GET /api/payables/total-owed", handlePayableTotalOwed(db))
	mux.HandleFunc("GET /api/payables/total-to-receive", handlePayableTotalToReceive(db))
	mux.HandleFunc("GET /api/payables/eligible-transactions", handleListEligiblePayableTransactions(db))
	mux.HandleFunc("GET /api/payables/{id}", handleGetPayable(db))
	mux.HandleFunc("PUT /api/payables/{id}", handleUpdatePayable(db))
	mux.HandleFunc("DELETE /api/payables/{id}", handleDeletePayable(db))
	mux.HandleFunc("POST /api/payables/{id}/links", handleCreatePayableLink(db))
	mux.HandleFunc("DELETE /api/payables/{id}/links/{linkId}", handleDeletePayableLink(db))

	mux.HandleFunc("GET /api/payables/{id}/scenarios", handleListPayableScenarios(db))
	mux.HandleFunc("POST /api/payables/{id}/scenarios", handleCreatePayableScenario(db))
	mux.HandleFunc("GET /api/scenarios", handleListScenarios(db))
	mux.HandleFunc("POST /api/scenarios", handleCreateStandaloneScenario(db))
	mux.HandleFunc("GET /api/scenarios/{id}", handleGetScenario(db))
	mux.HandleFunc("PATCH /api/scenarios/{id}", handlePatchScenario(db))
	mux.HandleFunc("DELETE /api/scenarios/{id}", handleDeleteScenario(db))
	mux.HandleFunc("GET /api/scenarios/{id}/planned-transactions", handleListScenarioPlannedTransactions(db))
	mux.HandleFunc("GET /api/scenarios/{id}/realizations", handleListScenarioRealizations(db))
	mux.HandleFunc("PUT /api/scenarios/{id}/planned-transactions/{eventKey}/realization", handlePutScenarioPlannedRealization(db))
	mux.HandleFunc("DELETE /api/scenarios/{id}/planned-transactions/{eventKey}/realization", handleDeleteScenarioPlannedRealization(db))
	mux.HandleFunc("POST /api/scenarios/{id}/generate-installments", handleGenerateInstallments(db))
	mux.HandleFunc("POST /api/scenarios/{id}/readjust", handleReadjustInstallments(db))
	mux.HandleFunc("POST /api/scenarios/{id}/transactions", handleCreateScenarioTransaction(db))
	mux.HandleFunc("PUT /api/scenarios/{id}/transactions/{transactionId}", handleUpdateScenarioTransaction(db))
	mux.HandleFunc("DELETE /api/scenarios/{id}/transactions/{transactionId}", handleDeleteScenarioTransaction(db))
	mux.HandleFunc("POST /api/scenarios/{id}/transactions/{transactionId}/realizations", handleCreateRealization(db))
	mux.HandleFunc("DELETE /api/scenarios/{id}/transactions/{transactionId}/realizations/{realizationId}", handleDeleteRealization(db))

	mux.HandleFunc("GET /api/timeline", handleGetTimeline(db))
	mux.HandleFunc("GET /api/timeline/range", handleGetTimelineDataRange(db))

	mux.HandleFunc("GET /api/net-worth", handleGetNetWorth(db))

	// A recurring commitment is a Scenario of kind 'recurring': every {id}
	// below is a scenario id — the same one the projection, the automation
	// reconcile target and the occurrence decisions are keyed by.
	//
	// The write collection is a sibling of /api/scenarios rather than a
	// "recurring" sub-collection nested under it, because creating and
	// editing one carries a schedule the generic scenario endpoints know
	// nothing about — and because a literal segment sitting where
	// /api/scenarios/{id} expects a wildcard is a trap: ServeMux refuses to
	// register "/api/scenarios/recurring/{id}" alongside
	// "/api/scenarios/{id}/…" for the same method (both match
	// "/api/scenarios/recurring/occurrences" and neither is more specific),
	// so the next route added there would panic at startup. A distinct first
	// segment cannot collide.
	mux.HandleFunc("GET /api/recurring-scenarios", handleListRecurringCommitments(db))
	mux.HandleFunc("POST /api/recurring-scenarios", handleCreateRecurringCommitment(db))
	mux.HandleFunc("PUT /api/recurring-scenarios/{id}", handleUpdateRecurringCommitment(db))
	mux.HandleFunc("PATCH /api/recurring-scenarios/{id}", handleSetRecurringCommitmentActive(db))
	mux.HandleFunc("DELETE /api/recurring-scenarios/{id}", handleDeleteRecurringCommitment(db))

	// Occurrences are addressed as plain scenario sub-resources: an
	// occurrence belongs to the scenario, not to the recurring write shape.
	mux.HandleFunc("GET /api/scenarios/{id}/occurrences", handleListRecurrenceOccurrences(db))
	mux.HandleFunc("GET /api/scenarios/{id}/occurrences/{date}/candidates", handleListReconciliationCandidates(db))
	mux.HandleFunc("PUT /api/scenarios/{id}/occurrences/{date}/reconciliation", handlePutRecurrenceReconciliation(db))
	mux.HandleFunc("DELETE /api/scenarios/{id}/occurrences/{date}/reconciliation", handleDeleteRecurrenceReconciliation(db))

	mux.Handle("/", spaHandler(frontend))

	return authentication.gate(mux)
}

func handleHealth(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := db.PingContext(r.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// spaHandler mirrors register_spa_host: it serves a file from the embedded
// build when the request path names one, falls back to index.html for any
// other path so client-side routing works, and returns a 404 JSON body
// (never the SPA shell) for anything under /api/ or /health that no other
// handler claimed.
func spaHandler(frontend fs.FS) http.Handler {
	fileServer := http.FileServerFS(frontend)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath := strings.TrimPrefix(r.URL.Path, "/")
		if requestedPath == "health" || strings.HasPrefix(requestedPath, "api/") || strings.HasPrefix(requestedPath, "health/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Not Found"})
			return
		}

		if requestedPath != "" {
			if f, err := frontend.Open(requestedPath); err == nil {
				info, statErr := f.Stat()
				f.Close()
				if statErr == nil && !info.IsDir() {
					fileServer.ServeHTTP(w, r)
					return
				}
			}
			if path.Ext(requestedPath) != "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Not Found"})
				return
			}
		}

		// Client-side routes (e.g. /transactions) and the bare "/" both fall
		// through to index.html. Serving its bytes directly (rather than
		// rewriting the request path and delegating to fileServer) avoids
		// http.FileServer's built-in redirect-away-from-"index.html"
		// behavior, which would otherwise bounce every SPA route in a loop.
		index, err := fs.ReadFile(frontend, "index.html")
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Not Found"})
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(index)
	})
}
