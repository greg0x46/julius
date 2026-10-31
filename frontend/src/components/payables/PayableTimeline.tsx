import { isPositiveDecimal, isZeroDecimal } from "../../presentation/decimal";
import { PlusOutlined } from "../ui/icons";
import { Alert, Button, DatePicker, Flex, InputNumber, Radio, Skeleton, Typography } from "../ui";
import type { CalendarDate } from "../../presentation/calendar";
import { todayIn } from "../../presentation/timezone";
import { useState } from "react";

import type {
  Cadence,
  EligibleTransaction,
  PayableKind,
  PayableLink,
  PayableLinkedTransaction,
  Realization,
  ScenarioTransaction,
} from "../../api/contracts";
import { usePayablePlan } from "../../hooks/usePayablePlan";
import { useTimezone } from "../../hooks/useTimezone";
import { calendarParts } from "../../presentation/dates";
import { errorMessage } from "../../presentation/errors";
import { formatBRL, subtractBRL, sumBRL } from "../../presentation/money";
import { payableVocabulary } from "../../presentation/payableLabels";
import { scenarioTransactionStatusLabel } from "../../presentation/scenarioLabels";
import { FormDrawer } from "../forms/FormDrawer";
import { FormField } from "../forms/FormField";
import { MoneyInput } from "../forms/MoneyInput";
import { Section } from "../layout";
import { Money } from "../shared/Money";
import { RecordMenu, type RecordMenuItem } from "../shared/RecordMenu";
import { StatusTag, type StatusTagTone } from "../shared/StatusTag";
import { TransactionPicker, type PickerTransaction } from "../shared/TransactionPicker";
import { useConfirm } from "../shared/useConfirm";
import { useFeedback } from "../shared/useFeedback";

/** "20/02", with the year only when it is not the current one — a plan can span years. */
function formatShortDate(value: string, timezone: string): string {
  const parts = calendarParts(value, timezone);
  if (parts === null) return "Data inválida";
  const day = `${String(parts.day).padStart(2, "0")}/${String(parts.month).padStart(2, "0")}`;
  return parts.year === todayIn(timezone).year() ? day : `${day}/${parts.year}`;
}

/**
 * Installment descriptions repeat the payable's name ("Parcela 1/12 -
 * Empréstimo pessoal Itaú"), which the page title already carries; the row
 * only needs the part that tells installments apart.
 */
function installmentLabel(description: string): string {
  return /^Parcela \d+\/\d+/.exec(description)?.[0] ?? description;
}

// Money leaves to pay a debt and arrives to settle a receivable; every row
// the pickers below offer flows the payable's own way.
function payableDirection(kind: PayableKind): "inflow" | "outflow" {
  return kind === "debt" ? "outflow" : "inflow";
}

function candidateRow(candidate: EligibleTransaction, kind: PayableKind): PickerTransaction {
  return {
    id: candidate.id,
    description: candidate.description ?? "Sem descrição",
    amount: candidate.effective_money.value,
    direction: payableDirection(kind),
    date: candidate.occurred_at,
    account: candidate.account_name,
  };
}

function installmentRow(installment: ScenarioTransaction, kind: PayableKind): PickerTransaction {
  return {
    id: installment.id,
    description: installment.description,
    amount: installment.amount,
    direction: payableDirection(kind),
    date: installment.projected_at,
    category: `Parcela · ${scenarioTransactionStatusLabel[installment.status]}`,
  };
}

/**
 * Only an installment that is off the common path says so: "Planejado" (not
 * due yet) and "Paga" (settled exactly as planned — the realization under it
 * already says it) are the defaults and stay unsaid.
 */
const installmentStatusTone: Partial<Record<ScenarioTransaction["status"], StatusTagTone>> = {
  atrasada: "danger",
  paga_parcialmente: "warning",
  paga_a_mais: "info",
};

function realizationLabel(realization: Realization, links: PayableLinkedTransaction[], timezone: string): string {
  const link = links.find((candidate) => candidate.id === realization.payable_link_id);
  const description = link?.description ?? "Transação vinculada";
  return link?.occurred_at ? `${formatShortDate(link.occurred_at, timezone)} · ${description}` : description;
}

/** The plan's overall standing as one quiet line, coloured only when it matters. */
function PlanNotice({ value, onTrackMessage }: { value: string; onTrackMessage: string }) {
  const isAhead = value.startsWith("-");
  const isOnTrack = !isAhead && isZeroDecimal(value);
  const magnitude = formatBRL(isAhead ? value.slice(1) : value);
  const tone = isOnTrack || isAhead ? "good" : "late";
  const text = isOnTrack
    ? onTrackMessage
    : isAhead
      ? `Adiantado ${magnitude} em relação ao plano até hoje.`
      : `Atrasado ${magnitude} em relação ao plano até hoje.`;
  return <p className={`payable-plan-notice payable-plan-notice-${tone}`}>{text}</p>;
}

// Lets a transaction that was linked standalone (no installment picked at
// link time) be allocated to a plan installment afterwards — the reverse
// direction of the combined vincular+alocar form below.
function AllocateExistingLinkControl({
  link,
  kind,
  installments,
  onAllocate,
}: {
  link: PayableLinkedTransaction;
  kind: PayableKind;
  installments: ScenarioTransaction[];
  onAllocate: (installmentId: string, amount: string) => Promise<void>;
}) {
  const [open, setOpen] = useState(false);
  const [installmentId, setInstallmentId] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  if (!open) {
    return (
      <Button type="link" size="small" className="payable-entry-link" onClick={() => setOpen(true)}>
        Alocar a uma parcela
      </Button>
    );
  }

  const submit = async () => {
    if (installmentId === null) return;
    setSubmitting(true);
    try {
      await onAllocate(installmentId, link.current_amount);
      setOpen(false);
      setInstallmentId(null);
    } catch {
      // The parent already surfaces the error via the shared action alert;
      // keep the form open so the user can adjust and retry.
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Flex gap="small" align="center" wrap>
      <div style={{ flex: "1 1 260px", minWidth: 0 }}>
        <TransactionPicker
          id={`allocate-link-${link.id}`}
          label="Parcela"
          placeholder="Escolha a parcela"
          emptyText="Nenhuma parcela disponível."
          value={installmentId}
          transactions={installments.map((installment) => installmentRow(installment, kind))}
          onSelect={setInstallmentId}
        />
      </div>
      <Button type="primary" loading={submitting} disabled={installmentId === null} onClick={submit}>
        Alocar
      </Button>
      <Button onClick={() => setOpen(false)}>Cancelar</Button>
    </Flex>
  );
}

type TimelineEntry =
  | { kind: "installment"; date: string; installment: ScenarioTransaction }
  | { kind: "link"; date: string; link: PayableLinkedTransaction };

/** A destructive or irreversible step waiting for the user's yes, shown in one shared dialog. */

// PayableTimeline replaces what used to be two separate tabs ("Plano de
// pagamento" and "Transações vinculadas"): a payable's installments and its
// linked transactions are one story, not two — an installment is settled by
// the very links shown underneath it, so they read as one chronological
// list instead of forcing the user to cross-reference two tables.
// Linking and allocating are also merged into a single "Vincular" action:
// picking a parcela at link time does both in one step, while leaving the
// parcela unset still links standalone. The only fork between a debt and a
// receivable is vocabulary (payableVocabulary, keyed by kind) — the
// mechanics are identical either way.
//
// Each row is one line (date · parcela, amount) plus a lighter status line;
// what can destroy data (excluir parcela, desalocar, desvincular, reajustar)
// sits behind a per-row `···` and asks through one dialog that is as usable
// on a phone as on a desktop, instead of a coloured link and a tiny popover
// on every row.
export function PayableTimeline({
  payableId,
  kind,
  links,
  search,
  onSearchChange,
  candidates,
  isSearching,
  isLinking,
  onLinkTransaction,
  onUnlinkTransaction,
}: {
  payableId: string;
  kind: PayableKind;
  links: PayableLinkedTransaction[];
  search: string;
  onSearchChange: (value: string) => void;
  candidates: EligibleTransaction[];
  isSearching: boolean;
  isLinking: boolean;
  onLinkTransaction: (transactionId: string) => Promise<PayableLink>;
  onUnlinkTransaction: (linkId: string) => Promise<void>;
}) {
  const vocab = payableVocabulary[kind];
  const plan = usePayablePlan(payableId);
  const [cadence, setCadence] = useState<Cadence>("mensal");
  const [generateBy, setGenerateBy] = useState<"months" | "amount">("months");
  const [months, setMonths] = useState<number | null>(6);
  const [installmentAmount, setInstallmentAmount] = useState<string | null>(null);
  const timezone = useTimezone();
  const [startDate, setStartDate] = useState<CalendarDate>(() => todayIn(timezone));
  const [actionError, setActionError] = useState<string | null>(null);
  const confirm = useConfirm();
  const feedback = useFeedback();

  const [addPaymentOpen, setAddPaymentOpen] = useState(false);
  const [selectedCandidateId, setSelectedCandidateId] = useState<string | null>(null);
  const [selectedInstallmentId, setSelectedInstallmentId] = useState<string | null>(null);
  const [linkSubmitting, setLinkSubmitting] = useState(false);
  const [linkError, setLinkError] = useState<string | null>(null);

  // A write started from a `···` or a confirm: the inline alert stays where
  // the timeline is, and a toast says it too — the alert alone can be out of
  // view once the list is long. A success toast closes the loop for a row that
  // changes under the finger.
  const runAction = async (action: () => Promise<unknown>, fallback: string, done?: string) => {
    setActionError(null);
    try {
      await action();
      if (done !== undefined) feedback.success(done);
    } catch (error) {
      const message = errorMessage(error, fallback);
      setActionError(message);
      feedback.error(message);
    }
  };

  const createPlan = () => {
    const name = vocab.planNoun.replace(/^./, (c) => c.toUpperCase());
    return runAction(() => plan.createPlan(name), "Não foi possível criar o plano.", "Plano criado");
  };

  const generate = () => {
    const start_date = startDate.format("YYYY-MM-DD");
    if (generateBy === "months") {
      if (months === null || months < 1) {
        setActionError("Informe o número de parcelas.");
        return;
      }
      return runAction(
        () => plan.generateInstallments({ cadence, months, start_date }),
        "Não foi possível gerar as parcelas.",
        "Parcelas geradas",
      );
    }
    if (installmentAmount === null || !isPositiveDecimal(installmentAmount)) {
      setActionError("Informe o valor de cada parcela.");
      return;
    }
    return runAction(
      () => plan.generateInstallments({ cadence, installment_amount: installmentAmount, start_date }),
      "Não foi possível gerar as parcelas.",
      "Parcelas geradas",
    );
  };

  const readjust = (strategy: "abater_do_final" | "redistribuir") =>
    runAction(
      () => plan.readjust({ strategy }),
      "Não foi possível reajustar as parcelas restantes.",
      "Parcelas reajustadas",
    );

  const deleteInstallment = (installment: ScenarioTransaction) =>
    runAction(() => plan.deleteInstallment(installment.id), "Não foi possível excluir a parcela.", "Parcela excluída");

  const deallocate = (installment: ScenarioTransaction, realization: Realization) =>
    runAction(
      () => plan.deallocateRealization({ transactionId: installment.id, realizationId: realization.id }),
      "Não foi possível desalocar a transação.",
      "Transação desalocada",
    );

  const unlink = (link: PayableLinkedTransaction) =>
    runAction(
      () => onUnlinkTransaction(link.id),
      "Não foi possível desvincular a transação.",
      "Transação desvinculada",
    );

  const allocateExistingLink = async (linkId: string, installmentId: string, amount: string) => {
    try {
      await plan.allocateRealization({
        transactionId: installmentId,
        write: { payable_link_id: linkId, allocated_amount: amount },
      });
      feedback.success("Transação alocada");
    } catch (error) {
      const message = errorMessage(error, "Não foi possível alocar a transação.");
      setActionError(message);
      feedback.error(message);
      throw error;
    }
  };

  const submitLink = async () => {
    if (selectedCandidateId === null) return;
    setLinkError(null);
    setLinkSubmitting(true);
    try {
      const link = await onLinkTransaction(selectedCandidateId);
      if (selectedInstallmentId !== null) {
        await plan.allocateRealization({
          transactionId: selectedInstallmentId,
          write: { payable_link_id: link.id, allocated_amount: link.linked_amount },
        });
      }
      setSelectedCandidateId(null);
      setSelectedInstallmentId(null);
      setAddPaymentOpen(false);
      feedback.success("Transação vinculada");
    } catch (error) {
      setLinkError(errorMessage(error, "Não foi possível vincular a transação."));
    } finally {
      setLinkSubmitting(false);
    }
  };

  const closeAddPayment = () => {
    setAddPaymentOpen(false);
    setSelectedCandidateId(null);
    setSelectedInstallmentId(null);
    setLinkError(null);
  };

  // The same section the loaded timeline fills, so the page does not jump.
  if (plan.isLoading) {
    return (
      <Section title="Linha do tempo" className="payable-timeline-section">
        <div role="status" aria-label="Carregando linha do tempo">
          <Skeleton active title={false} paragraph={{ rows: 4 }} />
        </div>
      </Section>
    );
  }

  const installments = plan.plan?.transactions ?? [];
  const allocatedLinkIds = new Set(installments.flatMap((i) => i.realizations.map((r) => r.payable_link_id)));
  const unallocatedLinks = links.filter((link) => !allocatedLinkIds.has(link.id));

  const entries: TimelineEntry[] = [
    ...installments.map((installment) => ({
      kind: "installment" as const,
      date: installment.projected_at,
      installment,
    })),
    ...unallocatedLinks.map((link) => ({
      kind: "link" as const,
      date: link.occurred_at ?? link.linked_at,
      link,
    })),
  ].sort((a, b) => a.date.localeCompare(b.date));

  const hasPlanInstallments = plan.plan !== null && plan.plan.transactions.length > 0;

  // A quiet text action, then the plan's own `···`: adding is the everyday
  // step but not a call to action louder than the figures under it.
  const trailing = (
    <Flex gap={4} align="center">
      <Button
        type="text"
        className="payable-add-action"
        icon={<PlusOutlined aria-hidden="true" />}
        onClick={() => setAddPaymentOpen(true)}
      >
        {vocab.addActionLabel}
      </Button>
      {hasPlanInstallments && (
        <RecordMenu
          className="payable-entry-menu"
          label="Ações do plano"
          loading={plan.isReadjusting}
          items={[
            {
              key: "abater_do_final",
              label: "Reajustar: abater do final",
              onClick: () =>
                confirm({
                  title: "Abater do final",
                  description:
                    "Mantém o valor da parcela; o prazo (número de parcelas restantes) muda para cobrir o saldo.",
                  okText: "Reajustar",
                  danger: false,
                  onConfirm: () => readjust("abater_do_final"),
                }),
            },
            {
              key: "redistribuir",
              label: "Reajustar: redistribuir entre os meses",
              onClick: () =>
                confirm({
                  title: "Redistribuir entre os meses",
                  description:
                    "Mantém o prazo (mesmas datas restantes); o valor de cada parcela muda para cobrir o saldo.",
                  okText: "Reajustar",
                  danger: false,
                  onConfirm: () => readjust("redistribuir"),
                }),
            },
          ]}
        />
      )}
    </Flex>
  );

  return (
    <Section title="Linha do tempo" trailing={trailing} className="payable-timeline-section">
      <Flex vertical gap="middle">
        {actionError && (
          <Alert type="error" showIcon closable onClose={() => setActionError(null)} message={actionError} />
        )}

        {plan.plan === null && (
          <Flex gap="small" align="center" wrap>
            <Typography.Text type="secondary">{vocab.noPlanMessage}</Typography.Text>
            <Button loading={plan.isCreating} onClick={createPlan}>
              {vocab.createPlanButtonText}
            </Button>
          </Flex>
        )}

        {plan.plan !== null && plan.plan.transactions.length === 0 && (
          <Flex vertical gap="small">
            <Typography.Text>{vocab.generateFromRemainingText}</Typography.Text>
            <Flex gap="small" align="center" wrap>
              <Typography.Text>Cadência:</Typography.Text>
              <Radio.Group
                value={cadence}
                onChange={(e) => setCadence(e.target.value as Cadence)}
              >
                <Radio.Button value="mensal">Mensal</Radio.Button>
                <Radio.Button value="semanal">Semanal</Radio.Button>
                <Radio.Button value="quinzenal">Quinzenal</Radio.Button>
              </Radio.Group>
            </Flex>
            <Radio.Group value={generateBy} onChange={(e) => setGenerateBy(e.target.value as "months" | "amount")}>
              <Radio value="months">Número de parcelas</Radio>
              <Radio value="amount">Valor da parcela</Radio>
            </Radio.Group>
            <Flex gap="small" align="center" wrap>
              <Typography.Text>Data de início:</Typography.Text>
              <DatePicker
                aria-label="Data de início"
                value={startDate}
                onChange={(value) => value && setStartDate(value)}
                format="DD/MM/YYYY"
                allowClear={false}
              />
            </Flex>
            <Flex gap="small" align="center" wrap>
              {generateBy === "months" ? (
                <>
                  <InputNumber
                    aria-label="Número de parcelas"
                    min={1}
                    max={520}
                    precision={0}
                    value={months}
                    onChange={(value) => setMonths(value)}
                  />
                  <Typography.Text>parcelas</Typography.Text>
                </>
              ) : (
                <MoneyInput
                  aria-label="Valor de cada parcela"
                  min="0"
                  value={installmentAmount}
                  onChange={(value) => setInstallmentAmount(value)}
                  style={{ width: "10rem" }}
                />
              )}
              <Button type="primary" loading={plan.isGenerating} onClick={generate}>
                Gerar parcelas
              </Button>
            </Flex>
          </Flex>
        )}

        {hasPlanInstallments && plan.plan !== null && (
          <PlanNotice value={plan.plan.accumulated_deviation} onTrackMessage={vocab.onTrackMessage} />
        )}

        {entries.length === 0 ? (
          <Typography.Text type="secondary">Nenhuma parcela ou transação vinculada ainda.</Typography.Text>
        ) : (
          <ul className="payable-entries" aria-label="Linha do tempo">
            {entries.map((entry) => {
              if (entry.kind === "installment") {
                const installment = entry.installment;
                const realizedTotal = sumBRL(installment.realizations.map((r) => r.allocated_amount));
                // When what was actually settled diverges from what was
                // planned (settled more or less than the installment), the
                // realized total is the number that matters — it reads as the
                // headline, with the planned amount demoted to the meta line.
                const divergesFromPlan =
                  installment.realizations.length > 0 && subtractBRL(realizedTotal, installment.amount) !== "0.00";
                const label = installmentLabel(installment.description);
                const tone = installmentStatusTone[installment.status];
                // One `···` per row: what undoes a payment sits next to what
                // deletes the installment, instead of a second menu on the
                // payment line that read as a second thing to tap.
                const menuItems: RecordMenuItem[] = [
                  ...installment.realizations.map((realization) => ({
                    key: `deallocate-${realization.id}`,
                    label:
                      installment.realizations.length === 1
                        ? "Desalocar transação"
                        : `Desalocar ${realizationLabel(realization, links, timezone)}`,
                    danger: true,
                    onClick: () =>
                      confirm({
                        title: "Desalocar transação",
                        description: vocab.deallocateDescription,
                        okText: "Desalocar",
                        danger: true,
                        onConfirm: () => deallocate(installment, realization),
                      }),
                  })),
                  {
                    key: "delete",
                    label: "Excluir parcela",
                    danger: true,
                    onClick: () =>
                      confirm({
                        title: "Excluir parcela",
                        description: "A parcela planejada será removida do plano.",
                        okText: "Excluir",
                        danger: true,
                        onConfirm: () => deleteInstallment(installment),
                      }),
                  },
                ];
                return (
                  <li key={`installment-${installment.id}`} className="payable-entry">
                    <div className="payable-entry-line">
                      <span className="payable-entry-text">
                        <span className="payable-entry-title">
                          <span className="payable-nowrap">{formatShortDate(installment.projected_at, timezone)}</span> ·{" "}
                          {/* Non-breaking: "Parcela 4/6" must not wrap between word and number. */}
                          {label.replace(/^Parcela /, "Parcela\u00a0")}
                        </span>
                        {(tone !== undefined || divergesFromPlan) && (
                          <span className="payable-entry-meta">
                            {tone !== undefined && (
                              <StatusTag tone={tone}>{scenarioTransactionStatusLabel[installment.status]}</StatusTag>
                            )}
                            {divergesFromPlan && <>planejado {formatBRL(installment.amount)}</>}
                          </span>
                        )}
                      </span>
                      <span className="payable-entry-amount">
                        <Money
                          value={divergesFromPlan ? realizedTotal : installment.amount}
                          tone="neutral"
                          size="row"
                        />
                      </span>
                      <RecordMenu
                        className="payable-entry-menu"
                        label={`Ações da ${label.toLowerCase()}`}
                        items={menuItems}
                      />
                    </div>
                    {installment.realizations.map((realization) => (
                      <div key={realization.id} className="payable-entry-line payable-entry-realization">
                        <span className="payable-entry-text">
                          <span className="payable-entry-realization-text">
                            ↳ {realizationLabel(realization, links, timezone)}
                          </span>
                        </span>
                        {/* A lone realization that matches the installment would only repeat its figure. */}
                        <span className="payable-entry-amount">
                          {(divergesFromPlan || installment.realizations.length > 1) && (
                            <Money value={realization.allocated_amount} tone="neutral" />
                          )}
                        </span>
                        <span className="payable-entry-menu-spacer" aria-hidden="true" />
                      </div>
                    ))}
                  </li>
                );
              }

              const link = entry.link;
              return (
                <li key={`link-${link.id}`} className="payable-entry">
                  <div className="payable-entry-line">
                    <span className="payable-entry-text">
                      <span className="payable-entry-title">
                        {formatShortDate(link.occurred_at ?? link.linked_at, timezone)} · {link.description ?? "Sem descrição"}
                      </span>
                      <span className="payable-entry-meta">Vínculo avulso</span>
                    </span>
                    <span className="payable-entry-amount">
                      <Money value={link.current_amount} tone="neutral" size="row" />
                    </span>
                    <RecordMenu
                      className="payable-entry-menu"
                      label="Ações do vínculo"
                      items={[
                        {
                          key: "unlink",
                          label: "Desvincular",
                          danger: true,
                          onClick: () =>
                            confirm({
                              title: "Desvincular transação",
                              description:
                                "A transação permanece inalterada e volta a ficar disponível para vincular.",
                              okText: "Desvincular",
                              danger: true,
                              onConfirm: () => unlink(link),
                            }),
                        },
                      ]}
                    />
                  </div>
                  {installments.length > 0 && (
                    <AllocateExistingLinkControl
                      link={link}
                      kind={kind}
                      installments={installments}
                      onAllocate={(installmentId, amount) => allocateExistingLink(link.id, installmentId, amount)}
                    />
                  )}
                </li>
              );
            })}
          </ul>
        )}
      </Flex>

      {/* A form shell, not a centered dialog: on a phone it is a full-screen sheet with a 48px Salvar. */}
      <FormDrawer
        title={vocab.addActionLabel}
        open={addPaymentOpen}
        onClose={closeAddPayment}
        onSubmit={submitLink}
        submitLabel="Vincular"
        submitting={isLinking || linkSubmitting}
        submitDisabled={selectedCandidateId === null}
        error={linkError}
      >
        <FormField label="Transação" labelId="payable-link-transaction-label">
          <TransactionPicker
            id="payable-link-transaction"
            label="Buscar transação para vincular"
            allowClear
            placeholder="Buscar por descrição"
            value={selectedCandidateId}
            search={{ value: search, onChange: onSearchChange }}
            loading={isSearching}
            emptyText="Nenhuma transação elegível encontrada"
            transactions={candidates.map((candidate) => candidateRow(candidate, kind))}
            onSelect={setSelectedCandidateId}
          />
        </FormField>
        {installments.length > 0 && (
          <FormField label="Parcela (opcional)" labelId="payable-link-installment-label">
            <TransactionPicker
              id="payable-link-installment"
              label="Parcela para alocar"
              allowClear
              placeholder="Sem parcela (vínculo avulso)"
              value={selectedInstallmentId}
              transactions={installments.map((installment) => installmentRow(installment, kind))}
              onSelect={setSelectedInstallmentId}
            />
          </FormField>
        )}
      </FormDrawer>
    </Section>
  );
}
