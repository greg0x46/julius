import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { expect, it, vi } from "vitest";
import { Button } from "./controls";
import { App, Input } from "./compounds";
import { Select } from "./Select";
import { Drawer } from "./overlays";
import { useConfirm } from "../shared/useConfirm";
import Table from "./Table";

it("clears a controlled search and reports the change", async () => {
  function Search() {
    const [value, setValue] = useState("Tesouro");
    return (
      <Input
        aria-label="Busca"
        value={value}
        onChange={(event) => setValue(event.target.value)}
        allowClear
      />
    );
  }
  render(<Search />);
  await userEvent.click(screen.getByRole("button", { name: "Limpar campo" }));
  expect(screen.getByRole("textbox")).toHaveValue("");
  expect(screen.getByRole("textbox")).toHaveFocus();
});
it("selects by keyboard, skips disabled options, and closes without submitting the form", async () => {
  const change = vi.fn();
  const submit = vi.fn((event) => event.preventDefault());
  render(
    <form onSubmit={submit}>
      <Select
        aria-label="Conta"
        options={[
          { value: "blocked", label: "Arquivada", disabled: true },
          { value: "a", label: "Conta A" },
          { value: "b", label: "Conta B" },
        ]}
        onChange={change}
      />
    </form>,
  );
  const select = screen.getByRole("combobox");
  await userEvent.click(select);
  await userEvent.keyboard("{ArrowDown}{Enter}");
  expect(change).toHaveBeenCalledWith(
    "b",
    expect.objectContaining({ label: "Conta B" }),
  );
  expect(submit).not.toHaveBeenCalled();
  expect(select).toHaveAttribute("aria-expanded", "false");
  expect(select).toHaveFocus();
});
it("keeps a multi-selection open and filters searchable options", async () => {
  function Choices() {
    const [value, setValue] = useState<string[]>([]);
    return (
      <Select
        aria-label="Categorias"
        mode="multiple"
        showSearch
        value={value}
        onChange={setValue}
        options={[
          { value: "rent", label: "Aluguel" },
          { value: "food", label: "Mercado" },
        ]}
      />
    );
  }
  render(<Choices />);
  await userEvent.type(screen.getByRole("combobox"), "alug");
  expect(screen.queryByRole("option", { name: "Mercado" })).toBeNull();
  await userEvent.click(screen.getByRole("option", { name: "Aluguel" }));
  expect(screen.getByRole("option", { name: "Aluguel" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await userEvent.keyboard("{Escape}");
  expect(screen.queryByRole("listbox")).toBeNull();
});
it("restores scrolling after nested dialogs are removed together", () => {
  const before = document.body.style.overflow;
  const { unmount } = render(
    <Drawer open title="Conta">
      <Drawer open title="Confirmar">
        <Button>Continuar</Button>
      </Drawer>
    </Drawer>,
  );
  expect(document.body.style.overflow).toBe("hidden");
  unmount();
  expect(document.body.style.overflow).toBe(before);
});
it("keeps a failed asynchronous confirmation open and allows retry", async () => {
  let resolve!: () => void;
  const confirmAction = vi
    .fn()
    .mockRejectedValueOnce(new Error("unavailable"))
    .mockImplementationOnce(
      () =>
        new Promise<void>((done) => {
          resolve = done;
        }),
    );
  function Confirm() {
    const confirm = useConfirm();
    return (
      <Button
        onClick={() =>
          confirm({ title: "Excluir conta", onConfirm: confirmAction })
        }
      >
        Abrir confirmação
      </Button>
    );
  }
  render(
    <App>
      <Confirm />
    </App>,
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Abrir confirmação" }),
  );
  await userEvent.click(screen.getByRole("button", { name: "Excluir" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("Tente novamente");
  await userEvent.click(screen.getByRole("button", { name: "Excluir" }));
  expect(screen.getByRole("button", { name: "Cancelar" })).toBeDisabled();
  resolve();
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
});
it("filters, sorts, and expands table records with their actions intact", async () => {
  render(
    <Table
      aria-label="Ativos"
      rowKey="id"
      dataSource={[
        { id: "a", name: "Tesouro", kind: "fixed" },
        { id: "b", name: "Ação", kind: "equity" },
      ]}
      columns={[
        {
          title: "Nome",
          dataIndex: "name",
          sorter: (a, b) => a.name.localeCompare(b.name),
        },
        {
          title: "Tipo",
          dataIndex: "kind",
          filters: [{ value: "fixed", text: "Renda fixa" }],
          onFilter: (value, row) => row.kind === value,
        },
      ]}
      expandable={{ expandedRowRender: (row) => <p>Detalhes de {row.name}</p> }}
    />,
  );
  await userEvent.click(screen.getByRole("button", { name: "Nome" }));
  const rows = screen.getAllByRole("row");
  expect(rows[1]).toHaveTextContent("Ação");
  await userEvent.selectOptions(
    screen.getByRole("combobox", { name: "Filtrar por tipo" }),
    "fixed",
  );
  expect(screen.queryByText("Ação")).toBeNull();
  await userEvent.click(
    screen.getByRole("button", { name: "Expandir detalhes" }),
  );
  expect(screen.getByText("Detalhes de Tesouro")).toBeVisible();
});
it("does not dismiss a panel when Escape closes its searchable selector", () => {
  const close = vi.fn();
  render(
    <Drawer open title="Conta" onClose={close}>
      <Select
        showSearch
        aria-label="Moeda"
        options={[{ value: "BRL", label: "Real" }]}
      />
    </Drawer>,
  );
  fireEvent.click(screen.getByRole("combobox"));
  fireEvent.keyDown(screen.getByRole("combobox"), { key: "Escape" });
  expect(close).not.toHaveBeenCalled();
  expect(screen.queryByRole("listbox")).toBeNull();
});
