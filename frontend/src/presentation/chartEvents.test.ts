import { describe, expect, it } from "vitest";

import type { TimelineEntry } from "../api/contracts";
import { selectChartEvents } from "./chartEvents";

const reference = "2026-10-22";

const points = Array.from({ length: 12 }, (_, index) => ({
  date: `2026-10-${String(20 + index).padStart(2, "0")}`,
  balance: String(1000 + index),
}));

function entry(overrides: Partial<TimelineEntry>): TimelineEntry {
  return {
    date: "2026-10-25",
    description: "Compra",
    amount: "-100.00",
    reportable_amount: "-100.00",
    category_id: null,
    category_name: "Sem categoria",
    tier: "confirmado",
    source: "real",
    source_ref_id: "ref",
    scenario_id: null,
    ...overrides,
  };
}

describe("selectChartEvents", () => {
  it("sums the card purchases due the same day into one bill", () => {
    const events = selectChartEvents(
      [
        entry({ amount: "-1500.10" }),
        entry({ amount: "-639.90", description: "Outra compra" }),
        entry({ amount: "-0.01", date: "2026-10-26" }),
      ],
      points,
      reference,
      3,
    );

    expect(events[0]).toMatchObject({ kind: "bill", date: "2026-10-25", label: "Fatura 25/10", amount: "-2140.00", balance: "1005" });
  });

  it("names installments and recurrences, keeping recurrences apart", () => {
    const events = selectChartEvents(
      [
        entry({ source: "plano_pagamento", tier: "confirmado", date: "2026-10-28", amount: "-800.00" }),
        entry({ source: "recorrente", tier: "projetado", date: "2026-10-30", description: "Salário", amount: "9000.00" }),
        entry({ source: "recorrente", tier: "projetado", date: "2026-10-30", description: "Aluguel", amount: "-2000.00" }),
      ],
      points,
      reference,
      3,
    );

    // One marker per day: on the 30th the salary (the bigger) wins over the rent.
    expect(events.map((event) => event.label)).toEqual(["Parcela 28/10", "Salário 30/10"]);
  });

  it("keeps the biggest events by exact amount, in date order", () => {
    const events = selectChartEvents(
      [
        entry({ source: "recorrente", tier: "projetado", date: "2026-10-23", description: "A", amount: "-10.00" }),
        entry({ source: "recorrente", tier: "projetado", date: "2026-10-24", description: "B", amount: "-10.01" }),
        entry({ source: "recorrente", tier: "projetado", date: "2026-10-27", description: "C", amount: "10.02" }),
        entry({ source: "recorrente", tier: "projetado", date: "2026-10-29", description: "D", amount: "9999999999999999.01" }),
        entry({ source: "recorrente", tier: "projetado", date: "2026-10-30", description: "E", amount: "9999999999999999.02" }),
      ],
      points,
      reference,
      3,
    );

    expect(events.map((event) => event.label)).toEqual(["C 27/10", "D 29/10", "E 30/10"]);
  });

  it("ignores the past, today, other tiers, other sources and zero nets", () => {
    const events = selectChartEvents(
      [
        entry({ date: "2026-10-21" }),
        entry({ date: "2026-10-22" }),
        entry({ tier: "realizado", date: "2026-10-25" }),
        entry({ tier: "hipotetico", source: "cenario", date: "2026-10-25" }),
        entry({ source: "investment", tier: "projetado", date: "2026-10-26" }),
        entry({ amount: "-50.00", date: "2026-10-27" }),
        entry({ amount: "50.00", date: "2026-10-27" }),
      ],
      points,
      reference,
      3,
    );

    expect(events).toEqual([]);
  });

  it("skips an event whose day has no point on the curve", () => {
    expect(selectChartEvents([entry({ date: "2026-12-25" })], points, reference, 3)).toEqual([]);
  });

  it("shortens a long recurrence name and falls back when it is blank", () => {
    const events = selectChartEvents(
      [
        entry({ source: "recorrente", tier: "projetado", date: "2026-10-25", description: "Mensalidade da faculdade", amount: "-900.00" }),
        entry({ source: "recorrente", tier: "projetado", date: "2026-10-26", description: "  ", amount: "-800.00" }),
      ],
      points,
      reference,
      3,
    );

    expect(events.map((event) => event.label)).toEqual(["Mensalidade d… 25/10", "Recorrência 26/10"]);
  });

  it("leaves out the days it is told to skip and fills the slot with the next event", () => {
    const events = selectChartEvents(
      [
        entry({ source: "recorrente", tier: "projetado", date: "2026-10-25", description: "Aluguel", amount: "-6000.00" }),
        entry({ source: "recorrente", tier: "projetado", date: "2026-10-26", description: "Internet", amount: "-100.00" }),
      ],
      points,
      reference,
      1,
      ["2026-10-25"],
    );

    expect(events.map((event) => event.label)).toEqual(["Internet 26/10"]);
  });

  it("returns nothing when asked for no events", () => {
    expect(selectChartEvents([entry({})], points, reference, 0)).toEqual([]);
  });
});
