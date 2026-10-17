import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import * as automationRulesApi from "../api/automationRules";
import type { AutomationRule, AutomationRuleWrite } from "../api/contracts";
import { useAutomationRules } from "./useAutomationRules";

vi.mock("../api/automationRules");

const ruleId = "33333333-3333-4333-8333-333333333333";

const rule: AutomationRule = {
  id: ruleId,
  name: "Ignorar taxas",
  is_active: true,
  logic_operator: "or",
  conditions: [{ field: "description", operator: "contains", value: "taxa" }],
  actions: [{ type: "ignore", scenario_id: null, category_id: null }],
  created_at: "2026-07-30T12:00:00Z",
  updated_at: "2026-07-30T12:00:00Z",
};

const write: AutomationRuleWrite = {
  name: rule.name,
  is_active: rule.is_active,
  logic_operator: rule.logic_operator,
  conditions: rule.conditions,
  actions: rule.actions,
  apply_retroactively: false,
};

function setup() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  return { client, wrapper };
}

describe("useAutomationRules", () => {
  beforeEach(() => {
    vi.mocked(automationRulesApi.listAutomationRuleConditionOptions).mockResolvedValue({
      accounts: ["ultraviolet-black"],
      cards: ["1139"],
    });
  });

  it("lists rules from the backend", async () => {
    vi.mocked(automationRulesApi.listAutomationRules).mockResolvedValue([rule]);
    const { wrapper } = setup();
    const { result } = renderHook(useAutomationRules, { wrapper });
    await waitFor(() => expect(result.current.rules).toEqual([rule]));
    await waitFor(() =>
      expect(result.current.conditionOptions.accounts).toEqual(["ultraviolet-black"]),
    );
  });

  it("invalidates only the rules list when creating without retroactive apply", async () => {
    vi.mocked(automationRulesApi.listAutomationRules).mockResolvedValue([]);
    vi.mocked(automationRulesApi.createAutomationRule).mockResolvedValue({
      rule,
      retroactive_apply: null,
    });
    const { client, wrapper } = setup();
    const invalidateSpy = vi.spyOn(client, "invalidateQueries");
    const { result } = renderHook(useAutomationRules, { wrapper });
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    await act(() => result.current.createRule(write));

    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["automationRules"] });
    expect(invalidateSpy).not.toHaveBeenCalledWith({ queryKey: ["transactions"] });
  });

  it("also invalidates transactions when a retroactive apply happened", async () => {
    vi.mocked(automationRulesApi.listAutomationRules).mockResolvedValue([]);
    vi.mocked(automationRulesApi.createAutomationRule).mockResolvedValue({
      rule,
      retroactive_apply: { matched: 2, ignored: 1, categorized: 0 },
    });
    const { client, wrapper } = setup();
    const invalidateSpy = vi.spyOn(client, "invalidateQueries");
    const { result } = renderHook(useAutomationRules, { wrapper });
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    await act(() => result.current.createRule(write));

    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["automationRules"] });
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["transactions"] });
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["transaction-transfer"] });
  });

  it("updates a rule and invalidates the list", async () => {
    vi.mocked(automationRulesApi.listAutomationRules).mockResolvedValue([rule]);
    vi.mocked(automationRulesApi.updateAutomationRule).mockResolvedValue({
      rule: { ...rule, name: "Renomeada" },
      retroactive_apply: null,
    });
    const { wrapper } = setup();
    const { result } = renderHook(useAutomationRules, { wrapper });
    await waitFor(() => expect(result.current.rules).toEqual([rule]));

    await act(() => result.current.updateRule({ ruleId, write }));

    expect(automationRulesApi.updateAutomationRule).toHaveBeenCalledWith(ruleId, write);
  });

  it("toggles active state and deletes a rule", async () => {
    vi.mocked(automationRulesApi.listAutomationRules).mockResolvedValue([rule]);
    vi.mocked(automationRulesApi.setAutomationRuleActive).mockResolvedValue({
      ...rule,
      is_active: false,
    });
    vi.mocked(automationRulesApi.deleteAutomationRule).mockResolvedValue(undefined);
    const { wrapper } = setup();
    const { result } = renderHook(useAutomationRules, { wrapper });
    await waitFor(() => expect(result.current.rules).toEqual([rule]));

    await act(() => result.current.toggleRule({ ruleId, isActive: false }));
    expect(automationRulesApi.setAutomationRuleActive).toHaveBeenCalledWith(ruleId, false);

    await act(() => result.current.deleteRule(ruleId));
    expect(automationRulesApi.deleteAutomationRule).toHaveBeenCalledWith(ruleId);
  });
});
