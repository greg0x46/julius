import { Popover } from "antd";
import type { ReactNode } from "react";

import { moneyTone } from "../../presentation/money";
import { Money } from "../shared/Money";
import { formatPercent } from "./investmentMath";

export type InvestmentFigure = {
  label: string;
  value: ReactNode;
  hint?: string;
};

/**
 * The account and the goal views answer the same five questions, so they share
 * one strip instead of drifting into two slightly different vocabularies. A
 * figure's explanation opens on tap or hover (and from the keyboard) from its
 * underlined label — a hover-only tooltip would be unreachable on a phone.
 */
export function InvestmentFigures({ figures }: { figures: InvestmentFigure[] }) {
  return (
    <dl className="investment-figures">
      {figures.map((figure) => (
        <div key={figure.label} className="investment-figure">
          <dt>
            {figure.hint ? (
              <Popover content={figure.hint} trigger={["hover", "click"]} placement="bottomLeft">
                <button type="button" className="investment-figure-hint">
                  {figure.label}
                </button>
              </Popover>
            ) : (
              figure.label
            )}
          </dt>
          <dd>{figure.value}</dd>
        </div>
      ))}
    </dl>
  );
}

/**
 * Rendimento in R$ with its % beside it. The amount goes through the app's
 * money rule for a result (explicit +/−, never colour alone), and the % takes
 * the same tone so the two read as one figure.
 */
export function InvestmentYieldValue({
  gain,
  percent,
  currencyCode,
}: {
  gain: string;
  percent: number | null;
  currencyCode?: string;
}) {
  const { color } = moneyTone(gain, "result");
  return (
    <span className="investment-yield">
      <Money report={currencyCode !== undefined} currencyCode={currencyCode} value={gain} tone="result" />
      {percent !== null && (
        <span className={`investment-yield-percent money-${color}`}>{formatPercent(percent)}</span>
      )}
    </span>
  );
}
