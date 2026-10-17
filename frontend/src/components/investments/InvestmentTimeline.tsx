import { Skeleton } from "antd";

import type { InvestmentTransaction } from "../../api/contracts";
import { formatCompactDay } from "../../presentation/dates";
import { movementTypeLabel, quotaCount } from "../../presentation/investmentLabels";
import { UnavailableState } from "../AsyncState";
import { EmptyState, ListRow, Section } from "../layout";
import { Money } from "../shared/Money";

/**
 * The provider's movements as flat rows: type 500 with the date and cotas as
 * the one meta line, the amount 600 on the right. The amount is a plain
 * figure — an aplicação is money leaving the saldo and a resgate money coming
 * back, so a sign or a colour would read as good or bad news it is not.
 */
export function InvestmentTimeline({
  transactions,
  isLoading,
  error,
  onRetry,
}: {
  transactions: InvestmentTransaction[];
  isLoading: boolean;
  error: unknown;
  onRetry: () => void;
}) {
  const failed = error !== null && error !== undefined;
  return (
    <Section title="Movimentações">
      {isLoading ? (
        <div role="status" aria-label="Carregando movimentações">
          <Skeleton active paragraph={{ rows: 3 }} title={false} />
        </div>
      ) : failed ? (
        <UnavailableState onRetry={onRetry}>
          Não foi possível carregar as movimentações deste investimento.
        </UnavailableState>
      ) : transactions.length === 0 ? (
        <EmptyState title="Nenhuma movimentação sincronizada ainda" />
      ) : (
        <ul className="list-rows" aria-label="Movimentações">
          {transactions.map((transaction) => {
            const date = transaction.occurred_at ?? transaction.trade_date;
            return (
              <ListRow
                key={transaction.id}
                title={movementTypeLabel(transaction.movement_type)}
                meta={[
                  date !== null ? formatCompactDay(date) : "Sem data",
                  transaction.quantity !== null ? quotaCount(transaction.quantity) : null,
                ]
                  .filter(Boolean)
                  .join(" · ")}
                trailing={transaction.amount !== null ? <Money value={transaction.amount} tone="neutral" /> : "—"}
              />
            );
          })}
        </ul>
      )}
    </Section>
  );
}
