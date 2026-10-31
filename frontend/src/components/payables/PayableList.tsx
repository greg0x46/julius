import { useTranslation } from "../../i18n/context";
import { Skeleton, Table } from "../ui";
import type { ColumnsType } from "../ui/Table";
import type { ReactNode } from "react";
import { Link } from "react-router-dom";

import type { Payable } from "../../api/contracts";
import {
  payableDetailPath,
  payableStatusLabel,
  getPayableVocabulary,
  settledPercent,
} from "../../presentation/payableLabels";
import { ResponsiveList } from "../layout";
import { Money } from "../shared/Money";
import { StatusTag } from "../shared/StatusTag";
import { PayableMeter } from "./PayableMeter";

/** Only a settled payable carries a status; "Aberta" is the default and stays unsaid. */
function SettledTag({ payable }: { payable: Payable }) {
  const { language } = useTranslation();
  return payable.status === "settled" ? (
    <StatusTag tone="success">{payableStatusLabel(payable.kind, "settled", language)}</StatusTag>
  ) : null;
}

type Props = {
  payables: Payable[];
  isLoading: boolean;
  /** What to show when the list is empty — "nothing yet" and "no results" read differently, so the page decides. */
  empty: ReactNode;
  /**
   * Whether each row says if it is a debt or a receivable. Only the mixed
   * "Todas" view needs it: on a Dívidas or A receber tab every row would
   * repeat the tab's own name.
   */
  showKind: boolean;
  onOpen: (payable: Payable) => void;
};



/**
 * The payables as one list, two shapes: stacked rows below `lg` — the table's
 * five columns are squeezed and clipped on a tablet — and a single table from
 * `lg` up. Both open the detail on tap/click; editing and deleting live on the
 * detail page, so a row carries no actions of its own.
 */
export function PayableList({ payables, isLoading, empty, showKind, onOpen }: Props) {
  const { t, language } = useTranslation();
  const loading = <div className="payables-loading" role="status" aria-label={t("loadingPayables")}><Skeleton active paragraph={{ rows: 3 }} /></div>;
  const columns: ColumnsType<Payable> = [
    {
      title: t("name"),
      dataIndex: "name",
      render: (_, payable) => (
        <Link
          to={payableDetailPath(payable)}
          className="payable-name-link"
          onClick={(event) => event.stopPropagation()}
        >
          {payable.name}
        </Link>
      ),
    },
    ...(showKind
      ? [
          {
            title: t("type"),
            dataIndex: "kind",
            className: "payable-kind-cell",
            render: (_: unknown, payable: Payable) => getPayableVocabulary(payable.kind, language).kindLabel,
          },
        ]
      : []),
    {
      title: t("progress"),
      dataIndex: "settled_amount",
      render: (_, payable) => {
        const percent = settledPercent(payable.total_amount, payable.settled_amount);
        return (
          <span className="payable-progress-cell">
            <PayableMeter percent={percent} />
            <span className="payable-progress-label">
              {getPayableVocabulary(payable.kind, language).settledShareLabel} {percent}%
            </span>
          </span>
        );
      },
    },
    {
      title: t("remaining"),
      dataIndex: "remaining_amount",
      align: "right",
      render: (_, payable) => <Money value={payable.remaining_amount} tone="neutral" size="row" />,
    },
    {
      title: t("status"),
      dataIndex: "status",
      render: (_, payable) => <SettledTag payable={payable} />,
    },
  ];

  return (
    <ResponsiveList
      label={t("payables")}
      items={payables}
      getKey={(payable) => payable.id}
      isLoading={isLoading}
      loading={loading}
      empty={empty}
      stackBelow="xl"
      row={(payable) => {
        const percent = settledPercent(payable.total_amount, payable.settled_amount);
        const vocab = getPayableVocabulary(payable.kind, language);
        return {
          title: payable.name,
          meta: (
            <>
              <span className="payable-row-meta-text">
                {showKind && <>{vocab.kindLabel} · </>}
                {vocab.settledShareLabel} {percent}%
              </span>
              <PayableMeter percent={percent} className="payable-row-meter" />
            </>
          ),
          trailing: <Money value={payable.remaining_amount} tone="neutral" />,
          status: payable.status === "settled" ? <SettledTag payable={payable} /> : undefined,
          onClick: () => onOpen(payable),
          ariaLabel: t("openItem") + " " + payable.name,
        };
      }}
      wide={
        <Table<Payable>
          className="payables-table"
          aria-label={t("payables")}
          columns={columns}
          dataSource={payables}
          rowKey="id"
          pagination={false}
          onRow={(payable) => ({ onClick: () => onOpen(payable), style: { cursor: "pointer" } })}
        />
      }
    />
  );
}
