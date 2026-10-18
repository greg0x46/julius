import { ApiError } from "../api/problems";
import { BankOutlined, PieChartOutlined, SwapOutlined } from "@ant-design/icons";
import { Alert, Button, Skeleton } from "antd";
import { useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";

import type {
  Investment,
  InvestmentPeriodGroup,
  InvestmentAccount,
  InvestmentAccountUpdate,
  InvestmentAccountWrite,
  InvestmentOperation,
  InvestmentOperationWrite,
  InvestmentPosition,
  InvestmentPositionUpdate,
  InvestmentPositionWrite,
} from "../api/contracts";
import { UnavailableState } from "../components/AsyncState";
import { InvestmentAccountEditForm } from "../components/investments/InvestmentAccountEditForm";
import { InvestmentAccountForm } from "../components/investments/InvestmentAccountForm";
import { InvestmentList } from "../components/investments/InvestmentList";
import { InvestmentOperationForm } from "../components/investments/InvestmentOperationForm";
import { InvestmentPositionForm } from "../components/investments/InvestmentPositionForm";
import { InvestmentAccountsSection } from "../components/investments/InvestmentAccountsSection";
import { InvestmentPeriodView } from "../components/investments/InvestmentPeriodView";
import { InvestmentsSummary } from "../components/investments/InvestmentsSummary";
import { useInvestmentMenus } from "../components/investments/useInvestmentMenus";
import { PeriodNavigator } from "../components/filters/PeriodNavigator";
import { periodPresets } from "../components/filters/periodPresets";
import { CreateActionMenu, Page, PageTabs } from "../components/layout";
import { useFeedback } from "../components/shared/useFeedback";
import { useAccounts } from "../hooks/useAccounts";
import { useInvestmentWorkspace } from "../hooks/useInvestmentWorkspace";
import { useInvestments } from "../hooks/useInvestments";
import { usePeriod } from "../hooks/usePeriod";
import { useToday } from "../hooks/useToday";
import { errorMessage } from "../presentation/errors";
import { periodRequest } from "../presentation/investmentPeriod";

type View = "portfolio" | "synced";

const viewOptions: { label: string; value: View }[] = [
  { label: "Carteira", value: "portfolio" },
  { label: "Sincronizados", value: "synced" },
];

export function InvestmentsPage() {
  const workspace = useInvestmentWorkspace();
  const financialAccounts = useAccounts();
  const syncedInvestments = useInvestments();
  const navigate = useNavigate();
  const feedback = useFeedback();
  const { period, setPeriod } = usePeriod();
  const today = useToday().format("YYYY-MM-DD");

  const [view, setView] = useState<View>("portfolio");
  const [actionError, setActionError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);

  const [accountFormOpen, setAccountFormOpen] = useState(false);
  const [editingAccount, setEditingAccount] = useState<InvestmentAccount | null>(null);
  const [positionFormOpen, setPositionFormOpen] = useState(false);
  const [editingPosition, setEditingPosition] = useState<InvestmentPosition | null>(null);
  // InvestmentPositionForm picks its account from the list it is given, so an
  // account's "Nova posição" hands it only that account instead of a preselection.
  const [positionAccounts, setPositionAccounts] = useState<InvestmentAccount[] | null>(null);
  const [operationFormOpen, setOperationFormOpen] = useState(false);
  const [editingOperation, setEditingOperation] = useState<InvestmentOperation | null>(null);
  const [operationAccountId, setOperationAccountId] = useState<string | null>(null);

  // Manual holdings may sit in a manual account or next to a connection's holdings.
  const holdingAccounts = workspace.accounts.filter((account) => account.active);
  const request = useMemo(() => periodRequest(period, today), [period, today]);

  const run = async (action: () => Promise<unknown>, fallback: string, done?: string) => {
    setActionError(null);
    try {
      await action();
      if (done) feedback.success(done);
      return true;
    } catch (error) {
      // Inline, where the list is — and a toast too: the Alert sits at the top
      // of the page, out of sight when the action came from a row far below.
      const message = errorMessage(error, fallback);
      setActionError(message);
      feedback.error(message);
      return false;
    }
  };

  const save = async (action: () => Promise<unknown>, close: () => void, fallback: string) => {
    setSaveError(null);
    try {
      await action();
      close();
      feedback.success("Salvo");
    } catch (error) {
      setSaveError(errorMessage(error, fallback));
    }
  };

  const openAccountCreate = () => {
    setSaveError(null);
    setAccountFormOpen(true);
  };
  const openAccountEdit = (account: InvestmentAccount) => {
    setSaveError(null);
    setEditingAccount(account);
  };
  const openPositionCreate = (accounts: InvestmentAccount[] | null) => {
    setSaveError(null);
    setEditingPosition(null);
    setPositionAccounts(accounts);
    setPositionFormOpen(true);
  };
  const openPositionEdit = (position: InvestmentPosition) => {
    setSaveError(null);
    setEditingPosition(position);
    setPositionAccounts(null);
    setPositionFormOpen(true);
  };
  const openOperationCreate = (accountId: string | null) => {
    setSaveError(null);
    setEditingOperation(null);
    setOperationAccountId(accountId);
    setOperationFormOpen(true);
  };
  const openOperationEdit = (operation: InvestmentOperation) => {
    setSaveError(null);
    setEditingOperation(operation);
    setOperationAccountId(operation.account_id);
    setOperationFormOpen(true);
  };

  const submitAccount = (write: InvestmentAccountWrite) =>
    void save(() => workspace.createAccount(write), () => setAccountFormOpen(false), "Não foi possível criar a conta.");
  const submitAccountEdit = (write: InvestmentAccountUpdate) => {
    if (!editingAccount) return;
    void save(
      () => workspace.updateAccount({ accountId: editingAccount.id, write }),
      () => setEditingAccount(null),
      "Não foi possível salvar a conta.",
    );
  };
  const submitPositionCreate = (write: InvestmentPositionWrite) =>
    void save(
      () => workspace.createPosition(write),
      () => setPositionFormOpen(false),
      "Não foi possível criar a posição.",
    );
  const submitPositionUpdate = (write: InvestmentPositionUpdate) => {
    if (!editingPosition) return;
    void save(
      () => workspace.updatePosition({ positionId: editingPosition.id, write }),
      () => setPositionFormOpen(false),
      "Não foi possível salvar a posição.",
    );
  };
  const submitOperation = (write: InvestmentOperationWrite) =>
    void save(
      () =>
        editingOperation
          ? workspace.updateOperation({ operationId: editingOperation.id, write })
          : workspace.createOperation(write),
      () => setOperationFormOpen(false),
      "Não foi possível salvar a movimentação.",
    );

  const busy = workspace.isSaving || workspace.isDeleting;

  const menus = useInvestmentMenus({
    busy,
    onRename: openAccountEdit,
    onRemove: (account) =>
      void run(() => workspace.deleteAccount(account.id), "Não foi possível excluir a conta.", "Excluído"),
    onNewPosition: (account) => openPositionCreate([account]),
    onNewOperation: (account) => openOperationCreate(account.id),
    onEditPosition: openPositionEdit,
    onRemovePosition: (position) =>
      void run(() => workspace.deletePosition(position.id), "Não foi possível excluir a posição.", "Excluído"),
  });
  const accountOfGroup = (group: InvestmentPeriodGroup) =>
    group.account_id === null ? undefined : workspace.accounts.find((account) => account.id === group.account_id);

  const createMenu = (
    <CreateActionMenu
      label="Adicionar"
      options={[
        {
          key: "operation",
          label: "Registrar movimentação",
          description:
            workspace.accounts.length === 0
              ? "Cadastre uma conta de investimento primeiro"
              : "Aporte, resgate ou outra movimentação em uma conta",
          icon: <SwapOutlined aria-hidden="true" />,
          disabled: workspace.accounts.length === 0,
          onClick: () => openOperationCreate(null),
        },
        {
          key: "position",
          label: "Nova posição",
          description:
            holdingAccounts.length === 0
              ? "Cadastre uma conta de investimento primeiro"
              : "Um ativo dentro de uma conta de investimento",
          icon: <PieChartOutlined aria-hidden="true" />,
          disabled: holdingAccounts.length === 0,
          onClick: () => openPositionCreate(null),
        },
        {
          key: "account",
          label: "Nova conta de investimento",
          description: "Onde suas posições ficam guardadas",
          icon: <BankOutlined aria-hidden="true" />,
          onClick: openAccountCreate,
        },
      ]}
    />
  );

  return (
    <Page
      title="Investimentos"
      description="Patrimônio e rendimento da carteira no período"
      compactMobileHeader
      context={
        view === "portfolio" ? (
          <PeriodNavigator
            id="investments-period"
            value={[period.from, period.to]}
            presets={periodPresets()}
            onChange={setPeriod}
            reset={{ preset: "this-month", label: "Este mês" }}
            bare
          />
        ) : undefined
      }
      actions={createMenu}
      tabs={<PageTabs label="Visão dos investimentos" options={viewOptions} value={view} onChange={setView} />}
    >
      {workspace.error && !workspace.isLoading && (
        <UnavailableState onRetry={() => void workspace.refetch()}>
          {workspace.error instanceof ApiError ? workspace.error.message : "Não foi possível carregar os investimentos"}
        </UnavailableState>
      )}
      {actionError && (
        <Alert
          type="error"
          showIcon
          closable
          onClose={() => setActionError(null)}
          message={actionError}
          style={{ marginBottom: 16 }}
        />
      )}

      {view === "portfolio" && (
        <>
          <InvestmentPeriodView
            request={request}
            workspacePositions={workspace.positions}
            onNewPosition={() => (holdingAccounts.length === 0 ? openAccountCreate() : openPositionCreate(null))}
            positionMenu={menus.positionMenu}
            groupMenu={(group) => {
              const account = accountOfGroup(group);
              return account === undefined ? [] : menus.accountMenu(account);
            }}
          />
          {workspace.isLoading ? (
            <div className="section" role="status" aria-label="Carregando contas">
              <div className="section-body">
                <Skeleton active paragraph={{ rows: 2 }} />
              </div>
            </div>
          ) : workspace.error ? null : (
            // A failed load shows only the retry above: "Nenhuma conta ainda"
            // beside it would claim there is nothing there.
            <InvestmentAccountsSection
              accounts={workspace.accounts}
              positions={workspace.positions}
              operations={workspace.operations}
              menuOf={menus.accountMenu}
              onNewAccount={openAccountCreate}
              onEditOperation={openOperationEdit}
              onRemoveOperation={(operation) =>
                void run(
                  () => workspace.deleteOperation(operation.id),
                  "Não foi possível excluir a movimentação.",
                  "Excluído",
                )
              }
              busy={busy}
            />
          )}
        </>
      )}

      {view === "synced" && (
        <>
          {!syncedInvestments.isLoading && syncedInvestments.investments.length > 0 && (
            <InvestmentsSummary
              valuation={workspace.summary ?? undefined}
              investments={syncedInvestments.investments}
            />
          )}
          {syncedInvestments.error && (
            <Alert
              type="error"
              showIcon
              message="Não foi possível carregar os investimentos sincronizados"
              description={<Button onClick={() => syncedInvestments.refetch()}>Tentar novamente</Button>}
              style={{ marginBottom: 16 }}
            />
          )}
          {!syncedInvestments.error && (
            <InvestmentList
              investments={syncedInvestments.investments}
              isLoading={syncedInvestments.isLoading}
              onOpen={(investment: Investment) => navigate(`/investimentos/${investment.id}`)}
            />
          )}
          <p className="investments-hint">
            Estes ativos também aparecem como posições dentro da conta integrada correspondente. Aqui fica o detalhe
            importado, com o histórico do provedor. Esta visão não segue o período escolhido.
          </p>
        </>
      )}

      <InvestmentAccountForm
        open={accountFormOpen}
        financialAccounts={financialAccounts.accounts}
        submitting={workspace.isSaving}
        submitError={saveError}
        onSubmit={submitAccount}
        onCancel={() => setAccountFormOpen(false)}
      />
      <InvestmentAccountEditForm
        open={editingAccount !== null}
        account={editingAccount}
        financialAccounts={financialAccounts.accounts}
        submitting={workspace.isSaving}
        submitError={saveError}
        onSubmit={submitAccountEdit}
        onCancel={() => setEditingAccount(null)}
      />
      <InvestmentPositionForm
        open={positionFormOpen}
        position={editingPosition}
        accounts={positionAccounts ?? workspace.accounts}
        submitting={workspace.isSaving}
        submitError={saveError}
        onCreate={submitPositionCreate}
        onUpdate={submitPositionUpdate}
        onCancel={() => setPositionFormOpen(false)}
      />
      <InvestmentOperationForm
        open={operationFormOpen}
        operation={editingOperation}
        initial={operationAccountId === null ? null : { account_id: operationAccountId }}
        accounts={workspace.accounts}
        positions={workspace.positions}
        submitting={workspace.isSaving}
        submitError={saveError}
        onSubmit={submitOperation}
        onSubmitCompound={(operations) => void save(() => workspace.createOperations({ operations }), () => setOperationFormOpen(false), "Não foi possível salvar a movimentação.")}
        onCancel={() => setOperationFormOpen(false)}
      />
    </Page>
  );
}
