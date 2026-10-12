import { dateOnlyPattern, decimal, isNullableString, isNullableUuid, isUuid, isValidDate, nullableDecimal, requiredRecord, requiredRecordWithOptionalKeys } from "./shared";

// Investment workspace ----------------------------------------------------
//
// These are deliberately separate from Investment above. Investment is the
// preserved, provider-imported detail API; the workspace below is the ledger
// used for custody accounts, goals, cash and manually recorded positions.

export const investmentAccountKinds = ["manual", "integrated", "synced"] as const;

export type InvestmentAccountKind = (typeof investmentAccountKinds)[number];

export interface InvestmentAccount {
  id: string;
  name: string;
  kind: InvestmentAccountKind;
  currency_code: "BRL";
  source_id: string | null;
  source_display_name: string | null;
  financial_account_id: string | null;
  active: boolean;
  cash_balance: string;
  created_at: string;
  updated_at: string;
}

export interface InvestmentAccountWrite {
  name: string;
  currency_code: "BRL";
  source_id?: string | null;
  financial_account_id?: string | null;
}

export interface InvestmentAccountUpdate {
  name?: string;
  active?: boolean;
  financial_account_id?: string | null;
}

const investmentAccountKeys = [
  "id",
  "name",
  "kind",
  "currency_code",
  "source_id",
  "source_display_name",
  "financial_account_id",
  "active",
  "cash_balance",
  "created_at",
  "updated_at",
] as const;

/**
 * An investment account id is not always a uuid: besides the accounts created
 * locally, the backend materialises one grouping row per provider connection
 * with the deterministic id `integrated:<data-source uuid>` (see
 * EnsureIntegratedAccounts and migration 00036/00037). The prefix exists so a
 * local grouping id can never be confused with an imported one — every other
 * investment id (positions, operations, portfolios, reconciliations) stays a
 * plain uuid.
 */
const integratedAccountIdPrefix = "integrated:";

function isInvestmentAccountId(value: unknown): value is string {
  if (typeof value !== "string") return false;
  return value.startsWith(integratedAccountIdPrefix)
    ? isUuid(value.slice(integratedAccountIdPrefix.length))
    : isUuid(value);
}

export function parseInvestmentAccount(value: unknown): InvestmentAccount {
  const item = requiredRecord(value, investmentAccountKeys, "Conta de investimento inválida.");
  if (
    !isInvestmentAccountId(item.id) ||
    typeof item.name !== "string" ||
    item.name.trim() === "" ||
    !investmentAccountKinds.includes(item.kind as InvestmentAccountKind) ||
    item.currency_code !== "BRL" ||
    !isNullableUuid(item.source_id) ||
    !isNullableString(item.source_display_name) ||
    !isNullableUuid(item.financial_account_id) ||
    typeof item.active !== "boolean" ||
    !isValidDate(item.created_at) ||
    !isValidDate(item.updated_at)
  ) {
    throw new TypeError("Conta de investimento inválida.");
  }
  return {
    id: item.id,
    name: item.name,
    kind: item.kind as InvestmentAccountKind,
    currency_code: "BRL",
    source_id: item.source_id as string | null,
    source_display_name: item.source_display_name as string | null,
    financial_account_id: item.financial_account_id as string | null,
    active: item.active,
    cash_balance: decimal(item.cash_balance),
    created_at: item.created_at as string,
    updated_at: item.updated_at as string,
  };
}

function parseInvestmentListEnvelope<T>(
  value: unknown,
  parser: (item: unknown) => T,
  message: string,
): T[] {
  const payload = requiredRecord(value, ["items"], message);
  if (!Array.isArray(payload.items)) throw new TypeError(message);
  return payload.items.map(parser);
}

export function parseInvestmentAccountList(value: unknown): InvestmentAccount[] {
  return parseInvestmentListEnvelope(value, parseInvestmentAccount, "Lista de contas de investimento inválida.");
}

export const investmentAssetClasses = ["fixed_income", "variable_income", "multimarket", "currency", "crypto", "other"] as const;

export type InvestmentAssetClass = typeof investmentAssetClasses[number];

function isInvestmentAssetClass(value: unknown): value is InvestmentAssetClass {
  return investmentAssetClasses.some((item) => item === value);
}

export interface InvestmentAssetClassDefinition {
  asset_class: InvestmentAssetClass;
  label: string;
  types: { name: string; quote_market: "b3" | "crypto" | null }[];
}

export function parseInvestmentAssetClassification(value: unknown): InvestmentAssetClassDefinition[] {
  return parseInvestmentListEnvelope(value, (entry) => {
    const item = requiredRecord(entry, ["asset_class", "label", "types"], "Classificação de ativo inválida.");
    if (!isInvestmentAssetClass(item.asset_class) || typeof item.label !== "string" || item.label.trim() === "" || !Array.isArray(item.types)) {
      throw new TypeError("Classificação de ativo inválida.");
    }
    const types = item.types.map((entry) => {
      const type = requiredRecord(entry, ["name", "quote_market"], "Tipo de ativo inválido.");
      if (typeof type.name !== "string" || type.name.trim() === "" ||
        (type.quote_market !== null && type.quote_market !== "b3" && type.quote_market !== "crypto")) {
        throw new TypeError("Tipo de ativo inválido.");
      }
      return { name: type.name, quote_market: type.quote_market as "b3" | "crypto" | null };
    });
    return { asset_class: item.asset_class, label: item.label, types };
  }, "Classificação de ativos inválida.");
}

export interface InvestmentAsset {
  id: string;
  name: string;
  ticker: string | null;
  asset_type: string;
  asset_class: InvestmentAssetClass;
  currency_code: string;
  quote_source: string | null;
  quote_symbol: string | null;
  created_at: string;
  updated_at: string;
}

export interface InvestmentAssetWrite {
  name: string;
  ticker: string | null;
  asset_type: string;
  asset_class: InvestmentAssetClass;
  currency_code: string;
  quote_source: string | null;
  quote_symbol: string | null;
}

const investmentAssetKeys = [
  "id",
  "name",
  "ticker",
  "asset_type",
  "asset_class",
  "currency_code",
  "quote_source",
  "quote_symbol",
  "created_at",
  "updated_at",
] as const;

export function parseInvestmentAsset(value: unknown): InvestmentAsset {
  const item = requiredRecord(value, investmentAssetKeys, "Ativo de investimento inválido.");
  if (
    typeof item.id !== "string" ||
    !isUuid(item.id) ||
    typeof item.name !== "string" ||
    item.name.trim() === "" ||
    !isNullableString(item.ticker) ||
    typeof item.asset_type !== "string" ||
    item.asset_type.trim() === "" ||
    !isInvestmentAssetClass(item.asset_class) ||
    typeof item.currency_code !== "string" ||
    !/^[A-Z]{3}$/.test(item.currency_code) ||
    !isNullableString(item.quote_source) ||
    !isNullableString(item.quote_symbol) ||
    !isValidDate(item.created_at) ||
    !isValidDate(item.updated_at)
  ) {
    throw new TypeError("Ativo de investimento inválido.");
  }
  return {
    id: item.id,
    name: item.name,
    ticker: item.ticker as string | null,
    asset_type: item.asset_type,
    asset_class: item.asset_class,
    currency_code: item.currency_code,
    quote_source: item.quote_source as string | null,
    quote_symbol: item.quote_symbol as string | null,
    created_at: item.created_at as string,
    updated_at: item.updated_at as string,
  };
}

export function parseInvestmentAssetList(value: unknown): InvestmentAsset[] {
  return parseInvestmentListEnvelope(value, parseInvestmentAsset, "Lista de ativos de investimento inválida.");
}

export interface InvestmentPortfolio {
  id: string;
  name: string;
  target_amount: string | null;
  target_date: string | null;
  notes: string | null;
  current_value: string;
  progress: string | null;
  created_at: string;
  updated_at: string;
}

export interface InvestmentPortfolioWrite {
  name: string;
  target_amount: string | null;
  target_date: string | null;
  notes: string | null;
}

const investmentPortfolioKeys = [
  "id",
  "name",
  "target_amount",
  "target_date",
  "notes",
  "current_value",
  "progress",
  "created_at",
  "updated_at",
] as const;

export function parseInvestmentPortfolio(value: unknown): InvestmentPortfolio {
  const item = requiredRecord(value, investmentPortfolioKeys, "Objetivo de investimento inválido.");
  if (
    typeof item.id !== "string" ||
    !isUuid(item.id) ||
    typeof item.name !== "string" ||
    item.name.trim() === "" ||
    !(item.target_date === null || (typeof item.target_date === "string" && dateOnlyPattern.test(item.target_date))) ||
    !isNullableString(item.notes) ||
    !isValidDate(item.created_at) ||
    !isValidDate(item.updated_at)
  ) {
    throw new TypeError("Objetivo de investimento inválido.");
  }
  return {
    id: item.id,
    name: item.name,
    target_amount: nullableDecimal(item.target_amount),
    target_date: item.target_date as string | null,
    notes: item.notes as string | null,
    current_value: decimal(item.current_value),
    progress: nullableDecimal(item.progress),
    created_at: item.created_at as string,
    updated_at: item.updated_at as string,
  };
}

export function parseInvestmentPortfolioList(value: unknown): InvestmentPortfolio[] {
  return parseInvestmentListEnvelope(value, parseInvestmentPortfolio, "Lista de objetivos inválida.");
}

export const investmentPositionSources = ["manual", "synced"] as const;

export type InvestmentPositionSource = (typeof investmentPositionSources)[number];

export const investmentValuationBases = ["manual_valuation", "cost_basis", "provider_balance", "market_quote"] as const;

export type InvestmentValuationBasis = (typeof investmentValuationBases)[number];

export interface InvestmentPosition {
  id: string;
  source: InvestmentPositionSource;
  account_id: string;
  asset_id: string | null;
  portfolio_id: string | null;
  name: string;
  ticker: string | null;
  asset_type: string;
  quantity: string;
  average_cost: string | null;
  /** Exact backend acquisition cost; absent only in older responses. */
  total_cost?: string | null;
  current_value: string;
  current_unit_price: string | null;
  valued_on: string | null;
  currency_code: string;
  closed: boolean;
  linked_investment_id: string | null;
  notes: string | null;
  valuation_basis: InvestmentValuationBasis;
}

export interface InvestmentPositionWrite {
  account_id: string;
  asset_id?: string | null;
  name: string;
  ticker?: string | null;
  asset_type?: string;
  portfolio_id?: string | null;
  initial_quantity?: string;
  initial_unit_cost?: string;
  initial_value?: string;
  occurred_on?: string;
  notes?: string | null;
}

export interface InvestmentPositionUpdate {
  name: string;
  ticker: string | null;
  asset_type: string;
  portfolio_id: string | null;
  notes: string | null;
}

const investmentPositionKeys = [
  "id",
  "source",
  "account_id",
  "asset_id",
  "portfolio_id",
  "name",
  "ticker",
  "asset_type",
  "quantity",
  "average_cost",
  "current_value",
  "current_unit_price",
  "valued_on",
  "currency_code",
  "closed",
  "linked_investment_id",
  "notes",
  "valuation_basis",
] as const;

export function parseInvestmentPosition(value: unknown): InvestmentPosition {
  const item = requiredRecordWithOptionalKeys(value, investmentPositionKeys, ["total_cost"], "Posição de investimento inválida.");
  if (
    typeof item.id !== "string" ||
    !isUuid(item.id) ||
    !investmentPositionSources.includes(item.source as InvestmentPositionSource) ||
    !isInvestmentAccountId(item.account_id) ||
    !isNullableUuid(item.asset_id) ||
    !isNullableUuid(item.portfolio_id) ||
    typeof item.name !== "string" ||
    item.name.trim() === "" ||
    !isNullableString(item.ticker) ||
    typeof item.asset_type !== "string" ||
    item.asset_type.trim() === "" ||
    !(item.valued_on === null || (typeof item.valued_on === "string" && dateOnlyPattern.test(item.valued_on))) ||
    typeof item.currency_code !== "string" || item.currency_code.length !== 3 ||
    typeof item.closed !== "boolean" ||
    !isNullableUuid(item.linked_investment_id) ||
    !isNullableString(item.notes) ||
    !investmentValuationBases.includes(item.valuation_basis as InvestmentValuationBasis)
  ) {
    throw new TypeError("Posição de investimento inválida.");
  }
  return {
    id: item.id,
    source: item.source as InvestmentPositionSource,
    account_id: item.account_id,
    asset_id: item.asset_id as string | null,
    portfolio_id: item.portfolio_id as string | null,
    name: item.name,
    ticker: item.ticker as string | null,
    asset_type: item.asset_type,
    quantity: decimal(item.quantity),
    average_cost: nullableDecimal(item.average_cost),
    ...(item.total_cost === undefined ? {} : { total_cost: nullableDecimal(item.total_cost) }),
    current_value: decimal(item.current_value),
    current_unit_price: nullableDecimal(item.current_unit_price),
    valued_on: item.valued_on as string | null,
    currency_code: item.currency_code,
    closed: item.closed,
    linked_investment_id: item.linked_investment_id as string | null,
    notes: item.notes as string | null,
    valuation_basis: item.valuation_basis as InvestmentValuationBasis,
  };
}

export function parseInvestmentPositionList(value: unknown): InvestmentPosition[] {
  return parseInvestmentListEnvelope(value, parseInvestmentPosition, "Lista de posições inválida.");
}

export const investmentOperationKinds = [
  "initial_balance",
  "deposit",
  "withdrawal",
  "buy",
  "sell",
  "income",
  "fee",
  "tax",
  "valuation",
  "transfer_out",
  "transfer_in",
] as const;

export type InvestmentOperationKind = (typeof investmentOperationKinds)[number];

export const investmentOperationSources = ["manual", "synced"] as const;

export type InvestmentOperationSource = (typeof investmentOperationSources)[number];

export interface InvestmentOperation {
  id: string;
  account_id: string;
  position_id: string | null;
  transfer_id: string | null;
  kind: InvestmentOperationKind;
  occurred_on: string;
  amount: string;
  quantity: string | null;
  unit_price: string | null;
  fees: string | null;
  taxes: string | null;
  notes: string | null;
  source: InvestmentOperationSource;
  is_editable: boolean;
  created_at: string;
  updated_at: string;
}

export interface InvestmentOperationWrite {
  account_id: string;
  position_id: string | null;
  kind: InvestmentOperationKind;
  occurred_on: string;
  amount?: string;
  quantity?: string | null;
  unit_price?: string | null;
  fees?: string | null;
  taxes?: string | null;
  notes?: string | null;
}

const investmentOperationKeys = [
  "id",
  "account_id",
  "position_id",
  "transfer_id",
  "kind",
  "occurred_on",
  "amount",
  "quantity",
  "unit_price",
  "fees",
  "taxes",
  "notes",
  "source",
  "is_editable",
  "created_at",
  "updated_at",
] as const;

export function parseInvestmentOperation(value: unknown): InvestmentOperation {
  const item = requiredRecord(value, investmentOperationKeys, "Movimentação de carteira inválida.");
  if (
    typeof item.id !== "string" ||
    !isUuid(item.id) ||
    !isInvestmentAccountId(item.account_id) ||
    !isNullableUuid(item.position_id) ||
    !isNullableUuid(item.transfer_id) ||
    !investmentOperationKinds.includes(item.kind as InvestmentOperationKind) ||
    typeof item.occurred_on !== "string" ||
    !dateOnlyPattern.test(item.occurred_on) ||
    !isNullableString(item.notes) ||
    !investmentOperationSources.includes(item.source as InvestmentOperationSource) ||
    typeof item.is_editable !== "boolean" ||
    !isValidDate(item.created_at) ||
    !isValidDate(item.updated_at)
  ) {
    throw new TypeError("Movimentação de carteira inválida.");
  }
  return {
    id: item.id,
    account_id: item.account_id,
    position_id: item.position_id as string | null,
    transfer_id: item.transfer_id as string | null,
    kind: item.kind as InvestmentOperationKind,
    occurred_on: item.occurred_on,
    amount: decimal(item.amount),
    quantity: nullableDecimal(item.quantity),
    unit_price: nullableDecimal(item.unit_price),
    fees: nullableDecimal(item.fees),
    taxes: nullableDecimal(item.taxes),
    notes: item.notes as string | null,
    source: item.source as InvestmentOperationSource,
    is_editable: item.is_editable,
    created_at: item.created_at as string,
    updated_at: item.updated_at as string,
  };
}

export function parseInvestmentOperationList(value: unknown): InvestmentOperation[] {
  return parseInvestmentListEnvelope(value, parseInvestmentOperation, "Lista de movimentações de carteira inválida.");
}

export interface InvestmentReconciliation {
  id: string;
  operation_id: string;
  financial_transaction_id: string | null;
  financial_investment_transaction_id: string | null;
  amount: string;
  created_at: string;
}

export interface InvestmentReconciliationWrite {
  operation_id?: string | null;
  financial_transaction_id?: string | null;
  financial_investment_transaction_id?: string | null;
  amount: string;
}

const investmentReconciliationKeys = [
  "id",
  "operation_id",
  "financial_transaction_id",
  "financial_investment_transaction_id",
  "amount",
  "created_at",
] as const;

export function parseInvestmentReconciliation(value: unknown): InvestmentReconciliation {
  const item = requiredRecord(value, investmentReconciliationKeys, "Vínculo de investimento inválido.");
  if (
    typeof item.id !== "string" ||
    !isUuid(item.id) ||
    typeof item.operation_id !== "string" ||
    !isUuid(item.operation_id) ||
    !isNullableUuid(item.financial_transaction_id) ||
    !isNullableUuid(item.financial_investment_transaction_id) ||
    (item.financial_transaction_id === null && item.financial_investment_transaction_id === null) ||
    !isValidDate(item.created_at)
  ) {
    throw new TypeError("Vínculo de investimento inválido.");
  }
  return {
    id: item.id,
    operation_id: item.operation_id,
    financial_transaction_id: item.financial_transaction_id as string | null,
    financial_investment_transaction_id: item.financial_investment_transaction_id as string | null,
    amount: decimal(item.amount),
    created_at: item.created_at as string,
  };
}

export function parseInvestmentReconciliationList(value: unknown): InvestmentReconciliation[] {
  return parseInvestmentListEnvelope(value, parseInvestmentReconciliation, "Lista de vínculos inválida.");
}

export interface InvestmentSummaryPortfolio {
  portfolio_id: string | null;
  name: string;
  current_value: string;
  target_amount: string | null;
  progress: string | null;
}

export interface InvestmentSummaryAccount {
  account_id: string;
  name: string;
  kind: InvestmentAccountKind;
  current_value: string;
  cash_balance: string;
}

export interface InvestmentSummary {
  currency_code: "BRL";
  total_value: string;
  manual_value: string;
  synced_value: string;
  cash_balance: string;
  unrealized_gain: string;
  portfolios: InvestmentSummaryPortfolio[];
  accounts: InvestmentSummaryAccount[];
}

function parseInvestmentSummaryPortfolio(value: unknown): InvestmentSummaryPortfolio {
  const item = requiredRecord(
    value,
    ["portfolio_id", "name", "current_value", "target_amount", "progress"],
    "Resumo de objetivo inválido.",
  );
  if (!isNullableUuid(item.portfolio_id) || typeof item.name !== "string" || item.name.trim() === "") {
    throw new TypeError("Resumo de objetivo inválido.");
  }
  return {
    portfolio_id: item.portfolio_id as string | null,
    name: item.name,
    current_value: decimal(item.current_value),
    target_amount: nullableDecimal(item.target_amount),
    progress: nullableDecimal(item.progress),
  };
}

function parseInvestmentSummaryAccount(value: unknown): InvestmentSummaryAccount {
  const item = requiredRecord(
    value,
    ["account_id", "name", "kind", "current_value", "cash_balance"],
    "Resumo de conta de investimento inválido.",
  );
  if (
    !isInvestmentAccountId(item.account_id) ||
    typeof item.name !== "string" ||
    item.name.trim() === "" ||
    !investmentAccountKinds.includes(item.kind as InvestmentAccountKind)
  ) {
    throw new TypeError("Resumo de conta de investimento inválido.");
  }
  return {
    account_id: item.account_id,
    name: item.name,
    kind: item.kind as InvestmentAccountKind,
    current_value: decimal(item.current_value),
    cash_balance: decimal(item.cash_balance),
  };
}

export function parseInvestmentSummary(value: unknown): InvestmentSummary {
  const item = requiredRecord(
    value,
    [
      "currency_code",
      "total_value",
      "manual_value",
      "synced_value",
      "cash_balance",
      "unrealized_gain",
      "portfolios",
      "accounts",
    ],
    "Resumo de investimentos inválido.",
  );
  if (item.currency_code !== "BRL" || !Array.isArray(item.portfolios) || !Array.isArray(item.accounts)) {
    throw new TypeError("Resumo de investimentos inválido.");
  }
  return {
    currency_code: "BRL",
    total_value: decimal(item.total_value),
    manual_value: decimal(item.manual_value),
    synced_value: decimal(item.synced_value),
    cash_balance: decimal(item.cash_balance),
    unrealized_gain: decimal(item.unrealized_gain),
    portfolios: item.portfolios.map(parseInvestmentSummaryPortfolio),
    accounts: item.accounts.map(parseInvestmentSummaryAccount),
  };
}
