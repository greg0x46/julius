import { calendar } from "../../presentation/calendar";
import { describe, expect, it } from "vitest";

import { draftFromGoal, goalWriteFrom, type GoalDraft } from "./goalDraft";

const valid: GoalDraft = {
  name: "  Cem mil  ",
  metric: "invested_value",
  targetAmount: "100000.00",
  targetDate: calendar("2027-12-31"),
  notes: "  ",
};

describe("goalWriteFrom", () => {
  it("trims the text, drops blank notes and formats the date", () => {
    expect(goalWriteFrom(valid)).toEqual({
      write: {
        name: "Cem mil",
        metric: "invested_value",
        target_amount: "100000.00",
        target_date: "2027-12-31",
        notes: null,
      },
    });
  });

  it("requires a name", () => {
    expect(goalWriteFrom({ ...valid, name: " " })).toEqual({ error: "Informe o nome da meta." });
  });

  it.each([null, "0.00", "0"])("requires a target above zero (%s)", (targetAmount) => {
    expect(goalWriteFrom({ ...valid, targetAmount })).toEqual({ error: "O valor da meta precisa ser maior que zero." });
  });

  it("allows no target date", () => {
    const result = goalWriteFrom({ ...valid, targetDate: null });
    expect("write" in result && result.write.target_date).toBeNull();
  });
});

describe("draftFromGoal", () => {
  it("starts blank on the only metric", () => {
    expect(draftFromGoal(null)).toMatchObject({ name: "", metric: "invested_value", targetAmount: null });
  });
});
