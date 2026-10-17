import { dateOnlyPattern, decimal, isNullableString, isNullableUuid, isRecord, isUuid, isValidDate, requiredRecord } from "./shared";

export const scenarioKinds = ["debt_plan", "receivable_plan", "standalone", "recurring"] as const;

export type ScenarioKind = (typeof scenarioKinds)[number];

export interface Scenario {
  id: string;
  kind: ScenarioKind;
  name: string;
  payable_id: string | null;
  // Optional only for responses from the pre-unification compatibility API;
  // the canonical Scenario endpoint always supplies both booleans.
  is_active?: boolean;
  is_accounting_source?: boolean;
  created_at: string;
  updated_at: string;
}

export const scenarioTransactionStatuses = [
  "atrasada",
  "projetada",
  "paga_parcialmente",
  "paga",
  "paga_a_mais",
] as const;

export type ScenarioTransactionStatus = (typeof scenarioTransactionStatuses)[number];

export interface Realization {
  id: string;
  payable_link_id: string | null;
  allocated_amount: string;
  created_at: string;
}

export interface ScenarioTransaction {
  id: string;
  scenario_id: string;
  description: string;
  amount: string;
  projected_at: string;
  category: string | null;
  status: ScenarioTransactionStatus;
  realizations: Realization[];
}

export interface ScenarioDetail extends Scenario {
  transactions: ScenarioTransaction[];
  accumulated_deviation: string;
}

export interface ScenarioCreate {
  name: string;
}

export interface ScenarioTransactionWrite {
  description: string;
  amount: string;
  projected_at: string;
  category?: string | null;
}

export interface RealizationWrite {
  payable_link_id: string;
  allocated_amount: string;
}

export const cadences = ["mensal", "semanal", "quinzenal"] as const;

export type Cadence = (typeof cadences)[number];

export interface GenerateInstallmentsWrite {
  cadence: Cadence;
  months?: number;
  installment_amount?: string;
  start_date?: string;
}

export const readjustStrategies = ["abater_do_final", "redistribuir"] as const;

export type ReadjustStrategy = (typeof readjustStrategies)[number];

export interface ReadjustWrite {
  strategy: ReadjustStrategy;
}

const legacyScenarioKeys = ["id", "kind", "name", "payable_id", "created_at", "updated_at"] as const;

const scenarioKeys = [
  ...legacyScenarioKeys,
  "is_active",
  "is_accounting_source",
] as const;

function scenarioFieldsFrom(scenario: Record<string, unknown>): Scenario {
  if (
    typeof scenario.id !== "string" ||
    !isUuid(scenario.id) ||
    !scenarioKinds.includes(scenario.kind as ScenarioKind) ||
    typeof scenario.name !== "string" ||
    scenario.name === "" ||
    !isNullableUuid(scenario.payable_id) ||
    (scenario.is_active !== undefined && typeof scenario.is_active !== "boolean") ||
    (scenario.is_accounting_source !== undefined && typeof scenario.is_accounting_source !== "boolean") ||
    !isValidDate(scenario.created_at) ||
    !isValidDate(scenario.updated_at)
  ) {
    throw new TypeError("Cenário inválido.");
  }
  const result: Scenario = {
    id: scenario.id,
    kind: scenario.kind as ScenarioKind,
    name: scenario.name,
    payable_id: scenario.payable_id as string | null,
    created_at: scenario.created_at,
    updated_at: scenario.updated_at,
  };
  if (scenario.is_active !== undefined) result.is_active = scenario.is_active as boolean;
  if (scenario.is_accounting_source !== undefined) {
    result.is_accounting_source = scenario.is_accounting_source as boolean;
  }
  return result;
}

export function parseScenario(value: unknown): Scenario {
  const hasActivation = isRecord(value) && "is_active" in value;
  const record = requiredRecord(value, hasActivation ? scenarioKeys : legacyScenarioKeys, "Cenário inválido.");
  return scenarioFieldsFrom(record);
}

export function parseScenarioList(value: unknown): Scenario[] {
  if (!Array.isArray(value)) {
    throw new TypeError("Lista de cenários inválida.");
  }
  return value.map(parseScenario);
}

const realizationKeys = ["id", "payable_link_id", "allocated_amount", "created_at"] as const;

function parseRealization(value: unknown): Realization {
  const item = requiredRecord(value, realizationKeys, "Alocação inválida.");
  if (
    typeof item.id !== "string" ||
    !isUuid(item.id) ||
    !isNullableUuid(item.payable_link_id) ||
    !isValidDate(item.created_at)
  ) {
    throw new TypeError("Alocação inválida.");
  }
  return {
    id: item.id,
    payable_link_id: item.payable_link_id as string | null,
    allocated_amount: decimal(item.allocated_amount),
    created_at: item.created_at,
  };
}

function parseRealizationList(value: unknown): Realization[] {
  if (!Array.isArray(value)) {
    throw new TypeError("Lista de alocações inválida.");
  }
  return value.map(parseRealization);
}

const scenarioTransactionKeys = [
  "id",
  "scenario_id",
  "description",
  "amount",
  "projected_at",
  "category",
  "status",
  "realizations",
] as const;

export function parseScenarioTransaction(value: unknown): ScenarioTransaction {
  const item = requiredRecord(value, scenarioTransactionKeys, "Parcela inválida.");
  if (
    typeof item.id !== "string" ||
    !isUuid(item.id) ||
    typeof item.scenario_id !== "string" ||
    !isUuid(item.scenario_id) ||
    typeof item.description !== "string" ||
    item.description === "" ||
    typeof item.projected_at !== "string" ||
    !dateOnlyPattern.test(item.projected_at) ||
    !isNullableString(item.category) ||
    !scenarioTransactionStatuses.includes(item.status as ScenarioTransactionStatus)
  ) {
    throw new TypeError("Parcela inválida.");
  }
  return {
    id: item.id,
    scenario_id: item.scenario_id,
    description: item.description,
    amount: decimal(item.amount),
    projected_at: item.projected_at,
    category: item.category,
    status: item.status as ScenarioTransactionStatus,
    realizations: parseRealizationList(item.realizations),
  };
}

export function parseScenarioTransactionList(value: unknown): ScenarioTransaction[] {
  if (!Array.isArray(value)) {
    throw new TypeError("Lista de parcelas inválida.");
  }
  return value.map(parseScenarioTransaction);
}

export function parseScenarioDetail(value: unknown): ScenarioDetail {
  const keys = isRecord(value) && "is_active" in value
    ? [...scenarioKeys, "transactions", "accumulated_deviation"]
    : [...legacyScenarioKeys, "transactions", "accumulated_deviation"];
  const detail = requiredRecord(
    value,
    keys,
    "Detalhe de cenário inválido.",
  );
  if (!Array.isArray(detail.transactions)) {
    throw new TypeError("Detalhe de cenário inválido.");
  }
  return {
    ...scenarioFieldsFrom(detail),
    transactions: detail.transactions.map(parseScenarioTransaction),
    accumulated_deviation: decimal(detail.accumulated_deviation),
  };
}
