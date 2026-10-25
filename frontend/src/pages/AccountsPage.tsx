import { PlusOutlined, UploadOutlined } from "../components/ui/icons";
import { Alert, Button } from "../components/ui";
import { useState } from "react";
import { useNavigate } from "react-router-dom";

import type { Account } from "../api/contracts";
import { ManualAccountForm } from "../components/accounts/ManualAccountForm";
import { AccountsSummary } from "../components/accounts/AccountsSummary";
import { BankAccountList } from "../components/accounts/BankAccountList";
import { CreditCardList } from "../components/accounts/CreditCardList";
import { EmptyState, Page, PageAction, Section } from "../components/layout";
import { useAccounts } from "../hooks/useAccounts";

export function AccountsPage() {
  const [creating, setCreating] = useState(false);
  const accounts = useAccounts();
  const navigate = useNavigate();

  const openDetail = (account: Account) => navigate(`/contas-e-cartoes/${account.id}`);
  // Accounts with no type at all are bank accounts for display purposes:
  // only "CREDIT" gets the card treatment, since the limit and invoice
  // columns are meaningless without it.
  const bank = accounts.accounts.filter((account) => account.account_type !== "CREDIT");
  const credit = accounts.accounts.filter((account) => account.account_type === "CREDIT");
  // A failed load with nothing to show is not "no accounts": it only says it failed.
  const loadFailed = accounts.error !== null && accounts.accounts.length === 0;
  const hasNoAccounts = !accounts.isLoading && !loadFailed && accounts.accounts.length === 0;

  const importAction = (
    <PageAction
      icon={<UploadOutlined aria-hidden="true" />}
      label="Importar extrato"
      shortLabel="Importar"
      onClick={() => navigate("/contas-e-cartoes/importar")}
    />
  );

  return (
    <Page
      title="Contas e cartões"
      description="Saldos e detalhes das suas contas bancárias e cartões"
      actions={<><PageAction icon={<PlusOutlined aria-hidden="true" />} label="Nova conta" shortLabel="Nova" onClick={() => setCreating(true)} />{importAction}</>}
      className="accounts-page"
      width="narrow"
      compactMobileHeader
    >
      {creating && <ManualAccountForm onClose={() => setCreating(false)} onSaved={(account) => { setCreating(false); openDetail(account); }} />}
      {accounts.error && (
        <Alert
          type="error"
          showIcon
          message="Não foi possível carregar as contas"
          action={<Button onClick={() => accounts.refetch()}>Tentar novamente</Button>}
          style={{ marginBottom: 16 }}
        />
      )}
      {hasNoAccounts ? (
        // One block instead of an empty "Contas bancárias" and an empty
        // "Cartões de crédito" stacked: with nothing at all, say it once.
        <Section title="Contas">
          <EmptyState
            title="Nenhuma conta ainda"
            hint="Crie uma conta manual, importe um extrato ou conecte um banco em Configurações."
            action={<Button onClick={() => setCreating(true)}>Nova conta</Button>}
          />
        </Section>
      ) : (
        <div className="accounts-page-sections">
          {!accounts.isLoading && accounts.accounts.length > 0 && <AccountsSummary accounts={accounts.accounts} />}
          <BankAccountList accounts={bank} isLoading={accounts.isLoading} failed={loadFailed} onOpen={openDetail} />
          <CreditCardList accounts={credit} isLoading={accounts.isLoading} failed={loadFailed} onOpen={openDetail} />
        </div>
      )}
    </Page>
  );
}
