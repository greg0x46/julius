import { CaretDownOutlined, CaretRightOutlined } from "../ui/icons";
import { Grid } from "../ui";
import { useId, useState, type ReactNode } from "react";
import { Link } from "react-router-dom";

import type {
  InvestmentPeriod,
  InvestmentPeriodGroup,
  InvestmentPeriodPosition,
  InvestmentPeriodTotals,
} from "../../api/contracts";
import { formatDecimal, investmentTypeLabel } from "../../presentation/investmentLabels";
import {
  formatRatioPercent,
  investmentAssetClassLabel,
  investedUnavailableHint,
  matchesAssetSearch,
  periodUnavailableLabel,
} from "../../presentation/investmentPeriod";
import { isZeroDecimal } from "../../presentation/decimal";
import { formatMoney } from "../../presentation/money";
import { EmptyState } from "../layout";
import { Money } from "../shared/Money";
import { RecordMenu, type RecordMenuItem } from "../shared/RecordMenu";
import { StatusTag } from "../shared/StatusTag";
import { formatDate } from "./investmentDate";
import { InvestmentHint } from "./InvestmentHint";
import { InvestmentYieldValue } from "./InvestmentYieldValue";

export interface InvestmentPositionsPeriodProps {
  data: InvestmentPeriod;
  /** Free text over name and ticker; it hides rows only. */
  search: string;
  /** Closed-at-the-end positions are hidden by default; totals never change. */
  showClosed: boolean;
  /** Where a position's detail page lives, if it has one. */
  hrefOf: (position: InvestmentPeriodPosition) => string | null;
  positionMenu: (position: InvestmentPeriodPosition) => RecordMenuItem[];
  /** Account actions, offered on the header of an account group. */
  groupMenu: (group: InvestmentPeriodGroup) => RecordMenuItem[];
  empty: ReactNode;
}

const dash = (
  <>
    <span aria-hidden="true">—</span>
    <span className="visually-hidden">Indisponível</span>
  </>
);

/** Why a figure is "—": the same words reach a screen reader and a tap. */
function Unavailable({ label, hint }: { label: string; hint: string }) {
  return (
    <InvestmentHint label={`${label}: ${hint}`} content={hint}>
      {dash}
    </InvestmentHint>
  );
}

function InvestedValue({ position }: { position: InvestmentPeriodPosition }) {
  if (position.unavailable_reason !== null) return dash;
  if (position.invested === null) {
    const reason = position.invested_unavailable_reason;
    return (
      <Unavailable
        label="Valor investido indisponível"
        hint={reason === null ? "O backend não informou o valor investido." : investedUnavailableHint[reason]}
      />
    );
  }
  return <Money value={position.invested} tone="neutral" />;
}

function GroupInvested({ totals }: { totals: InvestmentPeriodTotals }) {
  if (totals.invested_unknown_positions === 0) return <Money value={totals.invested} tone="neutral" />;
  const unknown = totals.invested_unknown_positions;
  return (
    <span className="investment-invested-partial">
      <Money value={totals.invested} tone="neutral" />
      <InvestmentHint
        label={`Soma só das posições com valor investido conhecido; ${unknown} ${unknown === 1 ? "não tem" : "não têm"}.`}
        content={`Soma só das posições com valor investido conhecido; ${unknown} ${unknown === 1 ? "posição não tem" : "posições não têm"}.`}
      >
        *
      </InvestmentHint>
    </span>
  );
}

/** The period's own figures for one position, plus the institution's number for conference. */
function PositionDetails({ position }: { position: InvestmentPeriodPosition }) {
  const rows: [string, ReactNode][] = [
    [`Valor inicial${position.opening_on ? ` (${formatDate(position.opening_on)})` : ""}`, <Money key="s" value={position.start_value ?? "0"} tone="neutral" />],
    ["Aportes", <Money key="c" value={position.contributions ?? "0"} tone="neutral" />],
    ["Resgates", <Money key="w" value={position.withdrawals ?? "0"} tone="neutral" />],
    ["Rendimento", <Money key="y" value={position.yield ?? "0"} tone="result" />],
  ];
  if (position.costs !== null && !isZeroDecimal(position.costs)) {
    rows.push(["Custos (já nos aportes e resgates)", <Money key="k" value={position.costs} tone="neutral" />]);
  }
  if (position.provisioned_taxes !== null && !isZeroDecimal(position.provisioned_taxes)) {
    rows.push(["IR/IOF provisionado (não deduzido)", <Money key="t" value={position.provisioned_taxes} tone="neutral" />]);
  }
  return (
    <dl className="investment-details">
      {rows.map(([label, value]) => (
        <div key={label}>
          <dt>{label}</dt>
          <dd>{value}</dd>
        </div>
      ))}
      {position.reported_profit !== null && (
        <div className="investment-details-reported">
          <dt>Informado pela instituição</dt>
          <dd>
            {formatMoney(position.reported_profit, position.currency_code)}
            {position.reported_as_of !== null && ` em ${formatDate(position.reported_as_of)}`}
            <span className="investment-quiet"> (conferência; não entra nos totais)</span>
          </dd>
        </div>
      )}
    </dl>
  );
}

function YieldFigure({ position }: { position: InvestmentPeriodPosition }) {
  if (position.unavailable_reason !== null || position.yield === null) return dash;
  return (
    <InvestmentHint label={`Detalhes do período de ${position.name}`} content={<PositionDetails position={position} />}>
      <InvestmentYieldValue gain={position.yield} rate={position.yield_rate} />
    </InvestmentHint>
  );
}

function PositionBadges({ position }: { position: InvestmentPeriodPosition }) {
  const reason = position.unavailable_reason;
  return (
    <>
      {position.closed_as_of_to && <StatusTag tone="neutral">Encerrada</StatusTag>}
      {reason !== null && (
        <InvestmentHint label={`${periodUnavailableLabel[reason].short}: ${periodUnavailableLabel[reason].hint}`} content={periodUnavailableLabel[reason].hint}>
          <StatusTag tone="warning">{periodUnavailableLabel[reason].short}</StatusTag>
        </InvestmentHint>
      )}
      {reason === null && position.partial_start && (
        <InvestmentHint
          label={`Início parcial: o período começou em ${formatDate(position.opening_on)}, primeiro dia com preço.`}
          content={`O dia pedido não tinha preço; o período desta posição começa em ${formatDate(position.opening_on)}, o primeiro dia com preço.`}
        >
          <StatusTag tone="info">Início parcial</StatusTag>
        </InvestmentHint>
      )}
      {reason === null && !position.history_complete && (
        <InvestmentHint
          label="Histórico incompleto: as movimentações não cobrem toda a posição, então o valor investido é desconhecido."
          content="As movimentações sincronizadas não cobrem toda a posição. O rendimento é medido sobre o período, mas o valor investido é desconhecido."
        >
          <StatusTag tone="warning">Histórico incompleto</StatusTag>
        </InvestmentHint>
      )}
    </>
  );
}

// Provider types are enums (MUTUAL_FUND); a manual type is the user's own text.
const typeLabel = (position: InvestmentPeriodPosition) =>
  position.source === "synced" ? investmentTypeLabel(position.asset_type) : position.asset_type;

const quantityLabel = (position: InvestmentPeriodPosition) =>
  position.quantity === null ? null : `Qtd. ${formatDecimal(position.quantity)}`;

/** Stacked rows have no columns, so type and custody go in the line. */
function stackedSubtitle(position: InvestmentPeriodPosition, data: InvestmentPeriod): string {
  return [
    position.ticker,
    typeLabel(position),
    data.group_by === "account" ? null : position.account_name,
    quantityLabel(position),
  ]
    .filter(Boolean)
    .join(" · ");
}

function PositionName({ position, href }: { position: InvestmentPeriodPosition; href: string | null }) {
  return href === null ? (
    <>{position.name}</>
  ) : (
    <Link className="investment-name-link" to={href}>
      {position.name}
    </Link>
  );
}

function NativeValue({ position, data }: { position: InvestmentPeriodPosition; data: InvestmentPeriod }) {
  if (position.native === null || position.currency_code === data.currency_code) return null;
  return <span className="investment-quiet">{formatMoney(position.native.end_value, position.currency_code)}</span>;
}

function GroupLabel({ group, shown }: { group: InvestmentPeriodGroup; shown: number }) {
  const total = group.positions.length;
  return (
    <>
      <span className="investment-group-name">{group.label}</span>
      <span className="investment-quiet">
        {shown === total ? `${total} ${total === 1 ? "posição" : "posições"}` : `${shown} de ${total} posições`}
      </span>
      {group.totals.partial && <StatusTag tone="warning">Parcial</StatusTag>}
    </>
  );
}

function share(group: InvestmentPeriodGroup): string {
  return group.share === null ? "—" : formatRatioPercent(group.share);
}

/** The group's share as a figure and a thin bar; the width only sizes the bar. */
function GroupShare({ group }: { group: InvestmentPeriodGroup }) {
  const width = group.share === null ? 0 : Math.max(0, Math.min(100, Number(group.share) * 100));
  return (
    <span className="investment-share">
      <span>{share(group)}</span>
      <span className="investment-share-bar" aria-hidden="true">
        <span style={{ width: `${width}%` }} />
      </span>
    </span>
  );
}

/**
 * The positions of the period, grouped as the backend grouped them. Group
 * subtotals, shares and every figure come from the response; search and the
 * closed switch only choose which rows are shown, never what a subtotal is.
 */
export function InvestmentPositionsPeriod({
  data,
  search,
  showClosed,
  hrefOf,
  positionMenu,
  groupMenu,
  empty,
}: InvestmentPositionsPeriodProps) {
  const idBase = useId();
  // Eight columns need the room of a wide screen; below it the rows stack.
  const stacked = !Grid.useBreakpoint().xl;
  // Groups start collapsed: the subtotal and share are the answer, the rows the detail.
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(new Set());
  const toggle = (key: string) =>
    setExpanded((current) => {
      const next = new Set(current);
      if (!next.delete(key)) next.add(key);
      return next;
    });

  const searching = search.trim() !== "";
  const grouped = data.group_by !== "none";
  // An unavailable position is in no total, so it is told apart from a closed one that is.
  const hiddenClosed = { counted: 0, unmeasured: 0 };
  if (!showClosed) {
    for (const position of data.groups.flatMap((group) => group.positions)) {
      if (!position.closed_as_of_to) continue;
      if (position.unavailable_reason === null) hiddenClosed.counted += 1;
      else hiddenClosed.unmeasured += 1;
    }
  }
  const visible = data.groups
    .map((group) => ({
      group,
      rows: group.positions.filter(
        (position) =>
          (showClosed || !position.closed_as_of_to) &&
          matchesAssetSearch(position, search),
      ),
    }))
    // A search keeps only groups with a match; otherwise a group stays even if its rows are all hidden.
    .filter(({ rows }) => !searching || rows.length > 0);

  if (data.groups.length === 0) return <>{empty}</>;

  const notes = (
    <>
      {searching && (
        <p className="investment-note" role="status">
          A busca só esconde linhas: totais, subtotais e participações continuam sendo os do grupo inteiro.
        </p>
      )}
      {hiddenClosed.counted > 0 && (
        <p className="investment-note">
          Totais e subtotais incluem {hiddenClosed.counted}{" "}
          {hiddenClosed.counted === 1 ? "encerrada oculta" : "encerradas ocultas"}.
        </p>
      )}
      {hiddenClosed.unmeasured > 0 && (
        <p className="investment-note">
          {hiddenClosed.unmeasured} {hiddenClosed.unmeasured === 1 ? "encerrada oculta está" : "encerradas ocultas estão"}{" "}
          sem valor no período e fora dos totais.
        </p>
      )}
    </>
  );

  if (visible.every(({ rows }) => rows.length === 0) && searching) {
    return (
      <>
        {notes}
        <EmptyState title="Nenhuma posição encontrada" hint={`Nada corresponde a “${search.trim()}” neste período.`} />
      </>
    );
  }

  // The grouping already says one of them; ungrouped rows carry both.
  const showClass = data.group_by !== "class";
  const showCustody = data.group_by !== "account";
  const columns = 5 + Number(showClass) + Number(showCustody);

  const menu = (items: RecordMenuItem[], label: string) =>
    items.length === 0 ? null : <RecordMenu label={label} items={items} />;

  if (stacked) {
    return (
      <>
        {notes}
        <div className="investment-groups">
          {visible.map(({ group, rows }) => {
            const open = !grouped || searching || expanded.has(group.key);
            const id = `${idBase}-${group.key}`;
            return (
              <section key={group.key} className="investment-group" aria-label={group.label}>
                {grouped && (
                  <div className="investment-group-header">
                    <button type="button" className="investment-group-toggle" aria-expanded={open} aria-controls={id} disabled={searching} onClick={() => toggle(group.key)}>
                      {open ? <CaretDownOutlined aria-hidden="true" /> : <CaretRightOutlined aria-hidden="true" />}
                      <span className="investment-group-text">
                        <span className="investment-group-title">
                          <GroupLabel group={group} shown={rows.length} />
                        </span>
                        <span className="investment-quiet investment-group-share">
                          <GroupShare group={group} />
                        </span>
                      </span>
                      <span className="investment-row-figures">
                        <span className="investment-row-value"><Money value={group.totals.end_value} tone="balance" /></span>
                        <span className="investment-row-yield">
                          <InvestmentYieldValue gain={group.totals.yield} rate={group.totals.yield_rate} />
                        </span>
                      </span>
                    </button>
                    {menu(groupMenu(group), `Ações de ${group.label}`)}
                  </div>
                )}
                {open && (
                  <ul id={id} className="list-rows" aria-label={`Posições de ${group.label}`}>
                    {rows.map((position) => (
                      <li key={`${position.source}-${position.id}`} className="list-row-item">
                        <div className="list-row">
                          <span className="list-row-text">
                            <span className="list-row-title">
                              <PositionName position={position} href={hrefOf(position)} />
                            </span>
                            <span className="list-row-meta">{stackedSubtitle(position, data)}</span>
                            <span className="list-row-meta investment-row-badges">
                              <PositionBadges position={position} />
                            </span>
                            {position.unavailable_reason === null && (
                              <span className="investment-mobile-metrics">
                                <span>
                                  <span className="investment-mobile-metric-label">Investido</span>
                                  <InvestedValue position={position} />
                                </span>
                                <span>
                                  <span className="investment-mobile-metric-label">Participação</span>
                                  {position.share === null ? "—" : formatRatioPercent(position.share)}
                                </span>
                              </span>
                            )}
                          </span>
                          <span className="list-row-side">
                            <span className="list-row-trailing">
                              <span className="investment-row-trailing">
                                <span className="investment-row-figures">
                                  <span className="investment-row-value">
                                    {position.end_value === null ? dash : <Money value={position.end_value} tone="balance" />}
                                  </span>
                                  <NativeValue position={position} data={data} />
                                  <span className="investment-row-yield">
                                    <YieldFigure position={position} />
                                  </span>
                                </span>
                                {menu(positionMenu(position), `Ações de ${position.name}`)}
                              </span>
                            </span>
                          </span>
                        </div>
</li>
                    ))}
                    {rows.length === 0 && (
                      <li className="list-row-item investment-quiet investment-group-empty">Nenhuma posição visível neste grupo.</li>
                    )}
                  </ul>
                )}
              </section>
            );
          })}
        </div>
      </>
    );
  }

  return (
    <>
      {notes}
      <div className="investment-table-wrap">
        <table className="investment-period-table" aria-label="Posições">
          <thead>
            <tr>
              <th scope="col">Ativo</th>
              {showClass && <th scope="col">Classe</th>}
              {showCustody && <th scope="col">Custódia</th>}
              <th scope="col" className="investment-col-end">Valor atual</th>
              <th scope="col" className="investment-col-end">Valor investido</th>
              <th scope="col" className="investment-col-end">Rendimento</th>
              <th scope="col" className="investment-col-end">Participação</th>
              <th scope="col" className="investment-col-menu">
                <span className="visually-hidden">Ações</span>
              </th>
            </tr>
          </thead>
          {visible.map(({ group, rows }) => {
            const open = !grouped || searching || expanded.has(group.key);
            return (
              <tbody key={group.key}>
                {grouped && (
                  <tr className="investment-group-row">
                    <th scope="row">
                      <button type="button" className="investment-group-toggle" aria-expanded={open} disabled={searching} onClick={() => toggle(group.key)}>
                        {open ? <CaretDownOutlined aria-hidden="true" /> : <CaretRightOutlined aria-hidden="true" />}
                        <GroupLabel group={group} shown={rows.length} />
                      </button>
                    </th>
                    {showClass && <td />}
                    {showCustody && <td />}
                    <td className="investment-col-end"><Money value={group.totals.end_value} tone="balance" /></td>
                    <td className="investment-col-end"><GroupInvested totals={group.totals} /></td>
                    <td className="investment-col-end">
                      <InvestmentYieldValue gain={group.totals.yield} rate={group.totals.yield_rate} />
                    </td>
                    <td className="investment-col-end"><GroupShare group={group} /></td>
                    <td className="investment-col-menu">{menu(groupMenu(group), `Ações de ${group.label}`)}</td>
                  </tr>
                )}
                {open &&
                  rows.map((position) => (
                    <tr key={`${position.source}-${position.id}`} className="investment-position-row">
                      <th scope="row">
                        <div className="investment-cell">
                          <span className="investment-cell-title">
                            <PositionName position={position} href={hrefOf(position)} />
                          </span>
                          <span className="investment-quiet">
                            {[position.ticker, quantityLabel(position)].filter(Boolean).join(" · ")}
                          </span>
                          <span className="investment-row-badges">
                            <PositionBadges position={position} />
                          </span>
                        </div>
                      </th>
                      {showClass && (
                        <td>
                          <div className="investment-cell">
                            <span className="investment-nowrap">{investmentAssetClassLabel[position.asset_class]}</span>
                            <span className="investment-quiet">{typeLabel(position)}</span>
                          </div>
                        </td>
                      )}
                      {showCustody && <td>{position.account_name}</td>}
                      <td className="investment-col-end">
                        <div className="investment-cell investment-cell-end">
                          <span className="investment-cell-value">
                            {position.end_value === null ? dash : <Money value={position.end_value} tone="balance" />}
                          </span>
                          <NativeValue position={position} data={data} />
                        </div>
                      </td>
                      <td className="investment-col-end"><InvestedValue position={position} /></td>
                      <td className="investment-col-end"><YieldFigure position={position} /></td>
                      <td className="investment-col-end">
                        {position.share === null ? dash : formatRatioPercent(position.share)}
                      </td>
                      <td className="investment-col-menu">{menu(positionMenu(position), `Ações de ${position.name}`)}</td>
                    </tr>
                  ))}
                {open && rows.length === 0 && (
                  <tr>
                    <td colSpan={columns} className="investment-quiet investment-group-empty">
                      Nenhuma posição visível neste grupo.
                    </td>
                  </tr>
                )}
              </tbody>
            );
          })}
        </table>
      </div>
    </>
  );
}
