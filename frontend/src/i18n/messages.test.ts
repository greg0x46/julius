import { describe, expect, it } from "vitest";
import { messages, translate } from "./messages";

const catalogModules = import.meta.glob<Record<string, { "pt-BR": object; en: object }>>("./catalogs/*.ts", { eager: true });

describe("message catalogs", () => {
  it("interpolates user text literally in one pass", () => {
    expect(translate("en", "aboutGoalProjection", { name: "$& {explanation}", explanation: "Details" })).toBe("About the projection for $& {explanation}: Details");
  });
  it("keeps both languages side by side in every area catalog", () => {
    const files = Object.entries(catalogModules);
    expect(files.length).toBeGreaterThan(0);
    for (const [file, module] of files) {
      for (const catalog of Object.values(module)) {
        expect(Object.keys(catalog.en).sort(), file).toEqual(Object.keys(catalog["pt-BR"]).sort());
      }
    }
  });
  it("defines each key in a single area", () => {
    const seen = new Set<string>();
    for (const module of Object.values(catalogModules)) {
      for (const catalog of Object.values(module)) {
        for (const key of Object.keys(catalog["pt-BR"])) {
          expect(seen.has(key), key).toBe(false);
          seen.add(key);
        }
      }
    }
  });
  it("provides an English translation for every Portuguese message", () => {
    expect(Object.keys(messages.en).sort()).toEqual(Object.keys(messages["pt-BR"]).sort());
    for (const key of Object.keys(messages["pt-BR"]) as (keyof typeof messages["pt-BR"])[]) {
      expect(translate("en", key)).not.toBe("");
      const placeholders = (value: string) => [...value.matchAll(/\{[a-zA-Z]+\}/g)].map((match) => match[0]).sort();
      expect(placeholders(messages.en[key]), key).toEqual(placeholders(messages["pt-BR"][key]));
    }
  });
});
