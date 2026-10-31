import { useTranslation } from "../../i18n/context";
import type { TransactionInclusionState, TransactionItem } from "../../api/contracts";
import { RecordMenu, type RecordMenuItem } from "../shared/RecordMenu";

/**
 * The wide row's secondary actions behind a single "···": what used to be a
 * permanent "Ignorar" link on every line. Only actions the row can already
 * perform are listed — deep links into the panel's sub-screens stay in the
 * panel.
 */
export function TransactionActions({
  item,
  onSelect,
  onInclusion,
  pending = false,
}: {
  item: TransactionItem;
  onSelect: (id: string) => void;
  onInclusion?: (id: string, state: TransactionInclusionState) => void;
  pending?: boolean;
}) {
  const { t } = useTranslation();
  const ignored = item.inclusion.state === "ignored";
  const name = item.description ?? t("transactionWithoutDescription");
  const items: RecordMenuItem[] = [
    { key: "details", label: t("viewDetails"), onClick: () => onSelect(item.id) },
    {
      key: "inclusion",
      label: ignored ? t("includeInTotals") : t("ignore"),
      disabled: !onInclusion || pending,
      onClick: () => onInclusion?.(item.id, ignored ? "considered" : "ignored"),
    },
  ];
  return (
    // Sits above the row's stretched hit area (see TransactionRow).
    <span className="transaction-actions">
      <RecordMenu
        className="transaction-actions-trigger"
        label={t("actionsFor", { name })}
        items={items}
        loading={pending}
      />
    </span>
  );
}
