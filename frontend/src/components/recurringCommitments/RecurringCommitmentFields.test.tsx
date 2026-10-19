import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import * as accountsApi from "../../api/accounts";
import { QueryTestProvider } from "../../test/QueryTestProvider";
import { RecurringCommitmentFields } from "./RecurringCommitmentForm";

vi.mock("../../api/accounts");

function renderFields(submitting: boolean) {
  vi.mocked(accountsApi.listAccounts).mockResolvedValue([]);
  const { container } = render(
    <QueryTestProvider>
      <RecurringCommitmentFields
        formId="recurrence-form"
        commitment={null}
        categories={[]}
        submitError={null}
        submitting={submitting}
        onSubmit={vi.fn()}
      />
    </QueryTestProvider>,
  );
  return container.querySelector("form") as HTMLFormElement;
}

describe("RecurringCommitmentFields", () => {
  it("validates on submit (the empty draft is refused with a message)", () => {
    const form = renderFields(false);
    fireEvent.submit(form);
    expect(form.querySelector(".ant-alert")).not.toBeNull();
  });

  it("ignores a submit — Enter in a field — while a write is already in flight", () => {
    const form = renderFields(true);
    fireEvent.submit(form);
    expect(form.querySelector(".ant-alert")).toBeNull();
  });
  it.each(["1.005", "9007199254740993.123456789", "0.000000000000000001"])("saves an unchanged exact amount %s", (amount) => {
    vi.mocked(accountsApi.listAccounts).mockResolvedValue([]);
    const onSubmit = vi.fn();
    const commitment = {
      id: "11111111-1111-4111-8111-111111111111", name: "Synthetic precision", kind: "expense" as const,
      amount, category_id: "22222222-2222-4222-8222-222222222222", account_id: null,
      cadence: "monthly" as const, day_of_month: 5, month_of_year: null,
      start_date: "2026-01-01", end_date: null, is_active: true,
      created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
    };
    const { container } = render(<QueryTestProvider><RecurringCommitmentFields formId="precision-form" commitment={commitment} categories={[]} submitError={null} submitting={false} onSubmit={onSubmit} /></QueryTestProvider>);
    const input = screen.getByLabelText("Valor");
    fireEvent.focus(input);
    fireEvent.blur(input);
    fireEvent.submit(container.querySelector("form")!);
    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ amount }));
  });

  it("never sends a fractional day of the month", () => {
    vi.mocked(accountsApi.listAccounts).mockResolvedValue([]);
    const onSubmit = vi.fn();
    const commitment = {
      id: "11111111-1111-4111-8111-111111111111", name: "Synthetic day", kind: "expense" as const,
      amount: "10.00", category_id: "22222222-2222-4222-8222-222222222222", account_id: null,
      cadence: "monthly" as const, day_of_month: 5, month_of_year: null,
      start_date: "2026-01-01", end_date: null, is_active: true,
      created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
    };
    const { container } = render(<QueryTestProvider><RecurringCommitmentFields formId="day-form" commitment={commitment} categories={[]} submitError={null} submitting={false} onSubmit={onSubmit} /></QueryTestProvider>);
    const input = screen.getByLabelText("Dia do mês");
    fireEvent.change(input, { target: { value: "6.5" } });
    fireEvent.blur(input);
    fireEvent.submit(container.querySelector("form")!);
    expect(onSubmit).toHaveBeenCalledOnce();
    expect(Number.isInteger(onSubmit.mock.calls[0]![0].day_of_month)).toBe(true);
  });
});
