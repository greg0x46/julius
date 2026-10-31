import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import * as preferencesApi from "../api/preferences";
import { useTimezone } from "../hooks/useTimezone";
import { browserTimezone } from "../presentation/timezone";
import { QueryTestProvider } from "../test/QueryTestProvider";
import { TimezoneProvider } from "./TimezoneProvider";

vi.mock("../api/preferences");

const preferences = (timezone: string) => ({
  transactions_period_basis: "occurred_at" as const,
  reference_currency: "BRL",
  timezone,
});

function Zone() {
  return <p data-testid="zone">{useTimezone()}</p>;
}

const renderApp = () =>
  render(
    <QueryTestProvider>
      <TimezoneProvider>
        <Zone />
      </TimezoneProvider>
    </QueryTestProvider>,
  );

describe("TimezoneProvider", () => {
  it("holds the first paint until the preferences arrive, then serves the configured zone", async () => {
    vi.mocked(preferencesApi.getPreferences).mockResolvedValue(preferences("Asia/Tokyo"));
    renderApp();
    expect(screen.getByRole("status")).toBeInTheDocument();
    expect(screen.queryByTestId("zone")).not.toBeInTheDocument();
    expect(await screen.findByTestId("zone")).toHaveTextContent("Asia/Tokyo");
  });

  it("does not block the app when the preferences fail: the browser zone stands in", async () => {
    vi.mocked(preferencesApi.getPreferences).mockRejectedValue(new Error("offline"));
    renderApp();
    expect(await screen.findByTestId("zone")).toHaveTextContent(browserTimezone());
  });

  it("ignores a zone this browser does not know", async () => {
    vi.mocked(preferencesApi.getPreferences).mockResolvedValue(preferences("Nowhere/Land"));
    renderApp();
    expect(await screen.findByTestId("zone")).toHaveTextContent(browserTimezone());
  });
});

describe("useTimezone", () => {
  it("falls back to the browser zone outside the provider", () => {
    render(<Zone />);
    expect(screen.getByTestId("zone")).toHaveTextContent(browserTimezone());
  });
});
