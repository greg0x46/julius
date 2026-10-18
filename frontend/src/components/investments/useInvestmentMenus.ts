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
  const confirm = useConfirm();
  const navigate = useNavigate();

  const accountMenu = (account: InvestmentAccount): RecordMenuItem[] => [
    { key: "operations", label: "Ver movimentações", onClick: () => handlers.onViewOperations(account) },
    ...(account.kind === "manual"
      ? [
          { key: "edit", label: "Editar conta", onClick: () => handlers.onRename(account) },
          {
            key: "delete",
            label: "Excluir conta",
            danger: true,
            disabled: handlers.busy,
            onClick: () =>
              confirm({
                title: "Excluir conta de investimento",
                description: "Só é possível excluir uma conta sem posições nem movimentações.",
                onConfirm: () => handlers.onRemove(account),
              }),
          },
        ]
      : [
          {
            key: "link",
            label: account.financial_account_id === null ? "Vincular caixa da corretora" : "Caixa da corretora",
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
        label: "Ver detalhes",
        onClick: () => navigate(`/investimentos/${position.linked_investment_id}`),
      });
    }
    if (position.source !== "synced") {
      items.push({
        key: "operations",
        label: "Ver movimentações",
        onClick: () => handlers.onViewPositionOperations(position),
      });
      items.push({ key: "edit", label: "Editar", onClick: () => handlers.onEditPosition(position) });
      items.push({
        key: "delete",
        label: "Excluir",
        danger: true,
        onClick: () =>
          confirm({
            title: "Excluir posição",
            description: "Só é possível excluir uma posição sem movimentações.",
            onConfirm: () => handlers.onRemovePosition(position),
          }),
      });
    }
    return items;
  };

  return { accountMenu, positionMenu };
}
