import { App } from "../ui";
import type { ReactNode } from "react";
import { useCallback } from "react";

export interface ConfirmOptions {
  title: string;
  description?: ReactNode;
  /** The action's verb, which is also the button ("Excluir", "Desvincular"). Defaults to "Excluir". */
  okText?: string;
  /** Red confirm button. Defaults to true: this is for what destroys or undoes something. */
  danger?: boolean;
  /**
   * What to do on confirm. Returning a promise keeps the button loading until
   * it settles, then the dialog closes; handle your own errors inside (an
   * inline Alert where the user is), a rejection would keep the dialog open.
   */
  onConfirm: () => void | Promise<unknown>;
}

/** Shared asynchronous confirmation; standalone consumers fall back to the browser dialog. */
export function useConfirm(): (options: ConfirmOptions) => void {
  const { modal } = App.useApp();
  return useCallback(
    ({ title, description, okText = "Excluir", danger = true, onConfirm }) => {
      if (typeof modal?.confirm !== "function") {
        const text = typeof description === "string" ? `${title}\n${description}` : title;
        if (window.confirm(text)) void onConfirm();
        return;
      }
      modal.confirm({
        title,
        content: description,
        okText,
        cancelText: "Cancelar",
        okButtonProps: { danger },
        onOk: onConfirm,
      });
    },
    [modal],
  );
}
