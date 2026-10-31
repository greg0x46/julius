import { useTranslation } from "../../i18n/context";
import { useNavigate } from "react-router-dom";

import type { InvestmentAccount, InvestmentPosition } from "../../api/contracts";
import type { RecordMenuItem } from "../shared/RecordMenu";
import { useConfirm } from "../shared/useConfirm";

export interface InvestmentMenuHandlers {
  busy: boolean;
  onRename: (account: InvestmentAccount) => void;
  onRemove: (account: InvestmentAccount) => void;
  onViewOperations: (account: InvestmentAccount) => void;
  onViewPositionOperations: (position: InvestmentPosition) => void;
  onEditPosition: (position: InvestmentPosition) => void;
  onRemovePosition: (position: InvestmentPosition) => void;
}

/**
 * The record actions of accounts and positions, shared by every place that
 * lists them (the account groups of the period view and the accounts drawer).
 * A synced account and a synced position stay read-only.
 */
export function useInvestmentMenus(handlers: InvestmentMenuHandlers) {
  const { t } = useTranslation();
  const confirm = useConfirm();
  const navigate = useNavigate();

  const accountMenu = (account: InvestmentAccount): RecordMenuItem[] => [
    { key: "operations", label: t("viewOperations"), onClick: () => handlers.onViewOperations(account) },
    ...(account.kind === "manual"
      ? [
          { key: "edit", label: t("editAccount"), onClick: () => handlers.onRename(account) },
          {
            key: "delete",
            label: t("deleteAccount"),
            danger: true,
            disabled: handlers.busy,
            onClick: () =>
              confirm({
                title: t("deleteInvestmentAccount"),
                description: t("deleteInvestmentAccountHint"),
                onConfirm: () => handlers.onRemove(account),
              }),
          },
        ]
      : [
          {
            key: "link",
            label: account.financial_account_id === null ? t("linkBrokerageCash") : t("brokerageCash"),
            onClick: () => handlers.onRename(account),
          },
        ]),
  ];

  const positionMenu = (position: InvestmentPosition | undefined): RecordMenuItem[] => {
    if (position === undefined) return [];
    const items: RecordMenuItem[] = [];
    if (position.linked_investment_id) {
      items.push({
        key: "details",
        label: t("viewDetails"),
        onClick: () => navigate(`/investimentos/${position.linked_investment_id}`),
      });
    }
    if (position.source !== "synced") {
      items.push({
        key: "operations",
        label: t("viewOperations"),
        onClick: () => handlers.onViewPositionOperations(position),
      });
      items.push({ key: "edit", label: t("edit"), onClick: () => handlers.onEditPosition(position) });
      items.push({
        key: "delete",
        label: t("delete"),
        danger: true,
        onClick: () =>
          confirm({
            title: t("deletePosition"),
            description: t("deletePositionHint"),
            onConfirm: () => handlers.onRemovePosition(position),
          }),
      });
    }
    return items;
  };

  return { accountMenu, positionMenu };
}
