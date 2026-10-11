import {
  formatToneMoney,
  moneyTone,
  type MoneyDirection,
  type MoneyToneKind,
} from "../../presentation/money";

interface MoneyProps {
  currencyCode?: string;
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
export function Money({ value, currencyCode = "BRL", tone, size, direction, className = "" }: MoneyProps) {
  const { color } = moneyTone(value, tone, direction);
  const classes = ["money", `money-${color}`, size ? `money-size-${size}` : "", className]
    .filter(Boolean)
    .join(" ");
  return <span className={classes}>{formatToneMoney(value, currencyCode, tone, direction)}</span>;
}
