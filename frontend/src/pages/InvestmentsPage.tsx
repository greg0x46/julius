import { AimOutlined, BankOutlined, PieChartOutlined, SwapOutlined } from "@ant-design/icons";
import { Alert, Button, Skeleton, Switch } from "antd";
import { useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";

import type {
  Investment,
  InvestmentAccount,
  InvestmentAccountUpdate,
  InvestmentAccountWrite,
  InvestmentOperation,
  InvestmentOperationWrite,
  InvestmentPortfolio,
  InvestmentPortfolioWrite,
  InvestmentPosition,
  InvestmentPositionUpdate,
  InvestmentPositionWrite,
} from "../api/contracts";
import { UnavailableState } from "../components/AsyncState";
import { InvestmentAccountCard } from "../components/investments/InvestmentAccountCard";
import { InvestmentAccountEditForm } from "../components/investments/InvestmentAccountEditForm";
import { InvestmentAccountForm } from "../components/investments/InvestmentAccountForm";
import { InvestmentGoalCard } from "../components/investments/InvestmentGoalCard";
import { InvestmentList } from "../components/investments/InvestmentList";
import { InvestmentOperationForm } from "../components/investments/InvestmentOperationForm";
import { InvestmentPortfolioForm } from "../components/investments/InvestmentPortfolioForm";
import { InvestmentPositionForm } from "../components/investments/InvestmentPositionForm";
import { InvestmentWorkspaceSummary } from "../components/investments/InvestmentWorkspaceSummary";
import { InvestmentsSummary } from "../components/investments/InvestmentsSummary";
import { CreateActionMenu, EmptyState, ListToolbar, Page, PageTabs } from "../components/layout";
import { useFeedback } from "../components/shared/useFeedback";
import { useAccounts } from "../hooks/useAccounts";
import { useInvestmentWorkspace } from "../hooks/useInvestmentWorkspace";
import { useInvestments } from "../hooks/useInvestments";
import { errorMessage } from "../presentation/errors";

type View = "accounts" | "goals" | "synced";

const viewOptions: { label: string; value: View }[] = [
  { label: "Por conta", value: "accounts" },
  { label: "Por objetivo", value: "goals" },
  { label: "Sincronizados", value: "synced" },
];

export function InvestmentsPage() {
  const workspace = useInvestmentWorkspace();
  const financialAccounts = useAccounts();
  const syncedInvestments = useInvestments();
  const navigate = useNavigate();
  const feedback = useFeedback();

  // Synced positions carry no local cost basis; their rendimento, principal
  // and IR/IOF live on the linked provider investment.
  const linkedInvestments = useMemo(
    () => new Map(syncedInvestments.investments.map((investment) => [investment.id, investment])),
    [syncedInvestments.investments],
  );

  const [view, setView] = useState<View>("accounts");
  const [showClosedPositions, setShowClosedPositions] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);

  const [accountFormOpen, setAccountFormOpen] = useState(false);
  const [editingAccount, setEditingAccount] = useState<InvestmentAccount | null>(null);
  const [portfolioFormOpen, setPortfolioFormOpen] = useState(false);
  const [editingPortfolio, setEditingPortfolio] = useState<InvestmentPortfolio | null>(null);
  const [positionFormOpen, setPositionFormOpen] = useState(false);
  const [editingPosition, setEditingPosition] = useState<InvestmentPosition | null>(null);
  // InvestmentPositionForm picks its account from the list it is given, so a
  // card's "Nova posição" hands it only that account instead of a preselection.
  const [positionAccounts, setPositionAccounts] = useState<InvestmentAccount[] | null>(null);
  const [operationFormOpen, setOperationFormOpen] = useState(false);
  const [editingOperation, setEditingOperation] = useState<InvestmentOperation | null>(null);
  const [operationAccountId, setOperationAccountId] = useState<string | null>(null);

  // Manual holdings may sit in a manual account or next to a connection's holdings.
  const holdingAccounts = workspace.accounts.filter((account) => account.active);
  const accountName = (accountId: string) =>
    workspace.accounts.find((account) => account.id === accountId)?.name ?? "Conta não encontrada";
  const summaryAccount = (accountId: string) =>
    workspace.summary?.accounts.find((entry) => entry.account_id === accountId);
  const summaryPortfolio = (portfolioId: string | null) =>
    workspace.summary?.portfolios.find((entry) => entry.portfolio_id === portfolioId);
  // Closed positions (fully sold or redeemed) are hidden by default so the
  // workspace reads as what is actually invested today; the switch below
  // brings them back for anyone reviewing history.
  const visiblePositions = showClosedPositions
    ? workspace.positions
    : workspace.positions.filter((position) => !position.closed);
  const positionsOfAccount = (accountId: string) =>
    visiblePositions.filter((position) => position.account_id === accountId);
  const positionsOfGoal = (portfolioId: string | null) =>
    visiblePositions.filter((position) => position.portfolio_id === portfolioId);
  // The switch only hides rows of the positions tables. Movement history and
  // totals still belong to a closed position: its buys and sells keep their
  // name in the operations table and keep counting in the goal's figures.
  const allPositionsOfAccount = (accountId: string) =>
    workspace.positions.filter((position) => position.account_id === accountId);
  const allPositionsOfGoal = (portfolioId: string | null) =>
    workspace.positions.filter((position) => position.portfolio_id === portfolioId);
  const operationsOfAccount = (accountId: string) =>
    workspace.operations.filter((operation) => operation.account_id === accountId);
  const operationsOfPositions = (positions: InvestmentPosition[]) => {
    const ids = new Set(positions.map((position) => position.id));
    return workspace.operations.filter((operation) => operation.position_id !== null && ids.has(operation.position_id));
  };

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
  const openPortfolio = (portfolio: InvestmentPortfolio | null) => {
    setSaveError(null);
    setEditingPortfolio(portfolio);
    setPortfolioFormOpen(true);
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
  const submitPortfolio = (write: InvestmentPortfolioWrite) =>
    void save(
      () =>
        editingPortfolio
          ? workspace.updatePortfolio({ portfolioId: editingPortfolio.id, write })
          : workspace.createPortfolio(write),
      () => setPortfolioFormOpen(false),
      "Não foi possível salvar o objetivo.",
    );
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

  /**
   * Changing a goal rewrites only the grouping. The position keeps its account,
   * its quantity and its value, so nothing about the money moves.
   */
  const assignGoal = (position: InvestmentPosition, portfolioId: string | null) =>
    void run(
      () =>
        workspace.updatePosition({
          positionId: position.id,
          write: {
            name: position.name,
            ticker: position.ticker,
            asset_type: position.asset_type,
            portfolio_id: portfolioId,
            notes: position.notes,
          },
        }),
      "Não foi possível mudar o objetivo desta posição.",
      "Objetivo alterado",
    );

  const busy = workspace.isSaving || workspace.isDeleting;
  const goalCards: { portfolio: InvestmentPortfolio | null }[] = [
    ...workspace.portfolios.map((portfolio) => ({ portfolio })),
    { portfolio: null },
  ];

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
          key: "goal",
          label: "Novo objetivo",
          description: "Agrupe posições por finalidade",
          icon: <AimOutlined aria-hidden="true" />,
          onClick: () => openPortfolio(null),
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
      description="Contas de investimento, objetivos e o saldo para investir"
      compactMobileHeader
      actions={createMenu}
      tabs={<PageTabs label="Visão dos investimentos" options={viewOptions} value={view} onChange={setView} />}
    >
      {workspace.error && !workspace.isLoading && view !== "synced" && (
        <UnavailableState onRetry={() => void workspace.refetch()}>
          Não foi possível carregar os investimentos
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

      {workspace.isLoading ? (
        <div className="section" role="status" aria-label="Carregando investimentos">
          <div className="section-body">
            <Skeleton active paragraph={{ rows: 5 }} />
          </div>
        </div>
      ) : workspace.error && view !== "synced" ? null : (
        // A failed load shows only the retry above: zeroed totals and "Nenhuma
        // conta ainda" beside it would claim there is nothing there.
        <>
          {workspace.positions.some((position) => position.currency_code !== "BRL") && (
            <Alert type="info" showIcon style={{ marginBottom: 16 }} message="Totais em reais"
              description="Posições em outras moedas são exibidas na moeda original e ficam fora dos totais e metas em reais. Esta versão não faz conversão de câmbio." />
          )}
          {/* Each view's hero is its own scope: the workspace total for the
              account and goal views, the imported investments' total for the
              synced one. Two heroes on one screen would compete. */}
          {view === "synced" ? (
            !syncedInvestments.isLoading &&
            syncedInvestments.investments.length > 0 && <InvestmentsSummary investments={syncedInvestments.investments} />
          ) : (
            <InvestmentWorkspaceSummary
              summary={workspace.summary}
              positions={workspace.positions}
              operations={workspace.operations}
              linked={linkedInvestments}
            />
          )}

          {view !== "synced" && (
            <ListToolbar
              label="Controles das posições"
              end={
                <label className="list-toolbar-switch">
                  <Switch size="small" checked={showClosedPositions} onChange={setShowClosedPositions} />
                  <span>Mostrar posições fechadas</span>
                </label>
              }
            />
          )}

          {view === "goals" && (
            <p className="investments-hint">
              Um objetivo só agrupa posições: não soma ao total e mudar o objetivo de uma posição não move dinheiro.
            </p>
          )}

          {view === "accounts" &&
            (workspace.accounts.length === 0 ? (
              <div className="section">
                <EmptyState
                  title="Nenhuma conta de investimento ainda"
                  hint="Crie uma conta manual ou conecte uma instituição."
                  action={<Button onClick={openAccountCreate}>Nova conta de investimento</Button>}
                />
              </div>
            ) : (
              <div className="investments-list">
                {workspace.accounts.map((account) => (
                  <InvestmentAccountCard
                    key={account.id}
                    account={account}
                    summary={summaryAccount(account.id)}
                    positions={positionsOfAccount(account.id)}
                    allPositions={allPositionsOfAccount(account.id)}
                    operations={operationsOfAccount(account.id)}
                    portfolios={workspace.portfolios}
                    linked={linkedInvestments}
                    onRename={openAccountEdit}
                    onRemove={(target) =>
                      void run(() => workspace.deleteAccount(target.id), "Não foi possível excluir a conta.", "Excluído")
                    }
                    onNewPosition={(target) => openPositionCreate([target])}
                    onNewOperation={(target) => openOperationCreate(target.id)}
                    onEditPosition={openPositionEdit}
                    onRemovePosition={(position) =>
                      void run(
                        () => workspace.deletePosition(position.id),
                        "Não foi possível excluir a posição.",
                        "Excluído",
                      )
                    }
                    onEditOperation={openOperationEdit}
                    onRemoveOperation={(operation) =>
                      void run(
                        () => workspace.deleteOperation(operation.id),
                        "Não foi possível excluir a movimentação.",
                        "Excluído",
                      )
                    }
                    onAssignGoal={assignGoal}
                    busy={busy}
                  />
                ))}
              </div>
            ))}

          {view === "goals" && (
            <div className="investments-list">
              {goalCards.map(({ portfolio }) => {
                const positions = positionsOfGoal(portfolio?.id ?? null);
                // The "sem objetivo" bucket only earns a section when it has
                // something in it; an empty one would just be noise.
                if (portfolio === null && positions.length === 0) return null;
                return (
                  <InvestmentGoalCard
                    key={portfolio?.id ?? "sem-objetivo"}
                    portfolio={portfolio}
                    summary={summaryPortfolio(portfolio?.id ?? null)}
                    positions={positions}
                    operations={operationsOfPositions(allPositionsOfGoal(portfolio?.id ?? null))}
                    portfolios={workspace.portfolios}
                    linked={linkedInvestments}
                    accountNameOf={accountName}
                    onEdit={openPortfolio}
                    onRemove={(target) =>
                      void run(
                        () => workspace.deletePortfolio(target.id),
                        "Não foi possível excluir o objetivo.",
                        "Excluído",
                      )
                    }
                    onAssignGoal={assignGoal}
                    busy={busy}
                  />
                );
              })}
            </div>
          )}

          {view === "synced" && (
            <>
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
                Estes ativos também aparecem como posições dentro da conta integrada correspondente. Aqui fica o
                detalhe importado, com o histórico do provedor.
              </p>
            </>
          )}

          {view !== "goals" && (
            <p className="investments-footnote">
              <Button type="link" size="small" onClick={() => navigate("/transacoes?period=all")}>
                Revisar transações antigas
              </Button>
            </p>
          )}
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
      <InvestmentPortfolioForm
        open={portfolioFormOpen}
        portfolio={editingPortfolio}
        submitting={workspace.isSaving}
        submitError={saveError}
        onSubmit={submitPortfolio}
        onCancel={() => setPortfolioFormOpen(false)}
      />
      <InvestmentPositionForm
        open={positionFormOpen}
        position={editingPosition}
        accounts={positionAccounts ?? workspace.accounts}
        positions={workspace.positions}
        portfolios={workspace.portfolios}
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
