import { apiMessage } from "../i18n/apiMessage";
import { apiFetch } from "./transport";
import {
  parseAutomationRule,
  parseAutomationRuleConditionOptions,
  parseAutomationRuleList,
  parseAutomationRuleWriteResult,
  parseProblem,
  type AutomationRule,
  type AutomationRuleConditionOptions,
  type AutomationRuleWrite,
  type AutomationRuleWriteResult,
  type Problem,
} from "./contracts";
import { ApiError, isAbortError } from "./problems";

const defaultMessage = () => apiMessage("apiRuleSave");

async function send<T>(
  input: RequestInfo | URL,
  init: RequestInit,
  expectedStatus: number,
  parser: ((value: unknown) => T) | null,
): Promise<T> {
  let response: Response;
  try {
    response = await apiFetch(input, init);
  } catch (error) {
    if (isAbortError(error)) throw error;
    throw new ApiError("transport", defaultMessage());
  }

  const contentType = response.headers.get("content-type")?.split(";")[0].trim();

  if (response.status !== expectedStatus) {
    let problem: Problem | undefined;
    if (contentType === "application/problem+json") {
      try {
        problem = parseProblem(await response.json());
      } catch {
        problem = undefined;
      }
    }
    throw new ApiError(
      response.status === 404 ? "not_found" : "response",
      problem?.detail ?? problem?.title ?? defaultMessage(),
      problem,
    );
  }

  if (parser === null) {
    return undefined as T;
  }
  if (contentType !== "application/json") {
    throw new ApiError("response", defaultMessage());
  }
  try {
    return parser(await response.json());
  } catch {
    throw new ApiError("response", defaultMessage());
  }
}

export function listAutomationRules(signal?: AbortSignal): Promise<AutomationRule[]> {
  return send(
    "/api/automation-rules",
    { method: "GET", headers: { Accept: "application/json" }, signal },
    200,
    parseAutomationRuleList,
  );
}

export function listAutomationRuleConditionOptions(
  signal?: AbortSignal,
): Promise<AutomationRuleConditionOptions> {
  return send(
    "/api/automation-rules/condition-options",
    { method: "GET", headers: { Accept: "application/json" }, signal },
    200,
    parseAutomationRuleConditionOptions,
  );
}

export function createAutomationRule(
  write: AutomationRuleWrite,
): Promise<AutomationRuleWriteResult> {
  return send(
    "/api/automation-rules",
    {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(write),
    },
    201,
    parseAutomationRuleWriteResult,
  );
}

export function updateAutomationRule(
  ruleId: string,
  write: AutomationRuleWrite,
): Promise<AutomationRuleWriteResult> {
  return send(
    `/api/automation-rules/${encodeURIComponent(ruleId)}`,
    {
      method: "PUT",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(write),
    },
    200,
    parseAutomationRuleWriteResult,
  );
}

export function setAutomationRuleActive(
  ruleId: string,
  isActive: boolean,
): Promise<AutomationRule> {
  return send(
    `/api/automation-rules/${encodeURIComponent(ruleId)}`,
    {
      method: "PATCH",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ is_active: isActive }),
    },
    200,
    parseAutomationRule,
  );
}

export function deleteAutomationRule(ruleId: string): Promise<void> {
  return send(
    `/api/automation-rules/${encodeURIComponent(ruleId)}`,
    { method: "DELETE" },
    204,
    null,
  );
}
