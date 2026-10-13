import {
  formatToneMoney,
  moneyTone,
  type MoneyDirection,
  type MoneyToneKind,
} from "../../presentation/money";

import { useReportCurrency } from "../../hooks/useReportCurrency";

interface MoneyProps {
  currencyCode?: string;
  /** Decimal string in its valuation currency, as the API sends it (e.g. "-1234.50"). */
  value: string;
  /** How the figure reads — see `moneyTone` for the rule behind each kind. */
  tone: MoneyToneKind;
  /** Row text size (15px/600) or the page's single hero figure. Default: inherit. */
  size?: "row" | "hero";
  /** For `flow`: the direction when `value` is an unsigned magnitude. */
  direction?: MoneyDirection;
  className?: string;
}

/**
 * An amount with the app's one money display rule: tabular figures, and
 * sign and colour decided by `tone` (never red for an ordinary expense).
 */
export function Money({ value, tone, size, direction, currencyCode, className = "" }: MoneyProps) {
  const reportCurrency = useReportCurrency();
  const { color } = moneyTone(value, tone, direction);
  const classes = ["money", `money-${color}`, size ? `money-size-${size}` : "", className]
    .filter(Boolean)
    .join(" ");
  return (
    <span className={classes}>{formatToneMoney(value, tone, currencyCode ?? reportCurrency, direction)}</span>
  );
}
