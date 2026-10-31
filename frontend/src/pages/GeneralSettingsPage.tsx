import { useTranslation } from "../i18n/context";
import { Alert, Button, Radio, Select, Skeleton } from "../components/ui";
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
  const { t, language } = useTranslation();
  const client = useQueryClient();
  const currencies = useQuery({
    queryKey: ["account-currencies"],
    queryFn: ({ signal }) => listAccountCurrencies(signal),
  });
  const names = new Intl.DisplayNames([language], { type: "currency" });
  const [currency, setCurrency] = useState("BRL");
  const [basis, setBasis] = useState<TransactionsPeriodBasis | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);
  const feedback = useFeedback();

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setLoadError(false);
    getPreferences(controller.signal)
      .then((preferences) => {
        setBasis(preferences.transactions_period_basis);
        setCurrency(preferences.reference_currency);
      })
      .catch((error) => {
        if (error instanceof DOMException && error.name === "AbortError") return;
        // A fixed sentence: whatever the transport said is not for the user.
        setLoadError(true);
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
      feedback.success(t("preferencesSaved"));
    } catch (error) {
      const message = errorMessage(error, t("preferencesSaveError"));
      setSaveError(message);
      feedback.error(message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <PageContainer title={t("general")} subTitle={t("generalSubtitle")} compactMobileHeader>
      <Section title={t("preferences")}>
        {loading ? (
          <Skeleton active paragraph={{ rows: 2 }} />
        ) : loadError ? (
          <UnavailableState onRetry={() => setAttempt((count) => count + 1)}>{t("preferencesLoadError")}</UnavailableState>
        ) : (
          <div className="settings-form">
            <FormField
              label={t("referenceCurrency")}
              htmlFor="reference-currency"
              hint={t("referenceCurrencyHint")}
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
                options={(currencies.data?.currencies.map((item) => item.code) ?? [currency]).map((code) => ({
                  value: code,
                  label: `${code} — ${names.of(code) ?? code}`,
                }))}
              />
            </FormField>
            {currencies.isError && (
              <Alert
                type="error"
                message={t("currenciesLoadError")}
                action={<Button onClick={() => currencies.refetch()}>{t("retry")}</Button>}
              />
            )}
            <h3>{t("cardTransactionMonth")}</h3>
            <p className="settings-radio-hint">
              {t("cardMonthHint")}
            </p>
            {saveError && <Alert type="error" showIcon message={saveError} />}
            <Radio.Group
              aria-label={t("cardTransactionMonth")}
              disabled={saving}
              value={basis}
              onChange={(event) => setBasis(event.target.value as TransactionsPeriodBasis)}
            >
              <div className="settings-radio">
                <Radio value="occurred_at">
                  <span className="settings-radio-body">
                    {t("purchaseMonth")}
                    <span className="settings-radio-hint">{t("purchaseMonthHint")}</span>
                  </span>
                </Radio>
                <Radio value="paid_at">
                  <span className="settings-radio-body">
                    {t("paymentMonth")}
                    <span className="settings-radio-hint">
                      {t("paymentMonthHint")}
                    </span>
                  </span>
                </Radio>
              </div>
            </Radio.Group>
            <Button type="primary" loading={saving} disabled={!currencies.data} onClick={save}>
              {t("save")}
            </Button>
          </div>
        )}
      </Section>
    </PageContainer>
  );
}
