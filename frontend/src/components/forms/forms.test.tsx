import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { App as FeedbackApp, Input } from "../ui";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as compactScreen from "../shared/useCompactScreen";
import { useFeedback } from "../shared/useFeedback";
import { Money } from "../shared/Money";
import { FormDrawer } from "./FormDrawer";
import { FormField } from "./FormField";
import { MoneyInput } from "./MoneyInput";
import { parseMoney } from "./moneyText";

afterEach(() => {
  vi.restoreAllMocks();
});

function Harness({ onSubmit, error }: { onSubmit: () => void; error?: string }) {
  return (
    <FormDrawer open title="Nova categoria" onClose={() => undefined} onSubmit={onSubmit} error={error}>
      <FormField label="Nome" htmlFor="name">
        <Input id="name" />
      </FormField>
    </FormDrawer>
  );
}

describe("FormDrawer", () => {
  it("submits with Enter inside a field and with the Salvar button", async () => {
    const onSubmit = vi.fn();
    render(<Harness onSubmit={onSubmit} />);

    await userEvent.type(screen.getByLabelText("Nome"), "{Enter}");
    expect(onSubmit).toHaveBeenCalledTimes(1);

    await userEvent.click(screen.getByRole("button", { name: "Salvar" }));
    expect(onSubmit).toHaveBeenCalledTimes(2);
  });

  it("shows the error above the fields and Cancelar on wide screens", () => {
    render(<Harness onSubmit={() => undefined} error="Não foi possível salvar." />);
    expect(screen.getByText("Não foi possível salvar.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Cancelar" })).toBeInTheDocument();
  });

  it("is full-screen on compact screens: close in the header, no Cancelar", () => {
    vi.spyOn(compactScreen, "useCompactScreen").mockReturnValue(true);
    render(<Harness onSubmit={() => undefined} />);
    expect(screen.getByRole("button", { name: "Fechar" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Cancelar" })).not.toBeInTheDocument();
  });
});

describe("FormField", () => {
  it("shows the error in place of the hint", () => {
    render(
      <FormField label="Valor" htmlFor="v" hint="Dica" error="Informe o valor.">
        <Input id="v" />
      </FormField>,
    );
    expect(screen.getByRole("alert")).toHaveTextContent("Informe o valor.");
    expect(screen.queryByText("Dica")).not.toBeInTheDocument();
  });
});

describe("MoneyInput", () => {
  it("asks phones for the decimal keypad and shows the R$ prefix", () => {
    render(<MoneyInput aria-label="Valor" />);
    expect(screen.getByLabelText("Valor")).toHaveAttribute("inputmode", "decimal");
    expect(screen.getByText("R$")).toBeInTheDocument();
  });

  it("groups thousands with a point and keeps the decimal comma while editing", async () => {
    const onChange = vi.fn();
    render(<MoneyInput aria-label="Valor" onChange={onChange} />);
    const input = screen.getByLabelText("Valor");

    await userEvent.type(input, "12850,5");
    expect(input).toHaveValue("12.850,5");
    expect(onChange).toHaveBeenLastCalledWith("12850.5");

    await userEvent.tab();
    expect(input).toHaveValue("12.850,50");
  });

  it("shows a controlled value grouped and with two decimals", () => {
    render(<MoneyInput aria-label="Valor" value="1234567.8" />);
    expect(screen.getByLabelText("Valor")).toHaveValue("1.234.567,80");
  });

  it("reads a backspaced group as pt-BR (12.85 is 1.285, not 12,85)", async () => {
    const onChange = vi.fn();
    render(<MoneyInput aria-label="Valor" onChange={onChange} />);
    const input = screen.getByLabelText("Valor");

    await userEvent.type(input, "12850{Backspace}");
    expect(input).toHaveValue("1.285");
    expect(onChange).toHaveBeenLastCalledWith("1285");
  });

  it("reads a pasted point-decimal figure as decimal, not as thousands (1234.56 is not 123456)", async () => {
    const onChange = vi.fn();
    render(<MoneyInput aria-label="Valor" onChange={onChange} />);
    const input = screen.getByLabelText("Valor");

    await userEvent.click(input);
    await userEvent.paste("1234.56");
    expect(onChange).toHaveBeenLastCalledWith("1234.56");
    await userEvent.tab();
    expect(input).toHaveValue("1.234,56");
  });

  it("reads a pasted pt-BR figure as thousands ('12.850' is 12850, '1.234,56' is 1234.56)", async () => {
    const onChange = vi.fn();
    render(<MoneyInput aria-label="Valor" onChange={onChange} />);
    const input = screen.getByLabelText("Valor");

    await userEvent.click(input);
    await userEvent.paste("12.850");
    expect(onChange).toHaveBeenLastCalledWith("12850");
    await userEvent.clear(input);
    await userEvent.paste("1.234,56");
    expect(onChange).toHaveBeenLastCalledWith("1234.56");
  });

  it("takes a point typed after the whole part as the decimal point", async () => {
    const onChange = vi.fn();
    render(<MoneyInput aria-label="Valor" onChange={onChange} />);
    const input = screen.getByLabelText("Valor");

    await userEvent.type(input, "1234.56");
    expect(input).toHaveValue("1.234,56");
    expect(onChange).toHaveBeenLastCalledWith("1234.56");
  });
  it("preserves large fractional amounts through typing and blur", async () => {
    const onChange = vi.fn();
    render(<MoneyInput aria-label="Valor" onChange={onChange} />);
    const input = screen.getByLabelText("Valor");
    await userEvent.click(input);
    await userEvent.paste("9.007.199.254.740.993,123456789");
    await userEvent.tab();
    expect(input).toHaveValue("9.007.199.254.740.993,123456789");
    expect(onChange).toHaveBeenLastCalledWith("9007199254740993.123456789");
  });

  it("retains saved fractional cents on focus and blur", async () => {
    const onChange = vi.fn();
    render(<MoneyInput aria-label="Valor" value="0.000000000000000001" min="0" onChange={onChange} />);
    const input = screen.getByLabelText("Valor");
    await userEvent.click(input);
    await userEvent.tab();
    expect(input).toHaveValue("0,000000000000000001");
    expect(onChange).not.toHaveBeenCalled();
  });

});

describe("parseMoney", () => {
  it.each([
    ["12.850", "12850"],
    ["1234.56", "1234.56"],
    ["1234.56789", "1234.56789"],
    ["1.234,56789", "1234.56789"],
    ["1.234,56", "1234.56"],
    ["1.234", "1234"],
    ["1.234.567", "1234567"],
    ["12.5", "12.5"],
    ["12,5", "12.5"],
    ["1234", "1234"],
    ["1234.", "1234."],
    ["1.234.", "1234."],
    ["-1234.56", "-1234.56"],
    ["R$ 1.234,56", "1234.56"],
  ])("%s -> %s", (text, expected) => {
    expect(parseMoney(text)).toBe(expected);
  });

  it("never turns a deleted character of the field's own grouping into a decimal point", () => {
    expect(parseMoney("12.85", "12.850")).toBe("1285");
    expect(parseMoney("12.85")).toBe("12.85");
  });

  it("continues a point typed after the grouped whole part into decimals", () => {
    expect(parseMoney("1.234.5", "1.234")).toBe("1234.5");
    expect(parseMoney("1.234.56", "1.234")).toBe("1234.56");
  });
});

describe("Money", () => {
  it("renders tone, sign and tabular class", () => {
    render(<Money value="-12.5" tone="balance" />);
    const node = screen.getByText(/^-R\$\s12,50$/);
    expect(node).toHaveClass("money", "money-negative");
  });

  it("keeps a negative result in the text colour, signed", () => {
    render(<Money value="-12.5" tone="result" />);
    expect(screen.getByText(/^-R\$\s12,50$/)).toHaveClass("money", "money-neutral");
  });
});

describe("useFeedback", () => {
  function Probe() {
    const feedback = useFeedback();
    return <button onClick={() => feedback.success("Salvo")}>go</button>;
  }

  it("shows a toast inside a feedback provider", async () => {
    render(
      <FeedbackApp>
        <Probe />
      </FeedbackApp>,
    );
    await userEvent.click(screen.getByRole("button", { name: "go" }));
    await waitFor(() => expect(screen.getByText("Salvo")).toBeInTheDocument());
  });

  it("does not crash without a feedback provider (component tests)", async () => {
    render(<Probe />);
    await userEvent.click(screen.getByRole("button", { name: "go" }));
    expect(screen.getByRole("button", { name: "go" })).toBeInTheDocument();
  });
});
