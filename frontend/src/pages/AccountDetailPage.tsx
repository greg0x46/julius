import { Skeleton } from "antd";
import { useParams } from "react-router-dom";

import { isUuid } from "../api/contracts";
import { ManualAccountActions } from "../components/accounts/ManualAccountActions";
import { AccountBillsTable } from "../components/accounts/AccountBillsTable";
import { AccountCardsTable } from "../components/accounts/AccountCardsTable";
import { AccountSummary } from "../components/accounts/AccountSummary";
import { AccountRecentTransactions } from "../components/accounts/AccountRecentTransactions";
import { DetailPage, InvalidDetailPage, Page } from "../components/layout";
import { TransactionPanel } from "../components/transactions/TransactionPanel";
import { useRowFocusReturn } from "../components/transactions/useRowFocusReturn";
import { useSelectedTransaction } from "../components/transactions/useSelectedTransaction";
import { useTransactionPanelWrites } from "../components/transactions/useTransactionPanelWrites";
import { useAccountDetail } from "../hooks/useAccountDetail";
import { useCategories } from "../hooks/useCategories";
import { accountHeaderTitle } from "../presentation/accountLabels";

const pageTitle = "Conta";
const backTo = "/contas-e-cartoes";
const backLabel = "Voltar para contas e cartões";

/**
 * The shape of the loaded page — a summary block, then a list — so the
 * content does not jump when the account arrives.
 */
function AccountDetailSkeleton() {
  return (
    <div role="status" aria-label="Carregando conta">
      <Skeleton active title={{ width: "35%" }} paragraph={{ rows: 2 }} />
      <Skeleton active title={{ width: "25%" }} paragraph={{ rows: 4 }} />
    </div>
  );
}

function ValidAccountDetail({ id }: { id: string }) {
  const account = useAccountDetail(id);

  // The recent-transactions panel reuses the exact same interaction the
  // Transações page offers — open a row, change its category/inclusion, edit
  // or delete a manual entry — through the same shared wiring, so a decision
  // made here and one made there never disagree, and a failed write is shown
  // the same way (inside the panel, with a retry).
  const categories = useCategories();
  const focusReturn = useRowFocusReturn();
  const { selected, select, patch } = useSelectedTransaction(account.transactions);
  const writes = useTransactionPanelWrites({
    categories: categories.categories,
    onDeleted: () => select(null),
    onConfirmed: patch,
    panelOpen: selected !== null,
  });

  const selectTransaction = (transactionId: string | null) => {
    writes.clearDeleteError();
    if (transactionId !== null) focusReturn.remember(transactionId);
    select(transactionId);
  };
  // Esc / × / ← on the panel: back to the row that opened it.
  const dismissPanel = () => {
    selectTransaction(null);
    focusReturn.restore();
  };

  if (account.state.freshness === "loading") {
    return (
      <Page
        className="accounts-page"
        title={pageTitle}
        backTo={backTo}
        backLabel={backLabel}
        compactMobileHeader
      >
        <AccountDetailSkeleton />
      </Page>
    );
  }

  return (
    <DetailPage
      className="accounts-page"
      title={pageTitle}
      backTo={backTo}
      backLabel={backLabel}
      recordTitle={accountHeaderTitle}
      state={account.state}
      retry={account.retry}
      notFoundMessage="Conta não encontrada"
      notFoundDescription="Não existe uma conta com este identificador."
      unavailableMessage="Não foi possível consultar esta conta agora."
    >
      {(snapshot) => (
        <>
          <AccountSummary
            account={snapshot}
            onSaveClosingDay={account.setClosingDay}
            savingClosingDay={account.savingClosingDay}
          />

          <ManualAccountActions account={snapshot} />
          {snapshot.account_type === "CREDIT" && (
            <>
              <AccountCardsTable
                cards={account.cards}
                isLoading={account.cardsLoading}
                error={account.cardsError}
              />
              <AccountBillsTable
                bills={account.bills}
                isLoading={account.billsLoading}
                error={account.billsError}
              />
            </>
          )}

          {writes.alerts(selected !== null)}

          <AccountRecentTransactions
            transactions={account.transactions}
            isLoading={account.transactionsLoading}
            error={account.transactionsError}
            accountId={id}
            selectedId={selected?.id ?? null}
            onSelect={selectTransaction}
            onInclusion={(transactionId, target) =>
              writes.inclusion.setInclusion({ transactionId, state: target })
            }
            pendingTransactionId={writes.inclusion.pendingTarget?.transactionId}
          />

          <TransactionPanel
            item={selected}
            categories={categories.categories}
            onClose={dismissPanel}
            {...writes.panelProps(selected)}
          />
        </>
      )}
    </DetailPage>
  );
}

export function AccountDetailPage() {
  const { id = "" } = useParams();
  return isUuid(id) ? (
    <ValidAccountDetail id={id} />
  ) : (
    <InvalidDetailPage
      className="accounts-page"
      title={pageTitle}
      backTo={backTo}
      backLabel={backLabel}
      invalidTitle="Endereço de conta inválido"
    />
  );
}
