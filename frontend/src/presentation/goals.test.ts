import { describe, expect, it } from "vitest";

import type { Goal } from "../api/contracts";
import { goalPercent, sortGoals } from "./goals";

describe("goalPercent", () => {
  it.each([
    ["0", 0],
    ["0.452105", 45],
    ["0.455", 46],
    ["0.995", 99],
    ["0.9999999999999999", 99],
    ["1", 100],
    ["1.2345", 123],
  ])("reads the ratio %s as %i%%", (progress, percent) => {
    expect(goalPercent(progress)).toBe(percent);
  });

  it("never reports a negative percent", () => {
    expect(goalPercent("-0.2")).toBe(0);
  });
});

describe("sortGoals", () => {
  const goal = (name: string, target_amount: string, progress: string) =>
    ({ name, target_amount, progress }) as Goal;

  it("puts open goals first, each group from the smallest target to the largest", () => {
    const sorted = sortGoals([
      goal("Reserva", "30000", "1"),
      goal("Casa", "500000", "0.1"),
      goal("Viagem", "10000", "1.2"),
      goal("Cem mil", "100000", "0.03"),
    ]);
    expect(sorted.map((g) => g.name)).toEqual(["Cem mil", "Casa", "Viagem", "Reserva"]);
  });

  it("does not mutate its input and breaks ties by name", () => {
    const input = [goal("B", "10", "0.5"), goal("A", "10", "0.5")];
    expect(sortGoals(input).map((g) => g.name)).toEqual(["A", "B"]);
    expect(input.map((g) => g.name)).toEqual(["B", "A"]);
  });
});
