import type {
  InvestmentAccount,
  InvestmentOperation,
  InvestmentPosition,
  InvestmentSummary,
} from "../api/contracts";

export const manualAccountId = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1";
export const integratedAccountId = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2";
export const manualPositionId = "cccccccc-cccc-4ccc-8ccc-ccccccccccc1";
export const syncedPositionId = "cccccccc-cccc-4ccc-8ccc-ccccccccccc2";
export const depositOperationId = "dddddddd-dddd-4ddd-8ddd-ddddddddddd1";
export const linkedInvestmentId = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeee1";

export const manualAccount: InvestmentAccount = {
  id: manualAccountId,
  name: "Corretora XP",
  kind: "manual",
  currency_code: "BRL",
  source_id: null,
  source_display_name: null,
  financial_account_id: null,
  active: true,
  cash_balance: "500.00",
  created_at: "2026-09-01T12:00:00Z",
  updated_at: "2026-09-10T12:00:00Z",
};

export const integratedAccount: InvestmentAccount = {
  id: integratedAccountId,
  name: "Banco Teste",
  kind: "integrated",
  currency_code: "BRL",
  source_id: "connection-1",
  source_display_name: "Banco Teste",
  financial_account_id: null,
  active: true,
  cash_balance: "0.00",
  created_at: "2026-09-01T12:00:00Z",
  updated_at: "2026-09-14T12:00:00Z",
};

export const manualPosition: InvestmentPosition = {
  id: manualPositionId,
  source: "manual",
  account_id: manualAccountId,
  asset_id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa9",
  name: "Tesouro Selic 2029",
  ticker: null,
  asset_type: "Tesouro",
  quantity: "1",
  average_cost: "1000.00",
  total_cost: "1000.00",
  current_value: "1200.00",
  current_unit_price: "1200.00",
  valued_on: "2026-09-12",
  currency_code: "BRL",
  closed: false,
  linked_investment_id: null,
  notes: null,
  valuation_basis: "manual_valuation",
};

export const closedPosition: InvestmentPosition = {
  id: "cccccccc-cccc-4ccc-8ccc-ccccccccccc3",
  source: "manual",
  account_id: manualAccountId,
  asset_id: "cccccccc-cccc-4ccc-8ccc-ccccccccccc9",
  name: "LCI Banco Antigo",
  ticker: null,
  asset_type: "LCI",
  quantity: "0",
  average_cost: "500.00",
  total_cost: "0.00",
  current_value: "0.00",
  current_unit_price: "0.00",
  valued_on: "2026-08-01",
  currency_code: "BRL",
  closed: true,
  linked_investment_id: null,
  notes: null,
  valuation_basis: "manual_valuation",
};

export const syncedPosition: InvestmentPosition = {
  id: syncedPositionId,
  source: "synced",
  account_id: integratedAccountId,
  asset_id: null,
  name: "CDB Banco Teste",
  ticker: null,
  asset_type: "CDB",
  quantity: "1",
  average_cost: null,
  current_value: "300.00",
  current_unit_price: null,
  valued_on: "2026-09-14",
  currency_code: "BRL",
  closed: false,
  linked_investment_id: linkedInvestmentId,
  notes: null,
  valuation_basis: "provider_balance",
};

export const depositOperation: InvestmentOperation = {
  id: depositOperationId,
  account_id: manualAccountId,
  position_id: null,
  transfer_id: null,
  kind: "deposit",
  occurred_on: "2026-09-01",
  amount: "2000.00",
  quantity: null,
  unit_price: null,
  fees: null,
  taxes: null,
  notes: null,
  source: "manual",
  is_editable: true,
  created_at: "2026-09-01T12:00:00Z",
  updated_at: "2026-09-01T12:00:00Z",
};

export const buyOperation: InvestmentOperation = {
  ...depositOperation,
  id: "dddddddd-dddd-4ddd-8ddd-ddddddddddd2",
  position_id: manualPositionId,
  kind: "buy",
  occurred_on: "2026-09-02",
  amount: "1000.00",
  quantity: "1",
  unit_price: "1000.00",
};

export const withdrawalOperation: InvestmentOperation = {
  ...depositOperation,
  id: "dddddddd-dddd-4ddd-8ddd-ddddddddddd3",
  kind: "withdrawal",
  occurred_on: "2026-09-05",
  amount: "300.00",
};

export const summary: InvestmentSummary = {
  currency_code: "BRL",
  total_value: "2000.00",
  manual_value: "1200.00",
  synced_value: "300.00",
  cash_balance: "500.00",
  unrealized_gain: "200.00",
  accounts: [
    { account_id: manualAccountId, name: "Corretora XP", kind: "manual", current_value: "1200.00", cash_balance: "500.00" },
    { account_id: integratedAccountId, name: "Banco Teste", kind: "integrated", current_value: "300.00", cash_balance: "0.00" },
  ],
};
