import { useTranslation } from "../i18n/context";
import { ApiError } from "../api/problems";
import { BankOutlined, PieChartOutlined, SwapOutlined } from "../components/ui/icons";
import { Alert } from "../components/ui";
import { useMemo, useState } from "react";

import type {
  InvestmentPeriodGroup,
  InvestmentPeriodGrouping,
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
import { InvestmentOperationForm } from "../components/investments/InvestmentOperationForm";
import { InvestmentPositionForm } from "../components/investments/InvestmentPositionForm";
import {
  InvestmentAccountsDrawer,
  type InvestmentAccountsScope,
} from "../components/investments/InvestmentAccountsDrawer";
import { InvestmentPeriodView } from "../components/investments/InvestmentPeriodView";
import { useInvestmentMenus } from "../components/investments/useInvestmentMenus";
import { PeriodNavigator } from "../components/filters/PeriodNavigator";
import { CreateActionMenu, Page, PageAction, PageTabs } from "../components/layout";
import { useFeedback } from "../components/shared/useFeedback";
import { useAccounts } from "../hooks/useAccounts";
import { useInvestmentWorkspace } from "../hooks/useInvestmentWorkspace";
import { usePeriod } from "../hooks/usePeriod";
import { useToday } from "../hooks/useToday";
import { errorMessage } from "../presentation/errors";
import { periodRequest } from "../presentation/investmentPeriod";



export function InvestmentsPage() {
  const { t } = useTranslation();
  const groupingOptions = [{ value: "class" as const, label: t("assetClassGrouping") }, { value: "account" as const, label: t("custody") }, { value: "none" as const, label: t("noGrouping") }];
  const workspace = useInvestmentWorkspace();
  const financialAccounts = useAccounts();
  const feedback = useFeedback();
  const { period, setPeriod, presets } = usePeriod();
  const today = useToday().format("YYYY-MM-DD");

  const [groupBy, setGroupBy] = useState<InvestmentPeriodGrouping>("class");
  const [actionError, setActionError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);

  const [accountFormOpen, setAccountFormOpen] = useState(false);
  const [editingAccount, setEditingAccount] = useState<InvestmentAccount | null>(null);
  const [positionFormOpen, setPositionFormOpen] = useState(false);
  const [editingPosition, setEditingPosition] = useState<InvestmentPosition | null>(null);
  const [operationFormOpen, setOperationFormOpen] = useState(false);
  const [editingOperation, setEditingOperation] = useState<InvestmentOperation | null>(null);
  const [operationAccountId, setOperationAccountId] = useState<string | null>(null);
  const [accountsScope, setAccountsScope] = useState<InvestmentAccountsScope | null>(null);

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
      feedback.success(t("saved"));
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
  const openPositionCreate = () => {
    setSaveError(null);
    setEditingPosition(null);
    setPositionFormOpen(true);
  };
  const openPositionEdit = (position: InvestmentPosition) => {
    setSaveError(null);
    setEditingPosition(position);
    setPositionFormOpen(true);
  };
  const openOperationCreate = () => {
    setSaveError(null);
    setEditingOperation(null);
    setOperationAccountId(null);
    setOperationFormOpen(true);
  };
  const openOperationEdit = (operation: InvestmentOperation) => {
    setSaveError(null);
    setEditingOperation(operation);
    setOperationAccountId(operation.account_id);
    setOperationFormOpen(true);
  };

  const submitAccount = (write: InvestmentAccountWrite) =>
    void save(() => workspace.createAccount(write), () => setAccountFormOpen(false), t("accountCreateError"));
  const submitAccountEdit = (write: InvestmentAccountUpdate) => {
    if (!editingAccount) return;
    void save(
      () => workspace.updateAccount({ accountId: editingAccount.id, write }),
      () => setEditingAccount(null),
      t("accountSaveError"),
    );
  };
  const submitPositionCreate = (write: InvestmentPositionWrite) =>
    void save(
      () => workspace.createPosition(write),
      () => setPositionFormOpen(false),
      t("positionCreateError"),
    );
  const submitPositionUpdate = (write: InvestmentPositionUpdate) => {
    if (!editingPosition) return;
    void save(
      () => workspace.updatePosition({ positionId: editingPosition.id, write }),
      () => setPositionFormOpen(false),
      t("positionSaveError"),
    );
  };
  const submitOperation = (write: InvestmentOperationWrite) =>
    void save(
      () =>
        editingOperation
          ? workspace.updateOperation({ operationId: editingOperation.id, write })
          : workspace.createOperation(write),
      () => setOperationFormOpen(false),
      t("operationSaveError"),
    );

  const busy = workspace.isSaving || workspace.isDeleting;
  // A form opened from the accounts drawer replaces it instead of stacking on
  // it; the scope is kept, so the drawer comes back where it was.
  const formOpen = accountFormOpen || editingAccount !== null || positionFormOpen || operationFormOpen;

  const menus = useInvestmentMenus({
    busy,
    onRename: openAccountEdit,
    onRemove: (account) =>
      void run(() => workspace.deleteAccount(account.id), t("accountDeleteError"), t("deleted")),
    onViewOperations: (account) => setAccountsScope({ accountId: account.id, positionId: null }),
    onViewPositionOperations: (position) =>
      setAccountsScope({ accountId: position.account_id, positionId: position.id }),
    onEditPosition: openPositionEdit,
    onRemovePosition: (position) =>
      void run(() => workspace.deletePosition(position.id), t("positionDeleteError"), t("deleted")),
  });
  const accountOfGroup = (group: InvestmentPeriodGroup) =>
    group.account_id === null ? undefined : workspace.accounts.find((account) => account.id === group.account_id);

  const createMenu = (
    <CreateActionMenu
      label={t("add")}
      options={[
        {
          key: "operation",
          label: t("recordOperation"),
          description:
            workspace.accounts.length === 0
              ? t("createInvestmentAccountFirst")
              : t("investmentOperationDescription"),
          icon: <SwapOutlined aria-hidden="true" />,
          disabled: workspace.accounts.length === 0,
          onClick: () => openOperationCreate(),
        },
        {
          key: "position",
          label: t("newPosition"),
          description:
            holdingAccounts.length === 0
              ? t("createInvestmentAccountFirst")
              : t("positionDescription"),
          icon: <PieChartOutlined aria-hidden="true" />,
          disabled: holdingAccounts.length === 0,
          onClick: openPositionCreate,
        },
        {
          key: "account",
          label: t("newInvestmentAccount"),
          description: t("custodyDescription"),
          icon: <BankOutlined aria-hidden="true" />,
          onClick: openAccountCreate,
        },
      ]}
    />
  );

  return (
    <Page
      title={t("investments")}
      description={t("investmentsDescription")}
      compactMobileHeader
      context={
        <PeriodNavigator
          id="investments-period"
          value={[period.from, period.to]}
          presets={presets}
          onChange={setPeriod}
          reset={{ preset: "this-month", label: t("thisMonth") }}
          bare
        />
      }
      actions={
        <>
          {createMenu}
          <PageAction
            icon={<BankOutlined aria-hidden="true" />}
            label={t("accountList")}
            // A failed load would read as "no accounts"; the retry above is the only answer then.
            disabled={workspace.isLoading || Boolean(workspace.error)}
            onClick={() => setAccountsScope({ accountId: null, positionId: null })}
          />
        </>
      }
      tabs={<PageTabs label={t("groupPositionsBy")} options={groupingOptions} value={groupBy} onChange={setGroupBy} />}
    >
      {workspace.error && !workspace.isLoading && (
        <UnavailableState onRetry={() => void workspace.refetch()}>
          {workspace.error instanceof ApiError ? workspace.error.message : t("investmentsLoadError")}
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

      <InvestmentPeriodView
        request={request}
        groupBy={groupBy}
        workspacePositions={workspace.positions}
        onNewPosition={() => (holdingAccounts.length === 0 ? openAccountCreate() : openPositionCreate())}
        positionMenu={menus.positionMenu}
        groupMenu={(group) => {
          const account = accountOfGroup(group);
          return account === undefined ? [] : menus.accountMenu(account);
        }}
      />
      <InvestmentAccountsDrawer
        scope={accountsScope}
        hidden={formOpen}
        accounts={workspace.accounts}
        positions={workspace.positions}
        operations={workspace.operations}
        menuOf={menus.accountMenu}
        onNewAccount={openAccountCreate}
        onScope={setAccountsScope}
        onEditOperation={openOperationEdit}
        onRemoveOperation={(operation) =>
          void run(
            () => workspace.deleteOperation(operation.id),
            t("operationDeleteError"),
            t("deleted"),
          )
        }
        onClose={() => setAccountsScope(null)}
        busy={busy}
      />

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
        accounts={workspace.accounts}
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
        onSubmitCompound={(operations) => void save(() => workspace.createOperations({ operations }), () => setOperationFormOpen(false), t("operationSaveError"))}
        onCancel={() => setOperationFormOpen(false)}
      />
    </Page>
  );
}
