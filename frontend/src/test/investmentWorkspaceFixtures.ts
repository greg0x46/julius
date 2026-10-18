import type {
  InvestmentAccount,
  InvestmentOperation,
  InvestmentPeriod,
  InvestmentPeriodGroup,
  InvestmentPeriodPart,
  InvestmentPeriodPosition,
  InvestmentPeriodTotals,
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

// Period view (GET /api/investment-portfolio) ---------------------------------
//
// Four positions: an open manual one, a manual one closed at the end of the
// period, a synced one with an incomplete history and one with no history at
// all. Every figure is what the backend would state; the identity
// start + contributions - withdrawals + yield = end holds at each level.

export const closedPositionId = "cccccccc-cccc-4ccc-8ccc-ccccccccccc3";
export const unavailablePositionId = "cccccccc-cccc-4ccc-8ccc-ccccccccccc4";

const emptyPeriodPosition: InvestmentPeriodPosition = {
  id: manualPositionId,
  source: "manual",
  account_id: manualAccountId,
  account_name: "Corretora XP",
  asset_id: null,
  name: "",
  ticker: null,
  asset_type: "Tesouro",
  asset_class: "fixed_income",
  currency_code: "BRL",
  quantity: "1",
  closed_as_of_to: false,
  unavailable_reason: null,
  opening_on: "2025-08-31",
  partial_start: false,
  history_complete: true,
  start_value: "0",
  end_value: "0",
  contributions: "0",
  withdrawals: "0",
  yield: "0",
  capital_base: "0",
  yield_rate: null,
  share: "0",
  native: { start_value: "0", end_value: "0", contributions: "0", withdrawals: "0", yield: "0", invested: null },
  invested: null,
  invested_unavailable_reason: null,
  costs: null,
  provisioned_taxes: null,
  reported_profit: null,
  reported_as_of: null,
};

export const periodTesouro: InvestmentPeriodPosition = {
  ...emptyPeriodPosition,
  name: "Tesouro Selic 2029",
  start_value: "1000.00",
  end_value: "1200.00",
  yield: "200.00",
  capital_base: "1000.00",
  yield_rate: "0.2",
  share: "0.8",
  native: { start_value: "1000.00", end_value: "1200.00", contributions: "0", withdrawals: "0", yield: "200.00", invested: "1000.00" },
  invested: "1000.00",
};

export const periodClosed: InvestmentPeriodPosition = {
  ...emptyPeriodPosition,
  id: closedPositionId,
  name: "LCI Banco Antigo",
  asset_type: "LCI",
  quantity: "0",
  closed_as_of_to: true,
  start_value: "500.00",
  end_value: "0",
  withdrawals: "520.00",
  yield: "20.00",
  capital_base: "500.00",
  yield_rate: "0.04",
  share: "0",
  native: { start_value: "500.00", end_value: "0", contributions: "0", withdrawals: "520.00", yield: "20.00", invested: "0" },
  invested: "0",
};

export const periodSynced: InvestmentPeriodPosition = {
  ...emptyPeriodPosition,
  id: syncedPositionId,
  source: "synced",
  account_id: integratedAccountId,
  account_name: "Banco Teste",
  name: "CDB Banco Teste",
  asset_type: "CDB",
  partial_start: true,
  history_complete: false,
  start_value: "280.00",
  end_value: "300.00",
  yield: "20.00",
  capital_base: "280.00",
  yield_rate: "0.0714285714",
  share: "0.2",
  native: { start_value: "280.00", end_value: "300.00", contributions: "0", withdrawals: "0", yield: "20.00", invested: null },
  invested: null,
  invested_unavailable_reason: "historico_incompleto",
  provisioned_taxes: "3.00",
  reported_profit: "21.50",
  reported_as_of: "2025-09-30",
};

export const periodUnavailable: InvestmentPeriodPosition = {
  ...emptyPeriodPosition,
  id: unavailablePositionId,
  source: "synced",
  account_id: integratedAccountId,
  account_name: "Banco Teste",
  name: "Fundo Sem Histórico",
  asset_type: "MUTUAL_FUND",
  asset_class: "multimarket",
  quantity: null,
  unavailable_reason: "sem_historico",
  opening_on: null,
  history_complete: false,
  start_value: null,
  end_value: null,
  contributions: null,
  withdrawals: null,
  yield: null,
  capital_base: null,
  share: null,
  native: null,
  invested_unavailable_reason: "sem_historico",
};

const zeroPart: InvestmentPeriodPart = {
  start_value: "0", end_value: "0", contributions: "0", withdrawals: "0", yield: "0", capital_base: "0", yield_rate: null,
};

// Positions open at the end of the period, and the one closed inside it (the LCI).
const heldPart: InvestmentPeriodPart = {
  start_value: "1280.00", end_value: "1500.00", contributions: "0", withdrawals: "0", yield: "220.00",
  capital_base: "1280.00", yield_rate: "0.171875",
};
const closedPart: InvestmentPeriodPart = {
  start_value: "500.00", end_value: "0", contributions: "0", withdrawals: "520.00", yield: "20.00",
  capital_base: "500.00", yield_rate: "0.04",
};

function totals(overrides: Partial<InvestmentPeriodTotals>): InvestmentPeriodTotals {
  return {
    held: zeroPart,
    closed: zeroPart,
    start_value: "0",
    end_value: "0",
    contributions: "0",
    withdrawals: "0",
    yield: "0",
    capital_base: "0",
    yield_rate: null,
    invested: "0",
    invested_unknown_positions: 0,
    costs: "0",
    provisioned_taxes: "0",
    positions_total: 0,
    positions_unavailable: 0,
    positions_partial_start: 0,
    partial: false,
    ...overrides,
  };
}

const manualTotals = totals({
  start_value: "1500.00", end_value: "1200.00", withdrawals: "520.00", yield: "220.00",
  capital_base: "1500.00", yield_rate: "0.1466666667", invested: "1000.00", positions_total: 2,
  held: { ...heldPart, start_value: "1000.00", end_value: "1200.00", yield: "200.00", capital_base: "1000.00", yield_rate: "0.2" },
  closed: closedPart,
});
const syncedTotals = totals({
  start_value: "280.00", end_value: "300.00", yield: "20.00", capital_base: "280.00",
  yield_rate: "0.0714285714", invested_unknown_positions: 1, provisioned_taxes: "3.00",
  positions_total: 2, positions_unavailable: 1, positions_partial_start: 1, partial: true,
  held: { ...heldPart, start_value: "280.00", end_value: "300.00", yield: "20.00", capital_base: "280.00", yield_rate: "0.0714285714" },
});
export const periodTotals = totals({
  start_value: "1780.00", end_value: "1500.00", withdrawals: "520.00", yield: "240.00",
  capital_base: "1780.00", yield_rate: "0.1348314607", invested: "1000.00",
  invested_unknown_positions: 1, provisioned_taxes: "3.00", positions_total: 4,
  positions_unavailable: 1, positions_partial_start: 1, partial: true,
  held: heldPart, closed: closedPart,
});

const manualGroup: InvestmentPeriodGroup = {
  key: manualAccountId, label: "Corretora XP", account_id: manualAccountId, asset_class: null,
  share: "0.8", totals: manualTotals, positions: [periodTesouro, periodClosed],
};
const syncedGroup: InvestmentPeriodGroup = {
  key: integratedAccountId, label: "Banco Teste", account_id: integratedAccountId, asset_class: null,
  share: "0.2", totals: syncedTotals, positions: [periodSynced, periodUnavailable],
};

const reference = {
  currency_code: "BRL",
  fx: {
    base: "BRL", quote: "BRL", value: "1", inverted: false, as_of: "2025-09-30T00:00:00Z",
    effective_on: "2025-09-30T00:00:00Z", source: "identity", fetched_at: "2025-09-30T12:00:00Z", stale: false,
  },
};

/** The period response for each grouping; totals do not depend on it. */
export function investmentPeriodFixture(overrides: Partial<InvestmentPeriod> = {}): InvestmentPeriod {
  const groupBy = overrides.group_by ?? "account";
  const groups: InvestmentPeriodGroup[] =
    groupBy === "account"
      ? [manualGroup, syncedGroup]
      : groupBy === "class"
        ? [
            {
              key: "fixed_income", label: "Renda fixa", account_id: null, asset_class: "fixed_income", share: "1",
              totals: { ...periodTotals, positions_total: 3, positions_unavailable: 0, invested_unknown_positions: 1, partial: false },
              positions: [periodTesouro, periodSynced, periodClosed],
            },
            {
              key: "multimarket", label: "Multimercado", account_id: null, asset_class: "multimarket", share: null,
              totals: totals({ positions_total: 1, positions_unavailable: 1, invested_unknown_positions: 1, partial: true }),
              positions: [periodUnavailable],
            },
          ]
        : [
            {
              key: "all", label: "Carteira", account_id: null, asset_class: null, share: "1", totals: periodTotals,
              positions: [periodTesouro, periodSynced, periodClosed, periodUnavailable],
            },
          ];
  return {
    ...reference,
    from: "2025-09-01",
    to: "2025-09-30",
    to_clamped: false,
    group_by: groupBy,
    source: null,
    account_id: null,
    native_rates: {},
    totals: periodTotals,
    groups,
    ...overrides,
  };
}
