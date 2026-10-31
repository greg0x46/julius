import { describe, expect, it } from "vitest";
import { messages, translate } from "./messages";

describe("message catalogs", () => {
  it("provides an English translation for every Portuguese message", () => {
    expect(Object.keys(messages.en).sort()).toEqual(Object.keys(messages["pt-BR"]).sort());
    for (const key of Object.keys(messages["pt-BR"]) as (keyof typeof messages["pt-BR"])[]) {
      expect(translate("en", key)).not.toBe("");
    }
  });
});
