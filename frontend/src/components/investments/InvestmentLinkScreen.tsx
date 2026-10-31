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
import { useTimezone } from "../../hooks/useTimezone";
import { calendarIn, todayIn } from "../../presentation/timezone";
import { formatDateOnly } from "../../presentation/dates";
import { isPositiveDecimal,isZeroDecimal, subtractDecimals, sumDecimals } from "../../presentation/decimal";
import { investmentOperationKindLabel } from "../../presentation/investmentWorkspaceLabels";
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

function operationDate(transaction: TransactionItem, timezone: string): string {
  return (transaction.occurred_at ? calendarIn(transaction.occurred_at, timezone) : todayIn(timezone)).format("YYYY-MM-DD");
}

function joinLabel(parts: Array<string | null | undefined>): string {
  return parts.filter(Boolean).join(" · ");
}

// A picker description reads as one phrase — "Resgate CDB - Nu Financeira" —
// not as kind · position; the picker's second line carries the separators.
function pickerDescription(kind: string, position: string | null | undefined): string {
  return [kind, position].filter(Boolean).join(" ");
}

function operationLabel(operation: InvestmentOperation, accountName: string, positionName: string | null): string {
  const figures = `${formatBRL(operation.amount)} · ${formatDateOnly(operation.occurred_on)}`;
  if (operation.source === "synced") {
    return joinLabel([operation.notes ?? "Movimento importado", accountName, figures]);
  }
  return joinLabel([investmentOperationKindLabel[operation.kind], accountName, positionName, figures]);
}

const movementTypeLabel: Record<string, string> = { BUY: "Aplicação", SELL: "Resgate", INTEREST: "Rendimento" };

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
  const workspace = useInvestmentWorkspace();
  const timezone = useTimezone();
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
        description: pickerDescription(investmentOperationKindLabel[operation.kind], positionName),
        amount: operation.amount,
        direction: transaction.classification === "unclassified" ? null : transaction.classification,
        date: operation.occurred_on,
        account: accountById.get(operation.account_id)?.name ?? "Conta não encontrada",
        category: "Movimentação manual",
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
              movementTypeLabel[movement.movement_type ?? ""] ?? movement.movement_type ?? "Movimento",
              position.name,
            ),
            amount: total,
            direction: transaction.classification === "unclassified" ? null : transaction.classification,
            date,
            account: accountById.get(position.account_id)?.name ?? null,
            category: "Importado da instituição",
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
    candidate.date ? Math.abs(calendar(candidate.date).diff(calendar(operationDate(transaction, timezone)), "day")) : Number.MAX_SAFE_INTEGER;
  const suggested = (candidate: Candidate) =>
    dateDistance(candidate) <= 5 && isZeroDecimal(subtractDecimals(candidate.available, available));
  const candidates = [...importedCandidates, ...operationCandidates]
    .filter((candidate) => positive(candidate.available) && !alreadyLinked(candidate))
    .sort((a, b) => Number(suggested(b)) - Number(suggested(a)) || dateDistance(a) - dateDistance(b));
  const selected = candidates.find((candidate) => candidate.key === selectedKey) ?? null;

  const link = async () => {
    if (!selected) {
      setError("Selecione uma movimentação de destino.");
      return;
    }
    const value = amountValue ?? "0";
    if (!positive(value)) {
      setError("Informe um valor maior que zero.");
      return;
    }
    if (positive(subtractDecimals(value, available))) {
      setError("O valor vinculado não pode ser maior que o saldo disponível desta transação.");
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
      setError(caught instanceof Error ? caught.message : "Não foi possível vincular a movimentação.");
    }
  };

  const unlink = async (reconciliationId: string) => {
    setError(null);
    try {
      await workspace.deleteReconciliation(reconciliationId);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Não foi possível desfazer o vínculo.");
    }
  };

  const unsupportedReason =
    fullValue === null
      ? "O valor desta transação não está disponível em reais para vincular."
      : direction === null
        ? "Só entradas e saídas classificadas podem ser vinculadas a um investimento."
        : null;
  const canLink = unsupportedReason === null && positive(available);

  return (
    <>
      <header className="transaction-screen-summary">
        <span className={`transaction-amount amount-${transaction.classification}`}>{detailValue(transaction)}</span>
        <span>{transaction.description ?? "Descrição não informada"}</span>
        {fullValue !== null && (
          <Typography.Text type="secondary">
            {formatBRL(linkedAmount)} de {formatBRL(fullValue)} vinculados
            {positive(available) ? " · ainda disponível para dividir" : " · valor integralmente vinculado"}
          </Typography.Text>
        )}
      </header>

      {error && <Alert type="error" showIcon message={error} />}
      {importedError && (
        <Alert type="warning" showIcon message="Não foi possível carregar o histórico importado de todos os investimentos." />
      )}

      {links.length > 0 && (
        <PanelSection title="Vínculos atuais">
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
                      Desvincular
                    </Button>,
                  ]}
                >
                  <Space direction="vertical" size={0}>
                    <span>
                      {operation
                        ? operationLabel(operation, account?.name ?? "Conta não encontrada", position?.name ?? null)
                        : "Movimentação não encontrada"}
                    </span>
                    <Typography.Text type="secondary">{formatBRL(link.amount)} vinculado</Typography.Text>
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
          <PanelSection title="Movimentação de destino">
            <TransactionPicker
              id="investment-link-destination"
              label="Movimentação de destino"
              value={selectedKey}
              loading={importedLoading}
              disabled={!canLink}
              placeholder={direction === "deposit" ? "Selecione um aporte ou aplicação" : "Selecione um resgate ou rendimento"}
              emptyText="Nenhuma movimentação compatível. Crie uma nova."
              transactions={candidates.map((candidate) => ({
                ...candidate.picker,
                tag: suggested(candidate) ? "Sugestão" : undefined,
              }))}
              onSelect={setSelectedKey}
            />
            <Button
              icon={<PlusOutlined aria-hidden="true" />}
              disabled={!canLink}
              onClick={() => onNewOperation(amountValue)}
            >
              Nova movimentação
            </Button>
          </PanelSection>

          <PanelSection title="Valor a vincular">
            <InputNumber
              id="investment-link-amount"
              aria-label="Valor a vincular"
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
              Disponível: {formatBRL(available)}. Você pode vincular o restante depois.
            </Typography.Text>
            <PanelDisclosure summary="Confira os dados antes de vincular.">
              As sugestões usam direção, moeda, valor disponível e datas próximas. Movimentos importados da
              instituição aparecem junto das movimentações manuais. Você decide o destino e pode dividir esta
              transação em mais de uma movimentação.
            </PanelDisclosure>
          </PanelSection>

          <PanelFooter>
            <Button type="primary" block loading={workspace.isSaving} disabled={!canLink} onClick={() => void link()}>
              Vincular
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
  const workspace = useInvestmentWorkspace();
  const timezone = useTimezone();
  const [error, setError] = useState<string | null>(null);
  const links = workspace.reconciliations.filter((link) => link.financial_transaction_id === transaction.id);
  const { available, direction, compatibleKinds } = linkFigures(transaction, sumDecimals(links.map((link) => link.amount)));

  const createAndLink = async (write: InvestmentOperationWrite) => {
    const value = amount ?? "0";
    if (!positive(value) || positive(subtractDecimals(value, available))) {
      setError("Revise o valor a vincular antes de criar a movimentação.");
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
      setError(caught instanceof Error ? caught.message : "Não foi possível criar e vincular a movimentação.");
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
            occurred_on: operationDate(transaction, timezone),
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
          Criar e vincular
        </Button>
      </PanelFooter>
    </>
  );
}
