import { Skeleton, Table } from "../ui";
import type { ColumnsType } from "../ui/Table";
import type { ReactNode } from "react";
import { Link } from "react-router-dom";

import type { Payable } from "../../api/contracts";
import {
  payableDetailPath,
  payableStatusLabel,
  payableVocabulary,
  settledPercent,
} from "../../presentation/payableLabels";
import { ResponsiveList } from "../layout";
import { Money } from "../shared/Money";
import { StatusTag } from "../shared/StatusTag";
import { PayableMeter } from "./PayableMeter";

/** Only a settled payable carries a status; "Aberta" is the default and stays unsaid. */
function SettledTag({ payable }: { payable: Payable }) {
  return payable.status === "settled" ? (
    <StatusTag tone="success">{payableStatusLabel[payable.kind].settled}</StatusTag>
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

const loading = (
  <div className="payables-loading" role="status" aria-label="Carregando pendências">
    <Skeleton active paragraph={{ rows: 3 }} />
  </div>
);

/**
 * The payables as one list, two shapes: stacked rows below `lg` — the table's
 * five columns are squeezed and clipped on a tablet — and a single table from
 * `lg` up. Both open the detail on tap/click; editing and deleting live on the
 * detail page, so a row carries no actions of its own.
 */
export function PayableList({ payables, isLoading, empty, showKind, onOpen }: Props) {
  const columns: ColumnsType<Payable> = [
    {
      title: "Nome",
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
            title: "Tipo",
            dataIndex: "kind",
            className: "payable-kind-cell",
            render: (_: unknown, payable: Payable) => payableVocabulary[payable.kind].kindLabel,
          },
        ]
      : []),
    {
      title: "Progresso",
      dataIndex: "settled_amount",
      render: (_, payable) => {
        const percent = settledPercent(payable.total_amount, payable.settled_amount);
        return (
          <span className="payable-progress-cell">
            <PayableMeter percent={percent} />
            <span className="payable-progress-label">
              {payableVocabulary[payable.kind].settledShareLabel} {percent}%
            </span>
          </span>
        );
      },
    },
    {
      title: "Restante",
      dataIndex: "remaining_amount",
      align: "right",
      render: (_, payable) => <Money value={payable.remaining_amount} tone="neutral" size="row" />,
    },
    {
      title: "Situação",
      dataIndex: "status",
      render: (_, payable) => <SettledTag payable={payable} />,
    },
  ];

  return (
    <ResponsiveList
      label="Pendências"
      items={payables}
      getKey={(payable) => payable.id}
      isLoading={isLoading}
      loading={loading}
      empty={empty}
      stackBelow="xl"
      row={(payable) => {
        const percent = settledPercent(payable.total_amount, payable.settled_amount);
        const vocab = payableVocabulary[payable.kind];
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
          ariaLabel: `Abrir ${payable.name}`,
        };
      }}
      wide={
        <Table<Payable>
          className="payables-table"
          aria-label="Pendências"
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
