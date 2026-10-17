import {
  formatToneMoney,
  formatToneReportMoney,
  moneyTone,
  type MoneyDirection,
  type MoneyToneKind,
} from "../../presentation/money";
import { useReportCurrency } from "../../hooks/useReportCurrency";

interface MoneyProps {
  currencyCode?: string;
  value: string;
  /** How the figure reads — see `moneyTone` for the rule behind each kind. */
  tone: MoneyToneKind;
  /** Row text size (15px/600) or the page's single hero figure. Default: inherit. */
  size?: "row" | "hero";
  /** For `flow`: the direction when `value` is an unsigned magnitude. */
  direction?: MoneyDirection;
  /** Consolidated figure: two decimals in the reference currency. Defaults to true when no `currencyCode` is given. */
  report?: boolean;
  className?: string;
}

/**
 * An amount with the app's one money display rule: tabular figures, and
 * sign and colour decided by `tone` (never red for an ordinary expense).
 */
export function Money({ value, currencyCode, tone, size, direction, report, className = "" }: MoneyProps) {
  const reportCurrency = useReportCurrency();
  const { color } = moneyTone(value, tone, direction);
  const classes = ["money", `money-${color}`, size ? `money-size-${size}` : "", className]
    .filter(Boolean)
    .join(" ");
  return <span className={classes}>{(report ?? currencyCode === undefined) ? formatToneReportMoney(value, currencyCode ?? reportCurrency, tone, direction) : formatToneMoney(value, currencyCode ?? reportCurrency, tone, direction)}</span>;
}
