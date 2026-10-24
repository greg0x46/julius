import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { App } from "../ui";
import { afterEach, describe, expect, it, vi } from "vitest";

import { RecordMenu } from "./RecordMenu";
import * as compactScreen from "./useCompactScreen";
import { useConfirm } from "./useConfirm";

afterEach(() => vi.restoreAllMocks());

const useCompact = (compact: boolean) =>
  vi.spyOn(compactScreen, "useCompactScreen").mockReturnValue(compact);

describe("RecordMenu", () => {
  it("is a dropdown on a wide screen and runs the chosen item", async () => {
    useCompact(false);
    const onEdit = vi.fn();
    render(
      <RecordMenu
        label="Ações de Conta"
        items={[
          { key: "edit", label: "Editar", onClick: onEdit },
          { key: "delete", label: "Excluir", danger: true },
        ]}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Ações de Conta" }));
    await userEvent.click(await screen.findByRole("menuitem", { name: "Editar" }));
    expect(onEdit).toHaveBeenCalledTimes(1);
  });

  it("is a bottom sheet of full-width rows on a phone, red for the destructive one", async () => {
    useCompact(true);
    const onDelete = vi.fn();
    render(
      <RecordMenu
        label="Ações de Conta"
        items={[
          { key: "edit", label: "Editar" },
          { key: "delete", label: "Excluir", danger: true, onClick: onDelete },
        ]}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Ações de Conta" }));
    const sheet = await screen.findByRole("dialog");
    expect(within(sheet).getByRole("heading", { name: "Ações de Conta" })).toBeVisible();
    const remove = within(sheet).getByRole("button", { name: "Excluir" });
    expect(remove).toHaveClass("record-menu-row-danger");
    await userEvent.click(remove);
    expect(onDelete).toHaveBeenCalledTimes(1);
  });

  it("lists a submenu's entries under its label on a phone", async () => {
    useCompact(true);
    const onMove = vi.fn();
    render(
      <RecordMenu
        label="Ações"
        items={[{ key: "move", label: "Mover para", children: [{ key: "g1", label: "Reserva", onClick: onMove }] }]}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Ações" }));
    await userEvent.click(await screen.findByRole("button", { name: "Reserva" }));
    expect(onMove).toHaveBeenCalledTimes(1);
  });

  it("renders nothing without items", () => {
    useCompact(false);
    const { container } = render(<RecordMenu label="Ações" items={[]} />);
    expect(container).toBeEmptyDOMElement();
  });
});

function ConfirmHarness({ onConfirm }: { onConfirm: () => void }) {
  const confirm = useConfirm();
  return (
    <button
      type="button"
      onClick={() => confirm({ title: "Excluir conta", description: "Sem volta.", onConfirm })}
    >
      Abrir
    </button>
  );
}

describe("useConfirm", () => {
  it("asks in a modal with the action verb and Cancelar, and confirms", async () => {
    const onConfirm = vi.fn();
    render(
      <App>
        <ConfirmHarness onConfirm={onConfirm} />
      </App>,
    );
    await userEvent.click(screen.getByRole("button", { name: "Abrir" }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("Sem volta.")).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Cancelar" })).toBeInTheDocument();
    await userEvent.click(within(dialog).getByRole("button", { name: "Excluir" }));
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it("does nothing when the user cancels", async () => {
    const onConfirm = vi.fn();
    render(
      <App>
        <ConfirmHarness onConfirm={onConfirm} />
      </App>,
    );
    await userEvent.click(screen.getByRole("button", { name: "Abrir" }));
    await userEvent.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Cancelar" }));
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("falls back to the browser confirm outside a feedback provider", async () => {
    const onConfirm = vi.fn();
    vi.spyOn(window, "confirm").mockReturnValue(true);
    render(<ConfirmHarness onConfirm={onConfirm} />);
    await userEvent.click(screen.getByRole("button", { name: "Abrir" }));
    expect(window.confirm).toHaveBeenCalled();
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });
});
