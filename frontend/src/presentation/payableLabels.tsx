import { DollarOutlined, WalletOutlined } from "../components/ui/icons";
import type { ReactNode } from "react";

import type { Payable, PayableKind, PayableStatus } from "../api/contracts";
import { floorPercent } from "./decimal";

export const payableStatusLabel: Record<PayableKind, Record<PayableStatus, string>> = {
  debt: { open: "Aberta", settled: "Quitada" },
  receivable: { open: "Aberta", settled: "Recebida" },
};

// payableVocabulary centralizes the one directional difference between a
// debt ("pagamento" wording) and a receivable ("recebimento" wording) — see
// PayableTimeline, PayableForm, PayableHeaderCard, PayableList — so those
// components read the right copy for whichever kind they're rendering
// instead of existing as two near-duplicate files.
export interface PayableVocabulary {
  /** Only for the create menu's phone sheet; rows and headings carry no icon. */
  icon: ReactNode;
  /** Short kind name used in row meta and the table's "Tipo" column. */
  kindLabel: string;
  settledColumnLabel: string;
  /** Lower-case participle for "pago 42%" / "recebido 10%". */
  settledShareLabel: string;
  /** The small label over the detail hero figure. */
  heroLabel: string;
  /** The empty list's title when no payable of this kind exists yet. */
  emptyTitle: string;
  /** The list summary figure's label ("Você deve" / "Te devem"). */
  summaryLabel: string;
  planNoun: string;
  onTrackMessage: string;
  noPlanMessage: string;
  createPlanButtonText: string;
  generateFromRemainingText: string;
  addActionLabel: string;
  deallocateDescription: string;
  newTitle: string;
  editTitle: string;
  nameFieldPlaceholder: string;
  nameRequiredError: string;
  deleteTitle: string;
}

export const payableVocabulary: Record<PayableKind, PayableVocabulary> = {
  debt: {
    icon: <WalletOutlined aria-hidden="true" />,
    kindLabel: "Dívida",
    settledColumnLabel: "Pago",
    settledShareLabel: "pago",
    heroLabel: "Restante a pagar",
    emptyTitle: "Nenhuma dívida ainda",
    summaryLabel: "Você deve",
    planNoun: "plano de pagamento",
    onTrackMessage: "Em dia com o plano de pagamento até hoje.",
    noPlanMessage: "Esta dívida ainda não tem um plano de pagamento.",
    createPlanButtonText: "Criar plano de pagamento",
    generateFromRemainingText: "Gerar parcelas a partir do valor restante da dívida",
    addActionLabel: "Adicionar pagamento",
    deallocateDescription:
      "A parcela deixa de contar esse valor como pago; a transação em si permanece vinculada à dívida.",
    newTitle: "Nova dívida",
    editTitle: "Editar dívida",
    nameFieldPlaceholder: "Ex.: Financiamento do carro",
    nameRequiredError: "Informe um nome para a dívida.",
    deleteTitle: "Excluir dívida",
  },
  receivable: {
    icon: <DollarOutlined aria-hidden="true" />,
    kindLabel: "A receber",
    settledColumnLabel: "Recebido",
    settledShareLabel: "recebido",
    heroLabel: "Restante a receber",
    emptyTitle: "Nenhuma conta a receber ainda",
    summaryLabel: "Te devem",
    planNoun: "plano de recebimento",
    onTrackMessage: "Em dia com o plano de recebimento até hoje.",
    noPlanMessage: "Esta conta a receber ainda não tem um plano de recebimento.",
    createPlanButtonText: "Criar plano de recebimento",
    generateFromRemainingText: "Gerar parcelas a partir do valor restante da conta a receber",
    addActionLabel: "Adicionar recebimento",
    deallocateDescription:
      "A parcela deixa de contar esse valor como recebido; a transação em si permanece vinculada à conta a receber.",
    newTitle: "Nova conta a receber",
    editTitle: "Editar conta a receber",
    nameFieldPlaceholder: "Ex.: Empréstimo para Ana",
    nameRequiredError: "Informe um nome para a conta a receber.",
    deleteTitle: "Excluir conta a receber",
  },
};

/** The detail route of a payable; the kind rides along so the detail loads the right list cache. */
export function payableDetailPath(payable: Payable): string {
  return `/pendencias/${payable.id}?kind=${payable.kind}`;
}

/** How much is already settled, as a 0–100 integer rounded down, so only a settled payable reads 100%. */
export function settledPercent(totalAmount: string, settledAmount: string): number {
  return floorPercent(settledAmount, totalAmount);
}
