import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Drawer, Dropdown, Popover } from "./overlays";

describe("Drawer", () => {
  it("closes on a backdrop press but not on a drag that began inside", () => {
    const onClose = vi.fn();
    render(
      <Drawer open title="T" onClose={onClose}>
        <input aria-label="campo" />
      </Drawer>,
    );
    const dialog = screen.getByRole("dialog", { hidden: true });
    fireEvent.pointerDown(screen.getByLabelText("campo"));
    fireEvent.click(dialog, { clientX: -5, clientY: -5 });
    expect(onClose).not.toHaveBeenCalled();
    fireEvent.pointerDown(dialog);
    fireEvent.click(dialog, { clientX: -5, clientY: -5 });
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});

describe("Dropdown and Popover", () => {
  it("marks the trigger as a menu popup", () => {
    render(
      <Dropdown menu={{ items: [{ key: "a", label: "Editar" }] }}>
        <button type="button">Ações</button>
      </Dropdown>,
    );
    expect(screen.getByRole("button", { name: "Ações" })).toHaveAttribute("aria-haspopup", "menu");
  });
  it("returns focus to the trigger on Escape", async () => {
    render(
      <Popover content={<input aria-label="dentro" />}>
        <button type="button">Abrir</button>
      </Popover>,
    );
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Abrir" }));
    screen.getByLabelText("dentro").focus();
    await user.keyboard("{Escape}");
    expect(screen.getByRole("button", { name: "Abrir" })).toHaveFocus();
  });
});
