import { useTranslation } from "../../i18n/context";
import { ExclamationCircleOutlined, InfoCircleOutlined } from "../ui/icons";
import { useState } from "react";

import type { InvestmentPeriod, InvestmentPeriodPart, InvestmentPeriodPosition } from "../../api/contracts";
import { formatDateOnly } from "../../presentation/dates";
import { isZeroDecimal } from "../../presentation/decimal";
import { partialReasons } from "../../presentation/investmentPeriod";
import { SummaryStrip } from "../layout";
import { Money } from "../shared/Money";
import { InvestmentHint } from "./InvestmentHint";
import { InvestmentYieldValue } from "./InvestmentYieldValue";

const partIsEmpty = (part: InvestmentPeriodPart) =>
  [part.start_value, part.end_value, part.contributions, part.withdrawals, part.yield, part.capital_base].every(isZeroDecimal);

/**
 * The period's result, as the backend states it: patrimônio at the end of the
 * period as the hero, rendimento and rentabilidade beside it, aportes,
 * resgates and custos as secondary figures. Caveats, the partial warning
 * included, sit behind the info icon beside the hero. Nothing here is summed
 * or divided in the browser; positions the backend could not measure are
 * named, never counted as zero.
 */
export function InvestmentPeriodSummary({
  data,
  positions,
  updating,
}: {
  data: InvestmentPeriod;
  positions: InvestmentPeriodPosition[];
  updating: boolean;
}) {
  const { t, language } = useTranslation();
  const [explained, setExplained] = useState(false);
  const { totals } = data;
  const reasons = partialReasons(positions, language);
  const items = [
    {
      label: t("investmentYield"),
      value:
        totals.yield_rate === null ? (
          <span className="investment-summary-result">
            <InvestmentYieldValue gain={totals.yield} rate={null} />
            <InvestmentHint
              label={t("unavailableReturnLabel")}
              content={t("unavailableReturnHint")}
            >
              <span className="investment-summary-unavailable">—</span>
            </InvestmentHint>
          </span>
        ) : (
          <InvestmentYieldValue gain={totals.yield} rate={totals.yield_rate} />
        ),
    },
    { label: t("contributions"), value: <Money value={totals.contributions} tone="neutral" /> },
    { label: t("withdrawals"), value: <Money value={totals.withdrawals} tone="neutral" /> },
    ...(isZeroDecimal(totals.costs) ? [] : [{ label: t("costs"), value: <Money value={totals.costs} tone="neutral" /> }]),
  ];

  const notes = [
    t("investmentCashExcluded"),
    isZeroDecimal(totals.provisioned_taxes)
      ? null
      : t("investmentTaxHint"),
    data.to_clamped ? t("periodClamped", { date: formatDateOnly(data.to, language) }) : null,
    totals.positions_partial_start > 0
      ? t(totals.positions_partial_start === 1 ? "onePositionPartialStart" : "manyPositionsPartialStart", { count: totals.positions_partial_start })
      : null,
  ].filter((note): note is string => note !== null);
  const partialTitle = t("partialInvestmentSummary", { unavailable: totals.positions_unavailable, total: totals.positions_total });

  return (
    <SummaryStrip
      className="investment-period-summary"
      label={t("portfolioOn", { date: formatDateOnly(data.to, language) })}
      value={
        <>
          <Money value={totals.end_value} tone="balance" size="hero" />
          <InvestmentHint
            label={`${totals.partial ? `${partialTitle}. ${reasons.join(", ")}. ` : ""}${t("aboutValues", { notes: notes.join(" ") })}`}
            content={
              <>
                {totals.partial && (
                  <>
                    <p>
                      <strong>{partialTitle}</strong>
                    </p>
                    <p>{t("partialPositionsExcluded")}</p>
                    <ul>
                      {reasons.map((reason) => (
                        <li key={reason}>{reason}</li>
                      ))}
                    </ul>
                  </>
                )}
                <ul>
                  {notes.map((note) => (
                    <li key={note}>{note}</li>
                  ))}
                </ul>
              </>
            }
          >
            {totals.partial ? (
              <ExclamationCircleOutlined className="investment-summary-info investment-summary-info-warning" />
            ) : (
              <InfoCircleOutlined className="investment-summary-info" />
            )}
          </InvestmentHint>
        </>
      }
      busy={updating}
      items={items}
    >
      <details
        className="investment-reconcile-disclosure"
        open={explained}
        onToggle={(event) => setExplained(event.currentTarget.open)}
      >
        <summary>{t("calculationDetails")}</summary>
        {explained && (
          <>
            <dl id="investment-reconcile" className="investment-reconcile">
              <div>
                <dt>
                  {t("startValue")} (
                  {data.from === null ? t("startEachPosition") : t("beforeDate", { date: formatDateOnly(data.from, language) })})
                </dt>
                <dd><Money value={totals.start_value} tone="neutral" /></dd>
              </div>
              <div>
                <dt>+ {t("contributions")}</dt>
                <dd><Money value={totals.contributions} tone="neutral" /></dd>
              </div>
              <div>
                <dt>− {t("withdrawals")}</dt>
                <dd><Money value={totals.withdrawals} tone="neutral" /></dd>
              </div>
              <div>
                <dt>+ {t("investmentYield")}</dt>
                <dd><Money value={totals.yield} tone="result" /></dd>
              </div>
              <div className="investment-reconcile-total">
                <dt>= {t("endingPortfolio", { date: formatDateOnly(data.to, language) })}</dt>
                <dd><Money value={totals.end_value} tone="balance" /></dd>
              </div>
            </dl>
            <dl className="investment-reconcile" role="group" aria-label={t("periodResultSource")}>
              <div className="investment-reconcile-total">
                <dt>{t("periodResultEquation")}</dt>
                <dd><InvestmentYieldValue gain={totals.yield} rate={totals.yield_rate} /></dd>
              </div>
              <div>
                <dt>{t("positionsHeldOn", { date: formatDateOnly(data.to, language) })}</dt>
                <dd><InvestmentYieldValue gain={totals.held.yield} rate={totals.held.yield_rate} /></dd>
              </div>
              {!partIsEmpty(totals.closed) && (
                <div>
                  <dt>{t("closedPeriodResult")}</dt>
                  <dd><InvestmentYieldValue gain={totals.closed.yield} rate={totals.closed.yield_rate} /></dd>
                </div>
              )}
            </dl>
          </>
        )}
      </details>
    </SummaryStrip>
  );
}
