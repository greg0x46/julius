package httpapi

import (
	"database/sql"
	"net/http"
)

// TransactionReadQuery shares the REST validation and filter contract with MCP.
type TransactionReadQuery = transactionQueryRequest

// FinancialReadHandlers is an allowlist for authenticated non-REST transports.
// Callers must enforce authentication; none of these handlers mutate owner data.
func FinancialReadHandlers(conn *sql.DB) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"list_accounts":        handleListAccounts(conn),
		"list_card_bills":      handleListAccountBills(conn),
		"search_transactions":  handleQueryTransactions(conn),
		"list_categories":      handleListCategories(conn),
		"spending_by_category": handleCategoryBreakdown(conn),
	}
}
