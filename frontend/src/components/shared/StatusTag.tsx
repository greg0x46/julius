import type { ReactNode } from "react";

export type StatusTagTone = "neutral" | "info" | "success" | "warning" | "danger";

interface StatusTagProps {
  /**
   * How the state reads: `danger` for what needs action (Atrasada, Falhou),
   * `warning` for what needs a look (Parcial, Fora dos totais), `success` for
   * a finished good state (Quitada), `info` for a neutral fact worth flagging
   * (Em andamento) and `neutral` for the quietest (Ignorada, Encerrada).
   */
  tone: StatusTagTone;
  children: ReactNode;
  className?: string;
}

/**
 * The one way to show a record's status: a small text chip (12px/500) with a
 * tinted background and toned text. No border or icon
 * colours.
 *
 * Only for non-default states. The common case carries no tag at all — an
 * open payable is not "Aberta", a synced run is not "Concluída", a new import
 * row is not "Nova"; a list where every row wears a tag has no tags. The text
 * must say the state in words (the colour is reinforcement, never the only
 * signal).
 */
export function StatusTag({ tone, children, className }: StatusTagProps) {
  return <span className={["status-tag", `status-tag-${tone}`, className].filter(Boolean).join(" ")}>{children}</span>;
}
