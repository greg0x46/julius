import { Alert, Skeleton } from "../ui";
import { Link } from "react-router-dom";

import type { TransactionInclusionState, TransactionItem } from "../../api/contracts";
import { EmptyState, Section } from "../layout";
import { useRowLayout } from "../transactions/rowLayout";
import { TransactionRow } from "../transactions/TransactionRow";

/**
 * Recent activity on this account, reusing the exact same TransactionRow as
 * the Transações list — same description/amount/category/meta hierarchy,
 * same click-to-open panel — rather than a page-specific row. The account is
 * the page's subject, so rows leave it out of their meta line. Deep
 * filtering and pagination still live on the full list; "Ver todas" links
 * there pre-filtered to this account.
 */
export function AccountRecentTransactions({
  transactions,
  isLoading,
  error,
  accountId,
  selectedId,
  onSelect,
  onInclusion,
  pendingTransactionId,
}: {
  transactions: TransactionItem[];
  isLoading: boolean;
  error: unknown;
  accountId: string;
  selectedId: string | null;
  onSelect: (id: string) => void;
  onInclusion?: (id: string, state: TransactionInclusionState) => void;
  pendingTransactionId?: string | null;
}) {
  const layout = useRowLayout();
  return (
    <Section
      title="Últimas transações"
      className="account-recent-transactions"
      trailing={<Link className="touch-link" to={`/transacoes?account_ids=${encodeURIComponent(accountId)}`}>
          Ver todas
        </Link>}
    >
      {error !== null && error !== undefined && (
        <Alert type="error" showIcon message="Não foi possível carregar as transações desta conta." />
      )}
      {isLoading ? (
        <div role="status" aria-label="Carregando últimas transações">
          <Skeleton active paragraph={{ rows: 3 }} title={false} />
        </div>
      ) : transactions.length === 0 ? (
        <EmptyState
          title="Nenhuma transação nesta conta"
          hint="As transações sincronizadas ou importadas aparecem aqui."
        />
      ) : (
        <div className="transaction-list">
          {transactions.map((item) => (
            <TransactionRow
              key={item.id}
              item={item}
              selected={item.id === selectedId}
              showAccount={false}
              layout={layout}
              onSelect={onSelect}
              onInclusion={onInclusion}
              inclusionPending={pendingTransactionId === item.id}
            />
          ))}
        </div>
      )}
    </Section>
  );
}
