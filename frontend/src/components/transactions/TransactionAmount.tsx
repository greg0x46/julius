import type { TransactionItem } from "../../api/contracts";
import { Money } from "../shared/Money";

/**
 * The signed effective amount, tinted only when money comes in (tone "flow"); the
 * anchor of every row scan. `size` picks the row figure or the panel's hero.
 * `amount-<classification>` stays on the element because other screens style
 * and query it.
 */
export function TransactionAmount({
  item,
  className = "",
  size = "row",
}: {
  item: TransactionItem;
  className?: string;
  size?: "row" | "hero";
}) {
  const classes = `transaction-amount amount-${item.classification} ${className}`;
  if (!item.effective_money) {
    return <span className={classes}>Valor indisponível</span>;
  }
  return (
    <Money
      value={item.effective_money.value}
      currencyCode={item.effective_money.currency_code}
      tone="flow"
      size={size}
      direction={item.classification}
      className={classes}
    />
  );
}
