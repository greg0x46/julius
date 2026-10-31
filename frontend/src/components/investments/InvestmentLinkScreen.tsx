import { useTranslation } from "../../i18n/context";
import { translate, type Language } from "../../i18n/messages";
import { PlusOutlined } from "../ui/icons";
import { useQueries } from "@tanstack/react-query";
import { Alert, Button, InputNumber, List, Space, Typography } from "../ui";
import { calendar } from "../../presentation/calendar";
import { useState } from "react";

import { listInvestmentTransactions } from "../../api/investments";
import type {
  InvestmentOperation,
  InvestmentOperationKind,
  InvestmentOperationWrite,
  InvestmentPosition,
  InvestmentTransaction,
  TransactionItem,
} from "../../api/contracts";
import { useInvestmentWorkspace } from "../../hooks/useInvestmentWorkspace";
import { formatDateOnly } from "../../presentation/dates";
import { isPositiveDecimal,isZeroDecimal, subtractDecimals, sumDecimals } from "../../presentation/decimal";
import { investmentOperationKindName } from "../../presentation/investmentWorkspaceLabels";
import { movementTypeLabel } from "../../presentation/investmentLabels";
import { formatBRL } from "../../presentation/money";
import { detailValue } from "../../presentation/transactionDetail";
import { PanelDisclosure, PanelFooter, PanelSection } from "../shared/PanelStack";
import { TransactionPicker, type PickerTransaction } from "../shared/TransactionPicker";
import { InvestmentOperationFields } from "./InvestmentOperationForm";
import { queryKeys } from "../../api/queryKeys";

function absolute(value: string): string {
  return value.replace(/^-/, "");
}
const positive = isPositiveDecimal;

function operationDate(transaction: TransactionItem): string {
  return transaction.occurred_at ? calendar(transaction.occurred_at).format("YYYY-MM-DD") : calendar().format("YYYY-MM-DD");
}

function joinLabel(parts: Array<string | null | undefined>): string {
  return parts.filter(Boolean).join(" · ");
}

// A picker description reads as one phrase — "Resgate CDB - Nu Financeira" —
// not as kind · position; the picker's second line carries the separators.
function pickerDescription(kind: string, position: string | null | undefined): string {
  return [kind, position].filter(Boolean).join(" ");
}

function operationLabel(operation: InvestmentOperation, accountName: string, positionName: string | null, language: Language): string {
  const figures = `${formatBRL(operation.amount)} · ${formatDateOnly(operation.occurred_on, language)}`;
  if (operation.source === "synced") {
    return joinLabel([operation.notes ?? translate(language, "importedMovement"), accountName, figures]);
  }
  return joinLabel([investmentOperationKindName(operation.kind, language), accountName, positionName, figures]);
}

function movementDate(movement: InvestmentTransaction): string | null {
  return (movement.trade_date ?? movement.occurred_at)?.slice(0, 10) ?? null;
}

/**
 * One row of the destination picker: either a local operation or a provider
 * movement, which the backend turns into its own read-only pivot on link.
 */
type Candidate = {
  key: string;
  /** How the picker shows this row; the amount is the movement's total. */
  picker: PickerTransaction;
  operationId: string | null;
  movementId: string | null;
  available: string;
  date: string | null;
};

/** How much of the bank line is still free to allocate, and which way it flows. */
function linkFigures(transaction: TransactionItem, linkedAmount: string) {
  const fullValue =
    transaction.effective_money?.currency_code === "BRL" ? absolute(transaction.effective_money.value) : null;
  const remaining = fullValue === null ? "0.00" : subtractDecimals(fullValue, linkedAmount);
  const direction: "deposit" | "withdrawal" | null =
    transaction.classification === "outflow" ? "deposit" : transaction.classification === "inflow" ? "withdrawal" : null;
  const compatibleKinds: InvestmentOperationKind[] =
    direction === "deposit" ? ["deposit", "fee", "tax"] : direction === "withdrawal" ? ["withdrawal", "income"] : [];
  return { fullValue, available: positive(remaining) ? remaining : "0.00", direction, compatibleKinds };
}

/**
 * "Vincular a investimento": bank transactions are never moved
 * automatically. This screen lists the links the line already has and
 * offers likely destinations — local aportes/resgates and the provider's
 * own movements on integrated custodies; the person chooses the destination
 * and the amount, including a partial split.
 */
export function InvestmentLinkScreen({
  transaction,
  onNewOperation,
  onDone,
}: {
  transaction: TransactionItem;
  /** Push the "Nova movimentação" screen, carrying the amount typed so far. */
  onNewOperation: (amount: string | null) => void;
  /** Called after a successful link so the stack can pop back. */
  onDone: () => void;
}) {
  const { t, language } = useTranslation();
  const workspace = useInvestmentWorkspace();
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const [amount, setAmount] = useState<string | null | undefined>(undefined);
  const [error, setError] = useState<string | null>(null);

  const syncedPositions = workspace.positions.filter(
    (position) => position.source === "synced" && position.currency_code === "BRL",
  );
  const importedQueries = useQueries({
    queries: syncedPositions.map((position) => ({
      queryKey: queryKeys.investmentTransactionsOf(position.id),
      queryFn: ({ signal }: { signal?: AbortSignal }) => listInvestmentTransactions(position.id, signal),
    })),
  });
  const importedLoading = importedQueries.some((query) => query.isLoading);
  const importedError = importedQueries.some((query) => query.isError);

  const links = workspace.reconciliations.filter((link) => link.financial_transaction_id === transaction.id);
  const linkedAmount = sumDecimals(links.map((link) => link.amount));
  const { fullValue, available, direction, compatibleKinds } = linkFigures(transaction, linkedAmount);
  // Until the person types, the field follows the available amount (which
  // only settles once the ledger has loaded).
  const amountValue = amount === undefined ? (positive(available) ? available : null) : amount;

  const accountById = new Map(workspace.accounts.map((account) => [account.id, account]));
  const positionById = new Map(workspace.positions.map((position) => [position.id, position]));
  const linkedToOperation = (operationId: string) =>
    sumDecimals(workspace.reconciliations.filter((link) => link.operation_id === operationId).map((link) => link.amount));
  const pivotFor = (movementId: string): InvestmentOperation | null => {
    const link = workspace.reconciliations.find((item) => item.financial_investment_transaction_id === movementId);
    return link
      ? workspace.operations.find((operation) => operation.id === link.operation_id && operation.source === "synced") ?? null
      : null;
  };

  const operationCandidates: Candidate[] = workspace.operations
    .filter((operation) => operation.source === "manual" && compatibleKinds.includes(operation.kind))
    .filter((operation) => accountById.get(operation.account_id)?.currency_code === "BRL")
    .map((operation) => {
      const positionName = operation.position_id ? positionById.get(operation.position_id)?.name ?? null : null;
      return {
      key: `operation:${operation.id}`,
      picker: {
        id: `operation:${operation.id}`,
        description: pickerDescription(investmentOperationKindName(operation.kind, language), positionName),
        amount: operation.amount,
        direction: transaction.classification === "unclassified" ? null : transaction.classification,
        date: operation.occurred_on,
        account: accountById.get(operation.account_id)?.name ?? t("accountNotFoundShort"),
        category: t("manualMovement"),
        keywords: operation.notes ? [operation.notes] : undefined,
      },
      operationId: operation.id,
      movementId: null,
      available: subtractDecimals(operation.amount, linkedToOperation(operation.id)),
      date: operation.occurred_on,
      };
    });
  const importedCandidates: Candidate[] = syncedPositions.flatMap((position: InvestmentPosition, index) =>
    (importedQueries[index]?.data ?? [])
      .filter((movement) => movement.amount !== null && movement.direction === (direction === "deposit" ? "inflow" : "outflow"))
      .map((movement) => {
        const pivot = pivotFor(movement.id);
        const total = absolute(movement.amount!);
        const date = movementDate(movement);
        return {
          key: `movement:${movement.id}`,
          picker: {
            id: `movement:${movement.id}`,
            description: pickerDescription(
              movement.movement_type === null ? t("movementGenericSingular") : movementTypeLabel(movement.movement_type, language),
              position.name,
            ),
            amount: total,
            direction: transaction.classification === "unclassified" ? null : transaction.classification,
            date,
            account: accountById.get(position.account_id)?.name ?? null,
            category: t("importedFromInstitution"),
            keywords: position.ticker ? [position.ticker] : undefined,
          },
          operationId: pivot?.id ?? null,
          movementId: movement.id,
          available: pivot ? subtractDecimals(pivot.amount, linkedToOperation(pivot.id)) : total,
          date,
        };
      }),
  );
  const alreadyLinked = (candidate: Candidate) =>
    links.some(
      (link) =>
        (candidate.operationId !== null && link.operation_id === candidate.operationId) ||
        (candidate.movementId !== null && link.financial_investment_transaction_id === candidate.movementId),
    );
  const dateDistance = (candidate: Candidate) =>
    candidate.date ? Math.abs(calendar(candidate.date).diff(calendar(operationDate(transaction)), "day")) : Number.MAX_SAFE_INTEGER;
  const suggested = (candidate: Candidate) =>
    dateDistance(candidate) <= 5 && isZeroDecimal(subtractDecimals(candidate.available, available));
  const candidates = [...importedCandidates, ...operationCandidates]
    .filter((candidate) => positive(candidate.available) && !alreadyLinked(candidate))
    .sort((a, b) => Number(suggested(b)) - Number(suggested(a)) || dateDistance(a) - dateDistance(b));
  const selected = candidates.find((candidate) => candidate.key === selectedKey) ?? null;

  const link = async () => {
    if (!selected) {
      setError(t("selectDestinationMovement"));
      return;
    }
    const value = amountValue ?? "0";
    if (!positive(value)) {
      setError(t("positiveAmountRequired"));
      return;
    }
    if (positive(subtractDecimals(value, available))) {
      setError(t("linkedAmountTooHigh"));
      return;
    }
    setError(null);
    try {
      await workspace.createReconciliation({
        operation_id: selected.movementId ? null : selected.operationId,
        financial_transaction_id: transaction.id,
        financial_investment_transaction_id: selected.movementId,
        amount: value,
      });
      onDone();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : t("linkMovementError"));
    }
  };

  const unlink = async (reconciliationId: string) => {
    setError(null);
    try {
      await workspace.deleteReconciliation(reconciliationId);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : t("unlinkError"));
    }
  };

  const unsupportedReason =
    fullValue === null
      ? t("amountNotInBrl")
      : direction === null
        ? t("onlyClassifiedLinkable")
        : null;
  const canLink = unsupportedReason === null && positive(available);

  return (
    <>
      <header className="transaction-screen-summary">
        <span className={`transaction-amount amount-${transaction.classification}`}>{detailValue(transaction, language)}</span>
        <span>{transaction.description ?? t("descriptionNotProvided")}</span>
        {fullValue !== null && (
          <Typography.Text type="secondary">
            {t("linkedOfTotal", { linked: formatBRL(linkedAmount), total: formatBRL(fullValue) })}
            {positive(available) ? t("stillAvailableToSplit") : t("fullyLinked")}
          </Typography.Text>
        )}
      </header>

      {error && <Alert type="error" showIcon message={error} />}
      {importedError && (
        <Alert type="warning" showIcon message={t("importedHistoryError")} />
      )}

      {links.length > 0 && (
        <PanelSection title={t("currentLinks")}>
          <List
            size="small"
            dataSource={links}
            renderItem={(link) => {
              const operation = workspace.operations.find((item) => item.id === link.operation_id);
              const account = operation ? accountById.get(operation.account_id) : undefined;
              const position = operation?.position_id ? positionById.get(operation.position_id) : undefined;
              return (
                <List.Item
                  actions={[
                    <Button
                      key="unlink"
                      type="link"
                      danger
                      size="small"
                      loading={workspace.isDeleting}
                      onClick={() => void unlink(link.id)}
                    >
                      {t("unlink")}
                    </Button>,
                  ]}
                >
                  <Space direction="vertical" size={0}>
                    <span>
                      {operation
                        ? operationLabel(operation, account?.name ?? t("accountNotFoundShort"), position?.name ?? null, language)
                        : t("movementNotFound")}
                    </span>
                    <Typography.Text type="secondary">{t("amountLinkedSingular", { amount: formatBRL(link.amount) })}</Typography.Text>
                  </Space>
                </List.Item>
              );
            }}
          />
        </PanelSection>
      )}

      {unsupportedReason ? (
        <Typography.Text type="secondary">{unsupportedReason}</Typography.Text>
      ) : (
        <>
          <PanelSection title={t("destinationMovement")}>
            <TransactionPicker
              id="investment-link-destination"
              label={t("destinationMovement")}
              value={selectedKey}
              loading={importedLoading}
              disabled={!canLink}
              placeholder={direction === "deposit" ? t("selectContributionOrApplication") : t("selectRedemptionOrYield")}
              emptyText={t("noCompatibleCreateNew")}
              transactions={candidates.map((candidate) => ({
                ...candidate.picker,
                tag: suggested(candidate) ? t("suggestionTag") : undefined,
              }))}
              onSelect={setSelectedKey}
            />
            <Button
              icon={<PlusOutlined aria-hidden="true" />}
              disabled={!canLink}
              onClick={() => onNewOperation(amountValue)}
            >
              {t("newOperationButton")}
            </Button>
          </PanelSection>

          <PanelSection title={t("amountToLink")}>
            <InputNumber
              id="investment-link-amount"
              aria-label={t("amountToLink")}
              style={{ width: "100%" }}
              min="0.01"
              max={available}
              step="0.01"
              stringMode
              decimalSeparator=","
              disabled={!canLink}
              value={amountValue}
              onChange={(value) => setAmount(value === null ? null : String(value))}
            />
            <Typography.Text type="secondary" className="transaction-field-hint">
              {t("availableAmount", { amount: formatBRL(available) })}
            </Typography.Text>
            <PanelDisclosure summary={t("checkBeforeLinking")}>
              {t("linkSuggestionsExplanation")}
            </PanelDisclosure>
          </PanelSection>

          <PanelFooter>
            <Button type="primary" block loading={workspace.isSaving} disabled={!canLink} onClick={() => void link()}>
              {t("link")}
            </Button>
          </PanelFooter>
        </>
      )}
    </>
  );
}

const formId = "investment-link-new-operation";

/**
 * "Nova movimentação" reached from the link screen: creates the operation
 * and links it to the bank line in one atomic write.
 */
export function InvestmentLinkNewOperationScreen({
  transaction,
  amount,
  onDone,
}: {
  transaction: TransactionItem;
  /** The amount typed on the link screen; also the operation's default value. */
  amount: string | null;
  onDone: () => void;
}) {
  const { t } = useTranslation();
  const workspace = useInvestmentWorkspace();
  const [error, setError] = useState<string | null>(null);
  const links = workspace.reconciliations.filter((link) => link.financial_transaction_id === transaction.id);
  const { available, direction, compatibleKinds } = linkFigures(transaction, sumDecimals(links.map((link) => link.amount)));

  const createAndLink = async (write: InvestmentOperationWrite) => {
    const value = amount ?? "0";
    if (!positive(value) || positive(subtractDecimals(value, available))) {
      setError(t("reviewLinkAmount"));
      return;
    }
    setError(null);
    try {
      await workspace.createOperations({
        operations: [write],
        reconciliation: {
          financial_transaction_id: transaction.id,
          financial_investment_transaction_id: null,
          amount: value,
        },
      });
      onDone();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : t("createLinkError"));
    }
  };

  return (
    <>
      <PanelSection>
        <InvestmentOperationFields
          formId={formId}
          operation={null}
          initial={{
            kind: direction ?? "deposit",
            occurred_on: operationDate(transaction),
            amount: amount ?? undefined,
          }}
          allowedKinds={compatibleKinds}
          accounts={workspace.accounts}
          positions={workspace.positions}
          submitError={error}
          onSubmit={(write) => void createAndLink(write)}
        />
      </PanelSection>
      <PanelFooter>
        <Button type="primary" block htmlType="submit" form={formId} loading={workspace.isSaving}>
          {t("createAndLink")}
        </Button>
      </PanelFooter>
    </>
  );
}
