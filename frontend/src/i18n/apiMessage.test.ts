import { afterEach, expect, it } from "vitest";
import { apiMessage } from "./apiMessage";
import { languageStorageKey } from "./context";

afterEach(() => localStorage.clear());

it("builds API error text in the selected language", () => {
  expect(apiMessage("apiCommunication")).toBe("Não foi possível comunicar com o servidor.");
  localStorage.setItem(languageStorageKey, "en");
  expect(apiMessage("apiCommunication")).toBe("Could not communicate with the server.");
});
