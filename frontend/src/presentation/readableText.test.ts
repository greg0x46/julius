import { expect, it } from "vitest";

import { readableTextOn } from "./readableText";

it("picks the higher-contrast text colour", () => {
  expect(readableTextOn("#eda100")).toBe("#000000");
  expect(readableTextOn("#495057")).toBe("#ffffff");
  expect(readableTextOn("#1baf7a")).toBe("#000000");
  expect(readableTextOn("not-a-colour")).toBe("#ffffff");
});
