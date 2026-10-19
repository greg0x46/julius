import { afterEach, describe, expect, it, vi } from "vitest";

import { parseGoal, parseGoalList, type Goal } from "./contracts";
import { createGoal, deleteGoal, listGoals, updateGoal } from "./goals";
import { expireSession } from "./transport";

const goal: Goal = {
  id: "11111111-1111-4111-8111-111111111111",
  name: "Cem mil investidos",
  metric: "invested_value",
  target_amount: "100000.00",
  target_date: "2027-12-31",
  notes: null,
  current_value: "45210.5",
  progress: "0.452105",
  projection: null,
  monthly_contribution: null,
  currency_code: "BRL",
  created_at: "2026-10-01T12:00:00Z",
  updated_at: "2026-10-01T12:00:00Z",
};

const projection = {
  status: "on_track",
  months_remaining: 6,
  monthly_return: "0.01",
  return_from: "2026-01-01",
  return_partial: false,
  projected_value: "110000",
  projected_progress: "1.1",
  required_monthly_contribution: "0",
};

const write = {
  name: goal.name,
  metric: goal.metric,
  target_amount: goal.target_amount,
  target_date: goal.target_date,
  notes: goal.notes,
};

function respond(status: number, body: unknown, type = "application/json") {
  return vi.fn().mockResolvedValue(
    new Response(body === null ? null : JSON.stringify(body), { status, headers: { "content-type": type } }),
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
  expireSession();
});

describe("parseGoal", () => {
  it("parses exact monthly contributions and rejects invalid data", () => {
    const monthly = { month: "2026-10", target: "2000.01", net_contributed: "650", remaining: "1350.01", assumes_zero_return: true };
    expect(parseGoal({ ...goal, monthly_contribution: monthly }).monthly_contribution).toEqual(monthly);
    expect(() => parseGoal({ ...goal, monthly_contribution: { ...monthly, remaining: 1350.01 } })).toThrow(TypeError);
    expect(() => parseGoal({ ...goal, monthly_contribution: { ...monthly, month: "2026-13" } })).toThrow(TypeError);
  });
  it("keeps decimals as the exact strings the server sent", () => {
    expect(parseGoal(goal)).toEqual(goal);
    expect(parseGoalList({ items: [goal] })).toEqual([goal]);
  });

  it("parses a projection and keeps its decimals as sent", () => {
    const projected: Goal = {
      ...goal,
      projection: {
        status: "behind",
        months_remaining: 14,
        monthly_return: "0.0081234567",
        return_from: "2026-01-01",
        return_partial: false,
        projected_value: "52000.123456789",
        projected_progress: "0.52000123",
        required_monthly_contribution: "3100.5",
      },
    };
    expect(parseGoal(projected)).toEqual(projected);
    const unmeasured = { ...projected.projection!, monthly_return: null, return_from: null };
    expect(parseGoal({ ...projected, projection: unmeasured }).projection).toEqual(unmeasured);
  });

  it.each([
    ["a numeric amount", { target_amount: 100000 }],
    ["an unknown metric", { metric: "net_worth_forecast" }],
    ["a blank name", { name: "  " }],
    ["a malformed date", { target_date: "31/12/2027" }],
    ["a missing progress", { progress: undefined }],
    ["an unexpected field", { portfolio_id: null }],
    ["a missing projection", { projection: undefined }],
    ["an unknown projection status", { projection: { ...projection, status: "late" } }],
    ["a numeric contribution", { projection: { ...projection, required_monthly_contribution: 100 } }],
    ["an extra projection field", { projection: { ...projection, confidence: "high" } }],
    ["a zero horizon", { projection: { ...projection, months_remaining: 0 } }],
  ])("rejects %s", (_label, change) => {
    expect(() => parseGoal({ ...goal, ...change })).toThrow(TypeError);
  });

  it("rejects a list that is not the items envelope", () => {
    expect(() => parseGoalList([goal])).toThrow("Lista de metas inválida.");
    expect(() => parseGoalList({ items: goal })).toThrow("Lista de metas inválida.");
  });
});

describe("goals client", () => {
  it("lists goals from /api/goals", async () => {
    const fetchMock = respond(200, { items: [goal] });
    vi.stubGlobal("fetch", fetchMock);
    await expect(listGoals()).resolves.toEqual([goal]);
    expect(fetchMock.mock.calls[0][0]).toBe("/api/goals");
  });

  it("creates with POST and expects 201", async () => {
    const fetchMock = respond(201, goal);
    vi.stubGlobal("fetch", fetchMock);
    await expect(createGoal(write)).resolves.toEqual(goal);
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/api/goals");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body as string)).toEqual(write);
  });

  it("updates with PUT on the encoded id", async () => {
    const fetchMock = respond(200, goal);
    vi.stubGlobal("fetch", fetchMock);
    await updateGoal(goal.id, write);
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe(`/api/goals/${goal.id}`);
    expect(init.method).toBe("PUT");
  });

  it("deletes with DELETE and expects 204", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);
    await expect(deleteGoal(goal.id)).resolves.toBeUndefined();
    expect((fetchMock.mock.calls[0][1] as RequestInit).method).toBe("DELETE");
  });

  it("surfaces the problem detail and marks a missing goal as not found", async () => {
    vi.stubGlobal(
      "fetch",
      respond(
        404,
        { type: "about:blank", title: "Meta não encontrada", status: 404, detail: "A meta informada não existe." },
        "application/problem+json",
      ),
    );
    await expect(updateGoal(goal.id, write)).rejects.toMatchObject({
      kind: "not_found",
      message: "A meta informada não existe.",
    });
  });

  it("turns an unreadable success body into a response error", async () => {
    vi.stubGlobal("fetch", respond(200, { items: [{ ...goal, target_amount: 1 }] }));
    await expect(listGoals()).rejects.toMatchObject({ kind: "response" });
  });
});
