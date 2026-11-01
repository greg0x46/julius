import { translate, type Language } from "../i18n/messages";
import { DollarOutlined, WalletOutlined } from "../components/ui/icons";
import type { ReactNode } from "react";

import type { Payable, PayableKind, PayableStatus } from "../api/contracts";
import { floorPercent } from "./decimal";

export function payableStatusLabel(kind: PayableKind, status: PayableStatus, language: Language = "pt-BR"): string {
  return translate(language, status === "open" ? "payableOpen" : kind === "debt" ? "debtPaidOff" : "receivableReceived");
}

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

export function getPayableVocabulary(kind: PayableKind, language: Language = "pt-BR"): PayableVocabulary {
  return {
    icon: kind === "debt" ? <WalletOutlined aria-hidden="true" /> : <DollarOutlined aria-hidden="true" />,
    kindLabel: translate(language, kind === "debt" ? "debtKind" : "receivableKind"),
    settledColumnLabel: translate(language, kind === "debt" ? "debtSettled" : "receivableSettled"),
    settledShareLabel: translate(language, kind === "debt" ? "debtSettledLower" : "receivableSettledLower"),
    heroLabel: translate(language, kind === "debt" ? "debtRemaining" : "receivableRemaining"),
    emptyTitle: translate(language, kind === "debt" ? "debtEmpty" : "receivableEmpty"),
    summaryLabel: translate(language, kind === "debt" ? "debtSummary" : "receivableSummary"),
    planNoun: translate(language, kind === "debt" ? "debtPlanNoun" : "receivablePlanNoun"),
    onTrackMessage: translate(language, kind === "debt" ? "debtOnTrack" : "receivableOnTrack"),
    noPlanMessage: translate(language, kind === "debt" ? "debtNoPlan" : "receivableNoPlan"),
    createPlanButtonText: translate(language, kind === "debt" ? "debtCreatePlan" : "receivableCreatePlan"),
    generateFromRemainingText: translate(language, kind === "debt" ? "debtGenerate" : "receivableGenerate"),
    addActionLabel: translate(language, kind === "debt" ? "debtAddAction" : "receivableAddAction"),
    deallocateDescription: translate(language, kind === "debt" ? "debtDeallocate" : "receivableDeallocate"),
    newTitle: translate(language, kind === "debt" ? "debtNew" : "receivableNew"),
    editTitle: translate(language, kind === "debt" ? "debtEdit" : "receivableEdit"),
    nameFieldPlaceholder: translate(language, kind === "debt" ? "debtNameExample" : "receivableNameExample"),
    nameRequiredError: translate(language, kind === "debt" ? "debtNameRequired" : "receivableNameRequired"),
    deleteTitle: translate(language, kind === "debt" ? "debtDelete" : "receivableDelete"),
  };
}

/** The detail route of a payable; the kind rides along so the detail loads the right list cache. */
export function payableDetailPath(payable: Payable): string {
  return `/pendencias/${payable.id}?kind=${payable.kind}`;
}

/** How much is already settled, as a 0–100 integer rounded down, so only a settled payable reads 100%. */
export function settledPercent(totalAmount: string, settledAmount: string): number {
  return floorPercent(settledAmount, totalAmount);
}
