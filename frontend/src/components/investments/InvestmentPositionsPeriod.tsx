import { translate, type Language } from "../../i18n/messages";
import { useTranslation } from "../../i18n/context";
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
  investmentClassName,
  investmentUnavailable,
  matchesAssetSearch,
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

function Dash() {
  const { t } = useTranslation();
  return <><span aria-hidden="true">—</span><span className="visually-hidden">{t("unavailable")}</span></>;
}

/** Why a figure is "—": the same words reach a screen reader and a tap. */
function Unavailable({ label, hint }: { label: string; hint: string }) {
  return (
    <InvestmentHint label={`${label}: ${hint}`} content={hint}>
      {<Dash />}
    </InvestmentHint>
  );
}

function InvestedValue({ position }: { position: InvestmentPeriodPosition }) {
  const { t, language } = useTranslation();
  if (position.unavailable_reason !== null) return <Dash />;
  if (position.invested === null) {
    const reason = position.invested_unavailable_reason;
    return (
      <Unavailable
        label={t("investedValueUnavailable")}
        hint={reason === null ? t("investedValueNotProvided") : investmentUnavailable(reason, language).investedHint}
      />
    );
  }
  return <Money value={position.invested} tone="neutral" />;
}

function GroupInvested({ totals }: { totals: InvestmentPeriodTotals }) {
  const { t } = useTranslation();
  if (totals.invested_unknown_positions === 0) return <Money value={totals.invested} tone="neutral" />;
  const unknown = totals.invested_unknown_positions;
  return (
    <span className="investment-invested-partial">
      <Money value={totals.invested} tone="neutral" />
      <InvestmentHint
        label={t("knownInvestedSum", { count: unknown, verb: t(unknown === 1 ? "noInvestedOne" : "noInvestedMany") })}
        content={t(unknown === 1 ? "knownInvestedSumDetailOne" : "knownInvestedSumDetailMany", { count: unknown })}
      >
        *
      </InvestmentHint>
    </span>
  );
}

/** The period's own figures for one position, plus the institution's number for conference. */
function PositionDetails({ position }: { position: InvestmentPeriodPosition }) {
  const { t, language } = useTranslation();
  const rows: [string, ReactNode][] = [
    [`${t("startValue")}${position.opening_on ? ` (${formatDate(position.opening_on, language)})` : ""}`, <Money key="s" value={position.start_value ?? "0"} tone="neutral" />],
    [t("contributions"), <Money key="c" value={position.contributions ?? "0"} tone="neutral" />],
    [t("withdrawals"), <Money key="w" value={position.withdrawals ?? "0"} tone="neutral" />],
    [t("investmentYield"), <Money key="y" value={position.yield ?? "0"} tone="result" />],
  ];
  if (position.costs !== null && !isZeroDecimal(position.costs)) {
    rows.push([t("costsIncludedInFlows"), <Money key="k" value={position.costs} tone="neutral" />]);
  }
  if (position.provisioned_taxes !== null && !isZeroDecimal(position.provisioned_taxes)) {
    rows.push([t("taxProvisioned"), <Money key="t" value={position.provisioned_taxes} tone="neutral" />]);
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
          <dt>{t("reportedByInstitution")}</dt>
          <dd>
            {formatMoney(position.reported_profit, position.currency_code)}
            {position.reported_as_of !== null && ` ${t("atDate")} ${formatDate(position.reported_as_of, language)}`}
            <span className="investment-quiet">{t("reportedNotTotaled")}</span>
          </dd>
        </div>
      )}
    </dl>
  );
}

function YieldFigure({ position }: { position: InvestmentPeriodPosition }) {
  const { t } = useTranslation();
  if (position.unavailable_reason !== null || position.yield === null) return <Dash />;
  return (
    <InvestmentHint label={t("periodDetailsOf", { name: position.name })} content={<PositionDetails position={position} />}>
      <InvestmentYieldValue gain={position.yield} rate={position.yield_rate} />
    </InvestmentHint>
  );
}

function PositionBadges({ position }: { position: InvestmentPeriodPosition }) {
  const { t, language } = useTranslation();
  const reason = position.unavailable_reason;
  return (
    <>
      {position.closed_as_of_to && <StatusTag tone="neutral">{t("closedFeminine")}</StatusTag>}
      {reason !== null && (
        <InvestmentHint label={`${investmentUnavailable(reason, language).short}: ${investmentUnavailable(reason, language).hint}`} content={investmentUnavailable(reason, language).hint}>
          <StatusTag tone="warning">{investmentUnavailable(reason, language).short}</StatusTag>
        </InvestmentHint>
      )}
      {reason === null && position.partial_start && (
        <InvestmentHint
          label={t("partialStartLabel", { date: formatDate(position.opening_on, language) })}
          content={t("partialStartHint", { date: formatDate(position.opening_on, language) })}
        >
          <StatusTag tone="info">{t("partialStart")}</StatusTag>
        </InvestmentHint>
      )}
      {reason === null && !position.history_complete && (
        <InvestmentHint
          label={t("incompleteHistoryLabel")}
          content={t("incompleteHistoryHint")}
        >
          <StatusTag tone="warning">{t("incompleteInvestmentHistory")}</StatusTag>
        </InvestmentHint>
      )}
    </>
  );
}

// Provider types are enums (MUTUAL_FUND); a manual type is the user's own text.
const typeLabel = (position: InvestmentPeriodPosition, language: Language) =>
  position.source === "synced" ? investmentTypeLabel(position.asset_type, language) : position.asset_type;

const quantityLabel = (position: InvestmentPeriodPosition, language: Language) =>
  position.quantity === null ? null : translate(language, "quantityAbbreviation", { quantity: formatDecimal(position.quantity, language) });

/** Stacked rows have no columns, so type and custody go in the line. */
function stackedSubtitle(position: InvestmentPeriodPosition, data: InvestmentPeriod, language: Language): string {
  return [
    position.ticker,
    typeLabel(position, language),
    data.group_by === "account" ? null : position.account_name,
    quantityLabel(position, language),
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
  const { t, language } = useTranslation();
  const total = group.positions.length;
  return (
    <>
      <span className="investment-group-name">{group.asset_class ? investmentClassName(group.asset_class, language) : group.label}</span>
      <span className="investment-quiet">
        {shown === total ? t(total === 1 ? "positionCountOne" : "positionCountMany", { count: total }) : t("positionShownCount", { shown, total })}
      </span>
      {group.totals.partial && <StatusTag tone="warning">{t("partial")}</StatusTag>}
    </>
  );
}


/** The group's share as a figure and a thin bar; the width only sizes the bar. */
function GroupShare({ group }: { group: InvestmentPeriodGroup }) {
  const { language } = useTranslation();
  const width = group.share === null ? 0 : Math.max(0, Math.min(100, Number(group.share) * 100));
  return (
    <span className="investment-share">
      <span>{group.share === null ? "—" : formatRatioPercent(group.share, { language })}</span>
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
  const { t, language } = useTranslation();
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
          {t("searchHidesOnlyRows")}
        </p>
      )}
      {hiddenClosed.counted > 0 && (
        <p className="investment-note">
          {t(hiddenClosed.counted === 1 ? "hiddenClosedCountOne" : "hiddenClosedCountMany", { count: hiddenClosed.counted })}
        </p>
      )}
      {hiddenClosed.unmeasured > 0 && (
        <p className="investment-note">
          {t(hiddenClosed.unmeasured === 1 ? "hiddenUnmeasuredOne" : "hiddenUnmeasuredMany", { count: hiddenClosed.unmeasured })}
        </p>
      )}
    </>
  );

  if (visible.every(({ rows }) => rows.length === 0) && searching) {
    return (
      <>
        {notes}
        <EmptyState title={t("noPositionsFound")} hint={t("noPositionsMatch", { search: search.trim() })} />
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
              <section key={group.key} className="investment-group" aria-label={group.asset_class ? investmentClassName(group.asset_class, language) : group.label}>
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
                    {menu(groupMenu(group), t("actionsFor", { name: group.asset_class ? investmentClassName(group.asset_class, language) : group.label }))}
                  </div>
                )}
                {open && (
                  <ul id={id} className="list-rows" aria-label={t("positionsOf", { name: group.asset_class ? investmentClassName(group.asset_class, language) : group.label })}>
                    {rows.map((position) => (
                      <li key={`${position.source}-${position.id}`} className="list-row-item">
                        <div className="list-row">
                          <span className="list-row-text">
                            <span className="list-row-title">
                              <PositionName position={position} href={hrefOf(position)} />
                            </span>
                            <span className="list-row-meta">{stackedSubtitle(position, data, language)}</span>
                            <span className="list-row-meta investment-row-badges">
                              <PositionBadges position={position} />
                            </span>
                            {position.unavailable_reason === null && (
                              <span className="investment-mobile-metrics">
                                <span>
                                  <span className="investment-mobile-metric-label">{t("invested")}</span>
                                  <InvestedValue position={position} />
                                </span>
                                <span>
                                  <span className="investment-mobile-metric-label">{t("share")}</span>
                                  {position.share === null ? "—" : formatRatioPercent(position.share, { language })}
                                </span>
                              </span>
                            )}
                          </span>
                          <span className="list-row-side">
                            <span className="list-row-trailing">
                              <span className="investment-row-trailing">
                                <span className="investment-row-figures">
                                  <span className="investment-row-value">
                                    {position.end_value === null ? <Dash /> : <Money value={position.end_value} tone="balance" />}
                                  </span>
                                  <NativeValue position={position} data={data} />
                                  <span className="investment-row-yield">
                                    <YieldFigure position={position} />
                                  </span>
                                </span>
                                {menu(positionMenu(position), t("actionsFor", { name: position.name }))}
                              </span>
                            </span>
                          </span>
                        </div>
</li>
                    ))}
                    {rows.length === 0 && (
                      <li className="list-row-item investment-quiet investment-group-empty">{t("noVisiblePositions")}</li>
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
        <table className="investment-period-table" aria-label={t("positions")}>
          <thead>
            <tr>
              <th scope="col">{t("asset")}</th>
              {showClass && <th scope="col">{t("assetClass")}</th>}
              {showCustody && <th scope="col" className="investment-col-custody">{t("custody")}</th>}
              <th scope="col" className="investment-col-end">{t("currentValueLabel")}</th>
              <th scope="col" className="investment-col-end">{t("investedValueLabel")}</th>
              <th scope="col" className="investment-col-end">{t("investmentYield")}</th>
              <th scope="col" className="investment-col-end">{t("share")}</th>
              <th scope="col" className="investment-col-menu">
                <span className="visually-hidden">{t("actions")}</span>
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
                    <td className="investment-col-menu">{menu(groupMenu(group), t("actionsFor", { name: group.asset_class ? investmentClassName(group.asset_class, language) : group.label }))}</td>
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
                            {[position.ticker, quantityLabel(position, language)].filter(Boolean).join(" · ")}
                          </span>
                          <span className="investment-row-badges">
                            <PositionBadges position={position} />
                          </span>
                        </div>
                      </th>
                      {showClass && (
                        <td>
                          <div className="investment-cell">
                            <span className="investment-nowrap">{investmentClassName(position.asset_class, language)}</span>
                            <span className="investment-quiet">{typeLabel(position, language)}</span>
                          </div>
                        </td>
                      )}
                      {showCustody && <td className="investment-col-custody">{position.account_name}</td>}
                      <td className="investment-col-end">
                        <div className="investment-cell investment-cell-end">
                          <span className="investment-cell-value">
                            {position.end_value === null ? <Dash /> : <Money value={position.end_value} tone="balance" />}
                          </span>
                          <NativeValue position={position} data={data} />
                        </div>
                      </td>
                      <td className="investment-col-end"><InvestedValue position={position} /></td>
                      <td className="investment-col-end"><YieldFigure position={position} /></td>
                      <td className="investment-col-end">
                        {position.share === null ? <Dash /> : formatRatioPercent(position.share, { language })}
                      </td>
                      <td className="investment-col-menu">{menu(positionMenu(position), t("actionsFor", { name: position.name }))}</td>
                    </tr>
                  ))}
                {open && rows.length === 0 && (
                  <tr>
                    <td colSpan={columns} className="investment-quiet investment-group-empty">
                      {t("noVisiblePositions")}
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
