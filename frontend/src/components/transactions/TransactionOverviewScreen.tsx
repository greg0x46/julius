import { LinkOutlined, RedoOutlined, SwapOutlined } from "@ant-design/icons";
import { Alert, Button, Switch, Typography } from "antd";
import { useQuery } from "@tanstack/react-query";
import { getTransactionTransfer } from "../../api/transfers";
import { queryKeys } from "../../api/queryKeys";
import { useMemo } from "react";

import type { Category, TransactionInclusionState, TransactionItem } from "../../api/contracts";
import { useTransactionReconciliation } from "../../hooks/useTransactionReconciliation";
import { internalCategoryOriginLabel } from "../../presentation/categoryLabels";
import { formatDay } from "../../presentation/dates";
import { formatBRL, formatMoney } from "../../presentation/money";
import {
  allowsInvestmentLink,
  categoryChoices,
  installmentLabel,
  investmentSplit,
  recurringCommitmentPrefill,
  remainderLabel,
  shortDateTime,
  suggestedCategory,
} from "../../presentation/transactionDetail";
import { CategoryField } from "../categories/CategoryField";
import { PanelNavRow, PanelSection } from "../shared/PanelStack";
import { TransactionAmount } from "./TransactionAmount";
import type { TransactionScreen } from "./TransactionPanel";
import type { WriteIssue } from "./useTransactionPanelWrites";

/**
 * The root screen: who the transaction is and the decisions people take on
 * almost every one (category, whether it counts). Everything else is one
 * tap away behind a row, never open by default.
 *
 * A failed write is shown here, next to the control that caused it — the
 * panel covers the list, so an error on the page would never be seen.
 */
export function TransactionOverviewScreen({
  item,
  categories,
  onCategory,
  categoryPending,
  categoryError = null,
  onInclusion,
  inclusionPending,
  inclusionError = null,
  deleteError,
  navigate,
}: {
  item: TransactionItem;
  categories: Category[];
  onCategory?: (id: string, categoryId: string) => void;
  categoryPending: boolean;
  categoryError?: WriteIssue | null;
  onInclusion?: (id: string, state: TransactionInclusionState) => void;
  inclusionPending: boolean;
  inclusionError?: WriteIssue | null;
  deleteError: string | null;
  navigate: (screen: TransactionScreen) => void;
}) {
  const ignored = item.inclusion.state === "ignored";
  const transfer = useQuery({ queryKey: queryKeys.transactionTransfer(item.id), queryFn: ({ signal }) => getTransactionTransfer(item.id, signal), retry: false });
  const linked = !!transfer.data || item.totals_eligibility.reason === "linked_transfer";
  const linkedPrincipal = linked && transfer.data?.fee?.transaction_id !== item.id;
  const otherMovement = transfer.data
    ? (transfer.data.source.transaction_id === item.id ? transfer.data.destination : transfer.data.source)
    : null;
  const transferHint = otherMovement
    ? `${otherMovement.account_name} · ${formatMoney(otherMovement.amount.replace(/^-/, ""), otherMovement.currency_code)}`
    : transfer.isLoading ? "Carregando…" : transfer.isError ? "Não foi possível consultar" : linked ? "Transferência vinculada" : "Nenhum vínculo";
  const choices = useMemo(() => categoryChoices(categories, item), [categories, item]);
  const suggestion = suggestedCategory(item, choices);
  const split = investmentSplit(item);
  const prefill = recurringCommitmentPrefill(item);
  // The row only summarises; the reconcile screen owns the writes.
  const reconciliation = useTransactionReconciliation(item.id, !ignored);

  const where = item.card
    ? [item.card.number, installmentLabel(item.card)].filter(Boolean).join(" · ")
    : [item.account.name, item.account.institution].filter(Boolean).join(" · ");

  const reconciliationHint = ignored
    ? "Transações fora dos totais não conciliam"
    : reconciliation.isLoading
      ? "Carregando…"
      : reconciliation.isError
        ? "Não foi possível consultar"
        : reconciliation.current
        ? `${reconciliation.current.commitment_name} · ${formatDay(reconciliation.current.occurrence_date)}`
        : reconciliation.options.length > 0
          ? `${reconciliation.options.length} ${reconciliation.options.length === 1 ? "ocorrência próxima" : "ocorrências próximas"}`
          : "Nenhuma recorrência por perto";

  const investmentHint = split
    ? `${formatBRL(split.transferred)} vinculados`
    : "Nenhum vínculo";

  // Origin and "out of the totals" are facts about the line, so they read as
  // plain text beside the date rather than as coloured tags.
  const when = [
    shortDateTime(item.occurred_at),
    item.origin === "manual" ? "Manual" : null,
    item.source_provider === "file" ? "Arquivo" : null,
    ignored ? "Fora dos totais" : null,
  ]
    .filter(Boolean)
    .join(" · ");

  return (
    <>
      <header className="transaction-identity">
        <strong className="transaction-identity-description">
          {item.description ?? "Descrição não informada"}
        </strong>
        {where && <span className="transaction-identity-where">{where}</span>}
        <TransactionAmount item={item} size="hero" className="transaction-identity-amount" />
        <span className="transaction-identity-when">{when}</span>
        {split && (
          <span className="transaction-identity-split">
            {item.classification === "inflow" ? "resgate" : "aporte"} vinculado:{" "}
            {formatBRL(split.transferred)} · {remainderLabel(item, split.remaining)}
          </span>
        )}
      </header>

      <PanelSection title="Categoria">
        <CategoryField
          id="transaction-category"
          value={item.internal_category?.id ?? null}
          options={choices}
          suggested={suggestion}
          loading={categoryPending}
          disabled={!onCategory}
          onChange={(value) => onCategory?.(item.id, value)}
        />
        {item.internal_category && (
          <Typography.Text type="secondary" className="transaction-field-hint">
            {internalCategoryOriginLabel(item.internal_category)}
          </Typography.Text>
        )}
        {categoryError && <WriteFailure issue={categoryError} title="Não foi possível salvar a categoria" />}
        {item.internal_category && !item.internal_category.is_active && (
          <Alert
            type="warning"
            showIcon
            message="Esta categoria foi desativada. Ela continua vigente para esta transação, mas não pode ser escolhida em outras."
          />
        )}
      </PanelSection>

      <PanelSection>
        <div className="transaction-toggle-row">
          <label htmlFor="transaction-considered">Considerar nos totais</label>
          <Switch
            id="transaction-considered"
            checked={!ignored}
            loading={inclusionPending}
            disabled={!onInclusion || linkedPrincipal}
            onChange={(checked) => onInclusion?.(item.id, checked ? "considered" : "ignored")}
          />
        </div>
        {linkedPrincipal && <Typography.Text type="secondary">Transferência vinculada: fora das receitas e despesas. Desvincule para alterar a inclusão.</Typography.Text>}
        {ignored && (
          <Typography.Text type="secondary" className="transaction-field-hint">
            Fora dos totais: esta transação não entra nos relatórios e cálculos do período.
          </Typography.Text>
        )}
        {inclusionError && <WriteFailure issue={inclusionError} title="Não foi possível salvar a decisão" />}
      </PanelSection>

      <PanelSection title="Ações" className="panel-section-rows">
        <PanelNavRow icon={<SwapOutlined />} label="Transferência entre contas" hint={transferHint} disabled={ignored || item.classification === "unclassified"} onClick={() => navigate({ kind: "transfer" })} />
        {prefill && (
          <PanelNavRow
            icon={<RedoOutlined />}
            label="Criar recorrência"
            onClick={() => navigate({ kind: "recurrence" })}
          />
        )}
        <PanelNavRow
          icon={<SwapOutlined />}
          label="Conciliar com recorrência"
          hint={reconciliationHint}
          disabled={ignored}
          onClick={() => navigate({ kind: "reconcile" })}
        />
        {allowsInvestmentLink(item) && (
          <PanelNavRow
            icon={<LinkOutlined />}
            label="Vincular a investimento"
            hint={investmentHint}
            onClick={() => navigate({ kind: "investment" })}
          />
        )}
      </PanelSection>

      <PanelSection title="Mais informações" className="panel-section-rows">
        <PanelNavRow label="Detalhes da transação" onClick={() => navigate({ kind: "details" })} />
        <PanelNavRow label="Informações técnicas" onClick={() => navigate({ kind: "technical" })} />
      </PanelSection>

      {item.origin === "manual" && split && (
        <Alert
          type="warning"
          showIcon
          message="Transação vinculada a investimento"
          description="Para editar ou excluir esta transação, desfaça antes os vínculos em “Vincular a investimento”."
        />
      )}
      {deleteError && <Alert type="error" showIcon message={deleteError} />}
    </>
  );
}

/** A failed write, in the panel and next to the control that caused it: what happened, and a retry. */
function WriteFailure({ issue, title }: { issue: WriteIssue; title: string }) {
  return (
    <Alert
      type="error"
      showIcon
      message={title}
      description={
        <>
          <p>{issue.message}</p>
          <Button size="small" onClick={issue.retry}>
            Tentar novamente
          </Button>
        </>
      }
    />
  );
}
