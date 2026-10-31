import { Alert, Button, Radio, Select, Skeleton } from "../components/ui";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { listAccountCurrencies } from "../api/accounts";
import { discardAfterTimezoneChange, queryKeys } from "../api/queryKeys";
import { FormField } from "../components/forms/FormField";
import { useMemo, useState } from "react";

import { updatePreferences } from "../api/preferences";
import { defaultTimezone, type Preferences, type TransactionsPeriodBasis } from "../api/contracts";
import { UnavailableState } from "../components/AsyncState";
import { Section } from "../components/layout";
import { SettingsPageContainer as PageContainer } from "../components/SettingsPageContainer";
import { useFeedback } from "../components/shared/useFeedback";
import { usePreferences } from "../hooks/usePreferences";
import { timezoneChoices } from "../presentation/timezone";

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error ? error.message : fallback;
}

export function GeneralSettingsPage() {
  const client = useQueryClient();
  const currencies = useQuery({
    queryKey: ["account-currencies"],
    queryFn: ({ signal }) => listAccountCurrencies(signal),
  });
  const names = new Intl.DisplayNames(["pt-BR"], { type: "currency" });
  const preferences = usePreferences();
  const saved = preferences.data;
  // What the person changed since the last save; the rest follows the server.
  const [edits, setEdits] = useState<Partial<Preferences>>({});
  const currency = edits.reference_currency ?? saved?.reference_currency ?? "BRL";
  const timezone = edits.timezone ?? saved?.timezone ?? defaultTimezone;
  const basis = edits.transactions_period_basis ?? saved?.transactions_period_basis ?? null;
  const zoneOptions = useMemo(
    () =>
      timezoneChoices(saved?.timezone).map((zone) => ({
        value: zone,
        label: zone,
        searchText: `${zone} ${zone.replaceAll("_", " ")}`,
      })),
    [saved?.timezone],
  );
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const feedback = useFeedback();

  const save = async () => {
    if (saved === undefined || basis === null) return;
    setSaving(true);
    setSaveError(null);
    try {
      const next = await updatePreferences({
        transactions_period_basis: basis,
        reference_currency: currency,
        timezone,
      });
      client.setQueryData(queryKeys.preferences, next);
      setEdits({});
      if (next.timezone !== saved.timezone) await discardAfterTimezoneChange(client);
      for (const queryKey of [
        queryKeys.timeline,
        queryKeys.netWorth,
        queryKeys.transactions,
        queryKeys.investmentWorkspacePart("summary"),
        queryKeys.investmentPeriod,
      ]) {
        await client.cancelQueries({ queryKey });
        client.removeQueries({ queryKey });
      }
      feedback.success("Preferências salvas");
    } catch (error) {
      const message = errorMessage(error, "Não foi possível salvar as preferências.");
      setSaveError(message);
      feedback.error(message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <PageContainer title="Geral" subTitle="Preferências gerais do Julius" compactMobileHeader>
      <Section title="Preferências">
        {preferences.isPending ? (
          <Skeleton active paragraph={{ rows: 2 }} />
        ) : saved === undefined ? (
          // A fixed sentence: whatever the transport said is not for the user.
          <UnavailableState onRetry={() => void preferences.refetch()}>
            Não foi possível carregar as preferências.
          </UnavailableState>
        ) : (
          <div className="settings-form">
            <FormField
              label="Moeda de referência"
              htmlFor="reference-currency"
              hint="Moeda usada nos totais e relatórios. Os valores originais de contas, transações e investimentos são preservados."
            >
              <Select
                id="reference-currency"
                aria-describedby="reference-currency-hint"
                value={currency}
                onChange={(code: string) => setEdits((current) => ({ ...current, reference_currency: code }))}
                disabled={saving}
                loading={currencies.isLoading}
                showSearch
                optionFilterProp="label"
                style={{ width: "100%" }}
                options={(currencies.data?.currencies.map((item) => item.code) ?? [currency]).map((code) => ({
                  value: code,
                  label: `${code} — ${names.of(code) ?? code}`,
                }))}
              />
            </FormField>
            {currencies.isError && (
              <Alert
                type="error"
                message="Não foi possível carregar as moedas."
                action={<Button onClick={() => currencies.refetch()}>Tentar novamente</Button>}
              />
            )}
            <FormField
              label="Fuso horário"
              htmlFor="timezone"
              hint="Define o que conta como “hoje”, onde cada mês começa e termina e em que dia cada transação aparece."
            >
              <Select
                id="timezone"
                aria-describedby="timezone-hint"
                value={timezone}
                onChange={(zone: string) => setEdits((current) => ({ ...current, timezone: zone }))}
                disabled={saving}
                showSearch
                optionFilterProp="searchText"
                style={{ width: "100%" }}
                options={zoneOptions}
              />
            </FormField>
            <h3>Mês das transações de cartão de crédito</h3>
            <p className="settings-radio-hint">
              Escolha se as compras no cartão contam no mês em que foram feitas ou no mês da fatura em que são
              pagas. Transações de débito e de outras contas não têm essa distinção.
            </p>
            {saveError && <Alert type="error" showIcon message={saveError} />}
            <Radio.Group
              aria-label="Mês das transações de cartão de crédito"
              disabled={saving}
              value={basis}
              onChange={(event) =>
                setEdits((current) => ({
                  ...current,
                  transactions_period_basis: event.target.value as TransactionsPeriodBasis,
                }))
              }
            >
              <div className="settings-radio">
                <Radio value="occurred_at">
                  <span className="settings-radio-body">
                    Mês da compra
                    <span className="settings-radio-hint">Cada compra conta no mês em que foi realizada.</span>
                  </span>
                </Radio>
                <Radio value="paid_at">
                  <span className="settings-radio-body">
                    Mês do pagamento
                    <span className="settings-radio-hint">
                      Cada compra conta no mês da fatura em que é efetivamente paga. Compras recentes, cuja
                      fatura ainda não fechou, usam uma estimativa que se ajusta sozinha assim que a fatura
                      fecha numa sincronização seguinte.
                    </span>
                  </span>
                </Radio>
              </div>
            </Radio.Group>
            <Button type="primary" loading={saving} disabled={!currencies.data} onClick={save}>
              Salvar
            </Button>
          </div>
        )}
      </Section>
    </PageContainer>
  );
}
