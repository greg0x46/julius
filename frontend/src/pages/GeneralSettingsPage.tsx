import { Alert, Button, Radio, Select, Skeleton } from "antd";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { listAccountCurrencies } from "../api/accounts";
import { queryKeys } from "../api/queryKeys";
import { FormField } from "../components/forms/FormField";
import { useEffect, useState } from "react";

import { getPreferences, updatePreferences } from "../api/preferences";
import type { TransactionsPeriodBasis } from "../api/contracts";
import { UnavailableState } from "../components/AsyncState";
import { Section } from "../components/layout";
import { SettingsPageContainer as PageContainer } from "../components/SettingsPageContainer";
import { useFeedback } from "../components/shared/useFeedback";

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
  const [currency, setCurrency] = useState("BRL");
  const [basis, setBasis] = useState<TransactionsPeriodBasis | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);
  const feedback = useFeedback();

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setLoadError(null);
    getPreferences(controller.signal)
      .then((preferences) => {
        setBasis(preferences.transactions_period_basis);
        setCurrency(preferences.reference_currency);
      })
      .catch((error) => {
        if (error instanceof DOMException && error.name === "AbortError") return;
        // A fixed sentence: whatever the transport said is not for the user.
        setLoadError("Não foi possível carregar as preferências.");
      })
      .finally(() => setLoading(false));
    return () => controller.abort();
  }, [attempt]);

  const save = async () => {
    if (basis === null) return;
    setSaving(true);
    setSaveError(null);
    try {
      await updatePreferences({
        transactions_period_basis: basis,
        reference_currency: currency,
      });
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
        {loading ? (
          <Skeleton active paragraph={{ rows: 2 }} />
        ) : loadError ? (
          <UnavailableState onRetry={() => setAttempt((count) => count + 1)}>{loadError}</UnavailableState>
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
                onChange={setCurrency}
                disabled={saving}
                loading={currencies.isLoading}
                showSearch
                optionFilterProp="label"
                style={{ width: "100%" }}
                options={(currencies.data ?? [currency]).map((code) => ({
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
              onChange={(event) => setBasis(event.target.value as TransactionsPeriodBasis)}
            >
              <div className="settings-radio">
                <Radio value="occurred_at">
                  Mês da compra
                  <span className="settings-radio-hint">Cada compra conta no mês em que foi realizada.</span>
                </Radio>
                <Radio value="paid_at">
                  Mês do pagamento
                  <span className="settings-radio-hint">
                    Cada compra conta no mês da fatura em que é efetivamente paga. Compras recentes, cuja
                    fatura ainda não fechou, usam uma estimativa que se ajusta sozinha assim que a fatura
                    fecha numa sincronização seguinte.
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
