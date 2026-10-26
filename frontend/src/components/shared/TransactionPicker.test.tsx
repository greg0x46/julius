import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as compactScreen from "./useCompactScreen";
import { TransactionPicker } from "./TransactionPicker";

const rows = [{ id: "t1", description: "QA mercado", amount: "10.00", direction: "outflow" as const, date: "2026-01-02" }];

afterEach(() => vi.restoreAllMocks());

describe("TransactionPicker compact", () => {
  it("offers a clear control when allowClear and a row is chosen", () => {
    vi.spyOn(compactScreen, "useCompactScreen").mockReturnValue(true);
    const onSelect = vi.fn();
    render(<TransactionPicker id="p" label="Taxa" value="t1" transactions={rows} allowClear onSelect={onSelect} />);
    fireEvent.click(screen.getByRole("button", { name: "Limpar Taxa" }));
    expect(onSelect).toHaveBeenCalledWith(null, null);
  });

  it("has no clear control without allowClear", () => {
    vi.spyOn(compactScreen, "useCompactScreen").mockReturnValue(true);
    render(<TransactionPicker id="p" label="Taxa" value="t1" transactions={rows} onSelect={vi.fn()} />);
    expect(screen.queryByRole("button", { name: "Limpar Taxa" })).toBeNull();
  });
});
