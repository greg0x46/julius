import { act, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ConfigurableFilters, type FilterConfig } from "./ConfigurableFilters";

type Values = { description: string | null };
const config: FilterConfig<Values>[] = [
  { key: "description", label: "Descrição", type: "text", placement: "main", debounceMs: 300 },
];

function Harness() {
  const [values, setValues] = useState<Values>({ description: null });
  return (
    <ConfigurableFilters
      label="Filtros"
      values={values}
      emptyValues={{ description: null }}
      config={config}
      onApply={(next) => setValues({ description: next.description?.trim() || null })}
      onClear={() => setValues({ description: null })}
    />
  );
}

afterEach(() => vi.useRealTimers());

describe("ConfigurableFilters search", () => {
  it("keeps a trailing space typed while the trimmed value is applied", () => {
    vi.useFakeTimers();
    render(<Harness />);
    const input = screen.getByLabelText("Descrição") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "pão " } });
    act(() => void vi.advanceTimersByTime(300));
    expect(input.value).toBe("pão ");
  });
});

type Wide = { description: string | null; kind: string | null; accounts: string[] };
const wideConfig: FilterConfig<Wide>[] = [
  { key: "description", label: "Descrição", type: "text", placement: "main", debounceMs: 300 },
  { key: "accounts", label: "Conta", type: "multiselect", placement: "main", placeholder: "Todas", options: [{ value: "a", label: "Conta A" }] },
];

function WideHarness({ initial = { description: null, kind: null, accounts: [] } }: { initial?: Wide }) {
  const [values, setValues] = useState<Wide>(initial);
  const empty = { description: null, kind: null, accounts: [] };
  return (
    <>
      <button onClick={() => setValues({ ...values, kind: "inflow" })}>Entradas</button>
      <output data-testid="applied">{JSON.stringify(values)}</output>
      <ConfigurableFilters
        label="Filtros"
        values={values}
        emptyValues={empty}
        config={wideConfig}
        onApply={(next) => setValues({ ...next, description: next.description?.trim() || null })}
        onClear={() => setValues(empty)}
      />
    </>
  );
}

describe("ConfigurableFilters shortcuts and unlisted selections", () => {
  it("keeps pending search text when an outside shortcut applies", () => {
    vi.useFakeTimers();
    render(<WideHarness />);
    fireEvent.change(screen.getByLabelText("Descrição"), { target: { value: "pão" } });
    fireEvent.click(screen.getByText("Entradas"));
    act(() => void vi.advanceTimersByTime(300));
    expect(screen.getByTestId("applied").textContent).toContain('"description":"pão"');
    expect(screen.getByTestId("applied").textContent).toContain('"kind":"inflow"');
    expect((screen.getByLabelText("Descrição") as HTMLInputElement).value).toBe("pão");
  });

  it("keeps a pending search clear when an outside shortcut applies", () => {
    vi.useFakeTimers();
    render(<WideHarness initial={{ description: "pão", kind: null, accounts: [] }} />);
    const input = screen.getByLabelText("Descrição") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "" } });
    fireEvent.click(screen.getByText("Entradas"));
    act(() => void vi.advanceTimersByTime(300));
    expect(screen.getByTestId("applied").textContent).toContain('"description":null');
    expect(screen.getByTestId("applied").textContent).toContain('"kind":"inflow"');
    expect(input.value).toBe("");
  });

  it("removing the search chip discards the pending search edit", () => {
    vi.useFakeTimers();
    render(<WideHarness initial={{ description: "pão", kind: null, accounts: [] }} />);
    const input = screen.getByLabelText("Descrição") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "pãos" } });
    fireEvent.click(screen.getByRole("button", { name: "Remover filtro" }));
    act(() => void vi.advanceTimersByTime(300));
    expect(screen.getByTestId("applied").textContent).toContain('"description":null');
    expect(input.value).toBe("");
  });

  it("clearing filters drops search text that was still debouncing", () => {
    vi.useFakeTimers();
    render(<WideHarness initial={{ description: "pão", kind: null, accounts: [] }} />);
    const input = screen.getByLabelText("Descrição") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "pãos" } });
    fireEvent.click(screen.getAllByRole("button", { name: "Limpar filtros" })[0]);
    act(() => void vi.advanceTimersByTime(300));
    expect(screen.getByTestId("applied").textContent).toContain('"description":null');
    expect(input.value).toBe("");
  });

  it("still shows an applied selection the options no longer list", () => {
    render(<WideHarness initial={{ description: null, kind: null, accounts: ["gone"] }} />);
    expect(screen.getByText("Seleção indisponível")).toBeInTheDocument();
  });
});
