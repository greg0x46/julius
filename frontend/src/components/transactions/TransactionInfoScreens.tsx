import { Descriptions, Tag } from "../ui";

import type { TransactionItem } from "../../api/contracts";
import { useTimezone } from "../../hooks/useTimezone";
import { internalCategoryOriginLabel } from "../../presentation/categoryLabels";
import { formatBRL, formatMoney, moneySourceLabel } from "../../presentation/money";
import { dateTime, installmentLabel, investmentSplit, remainderLabel } from "../../presentation/transactionDetail";
import { movementTypeLabel } from "../../presentation/transactionLabels";
import { exclusionReasonLabel, inclusionOriginLabel } from "../../presentation/transactionStatus";
import { PanelSection } from "../shared/PanelStack";

/** Where and when, plus the history behind the current decisions — read-only. */
export function TransactionDetailsScreen({ item }: { item: TransactionItem }) {
  const timezone = useTimezone();
  const split = investmentSplit(item);
  return (
    <>
      <PanelSection title="Quando & onde">
        <Descriptions column={1} size="small" colon={false}>
          <Descriptions.Item label="Data e hora">{dateTime(item.occurred_at, timezone)}</Descriptions.Item>
          <Descriptions.Item label="Conta">{item.account.name ?? "Conta não informada"}</Descriptions.Item>
          <Descriptions.Item label="Instituição">
            {item.account.institution ?? "Instituição não informada"}
          </Descriptions.Item>
          {item.card && (
            <Descriptions.Item label="Cartão">
              {item.card.number}
              {installmentLabel(item.card) && (
                <Tag className="transaction-card-installment" color="blue">
                  {installmentLabel(item.card)}
                </Tag>
              )}
            </Descriptions.Item>
          )}
        </Descriptions>
      </PanelSection>

      <PanelSection title="Decisões">
        <Descriptions column={1} size="small" colon={false}>
          <Descriptions.Item label="Categoria">
            {item.internal_category
              ? `${item.internal_category.name} · ${internalCategoryOriginLabel(item.internal_category)}`
              : "Sem categoria"}
          </Descriptions.Item>
          <Descriptions.Item label="Sugestão do banco">{item.source_category ?? "Não informada"}</Descriptions.Item>
          <Descriptions.Item label="Nos totais">
            {item.inclusion.state === "ignored" ? "Não" : "Sim"}
            {item.inclusion.changed_at &&
              ` · ${inclusionOriginLabel(item.inclusion)} em ${dateTime(item.inclusion.changed_at, timezone)}`}
          </Descriptions.Item>
          {split && (
            <Descriptions.Item label="Investimento">
              {item.classification === "inflow" ? "Resgate" : "Aporte"} vinculado: {formatBRL(split.transferred)} ·{" "}
              {remainderLabel(item, split.remaining)}
            </Descriptions.Item>
          )}
        </Descriptions>
      </PanelSection>
    </>
  );
}

/** The provider's raw view of the line, for when a figure needs to be traced back. */
export function TransactionTechnicalScreen({ item }: { item: TransactionItem }) {
  const reason = item.totals_eligibility.reason;
  return (
    <PanelSection>
      <Descriptions column={1} size="small" colon={false}>
        <Descriptions.Item label="Origem">{item.origin === "manual" ? "Manual" : item.source_provider === "file" ? "Arquivo importado" : "Conexão automática"}</Descriptions.Item>
        <Descriptions.Item label="Identificador externo">{item.external_id}</Descriptions.Item>
        <Descriptions.Item label="Tipo original">{movementTypeLabel(item.movement_type)}</Descriptions.Item>
        <Descriptions.Item label="Valor original">
          {item.amount !== null && item.currency_code ? formatMoney(item.amount, item.currency_code) : "Não informado"}
        </Descriptions.Item>
        <Descriptions.Item label="Moeda original">{item.currency_code ?? "Não informada"}</Descriptions.Item>
        <Descriptions.Item label="Status original da integração">
          {item.provider_status ?? "Não informado"}
        </Descriptions.Item>
        {item.effective_money && (
          <Descriptions.Item label="Origem do valor exibido">
            {moneySourceLabel(item.effective_money.source)}
          </Descriptions.Item>
        )}
        <Descriptions.Item label="Totalização">
          {item.totals_eligibility.included
            ? "Incluída nos totais"
            : reason
              ? exclusionReasonLabel[reason]
              : "Não incluída"}
        </Descriptions.Item>
      </Descriptions>
    </PanelSection>
  );
}
