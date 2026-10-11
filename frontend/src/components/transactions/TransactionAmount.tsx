import type { TransactionItem } from "../../api/contracts";
import { formatMoney } from "../../presentation/money";
import { Money } from "../shared/Money";

/**
 * The signed BRL amount, tinted only when money comes in (tone "flow"); the
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
  if (item.effective_money.currency_code !== "BRL") return <span className={`${classes} money money-size-${size}`}>{formatMoney(item.effective_money.value, item.effective_money.currency_code)}</span>;
  return (
    <Money
      value={item.effective_money.value}
      tone="flow"
      size={size}
      direction={item.classification}
      className={classes}
    />
  );
}
