import { useTranslation } from "../i18n/context";
import { translate, type Language } from "../i18n/messages";
import { Alert, Button, Input, Select, Skeleton, Switch, Typography } from "./ui";
import { useEffect, useState } from "react";
import type { QuoteLastRefresh, QuoteSchedule } from "../api/contracts";
import { ApiError, isAbortError } from "../api/problems";
import { getQuoteSettings, refreshQuotesNow, updateQuoteSchedule } from "../api/quoteSettings";
import { apiFetch } from "../api/transport";
import { formatOptionalDateTime } from "../presentation/dates";
import { UnavailableState } from "./AsyncState";
import { FormField } from "./forms/FormField";
import { Section } from "./layout";

const timezones = [
  "America/Sao_Paulo",
  "America/Manaus",
  "America/Cuiaba",
  "America/Belem",
  "America/Fortaleza",
  "America/Recife",
  "America/Noronha",
  "America/Rio_Branco",
  "UTC",
];

function refreshSummary(last: QuoteLastRefresh, language: Language): string {
  const t = (key: Parameters<typeof translate>[1], values?: Record<string, string | number>) => translate(language, key, values);
  if (last.status === "failed") return t("quoteRefreshFailed");
  if (last.assets === 0) return t("quoteNoAssets");
  const priced = t("quotePriced", { fetched: last.fetched, assets: last.assets });
  switch (last.status) {
    case "success":
      return `${priced}.`;
    case "partial":
      return last.deferred > 0
        ? t("quotePartialDeferred", { priced, failures: last.failures, deferred: last.deferred })
        : t("quotePartial", { priced, failures: last.failures });
    case "deferred":
      return t("quoteDeferred", { priced, deferred: last.deferred });
  }
}

function LastRefresh({ last }: { last: QuoteLastRefresh | null }) {
  const { t, language } = useTranslation();
  if (last === null) return <span>{t("noUpdateYet")}</span>;
  const tone =
    last.status === "failed"
      ? "danger"
      : last.assets === 0 || last.status === "success"
        ? undefined
        : "warning";
  return (
    <>
      <span>
        {formatOptionalDateTime(last.finished_at, language)} · {last.trigger === "manual" ? t("triggerManual") : t("triggerScheduled")}
      </span>
      <Typography.Text type={tone}>{refreshSummary(last, language)}</Typography.Text>
    </>
  );
}

export function QuotesSettings() {
  const { t } = useTranslation();
  const [schedule, setSchedule] = useState<QuoteSchedule | null>(null);
  const [last, setLast] = useState<QuoteLastRefresh | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [refreshError, setRefreshError] = useState<"conflict" | "failed" | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setLoadError(false);
    getQuoteSettings(controller.signal)
      .then(({ last_refresh, ...config }) => {
        setSchedule(config);
        setLast(last_refresh);
      })
      .catch((error) => {
        if (!isAbortError(error)) setLoadError(true);
      })
      .finally(() => setLoading(false));
    return () => controller.abort();
  }, [attempt]);

  const edit = (change: Partial<QuoteSchedule>) => {
    setSchedule((current) => (current ? { ...current, ...change } : current));
    setSaved(false);
  };

  const save = async () => {
    if (!schedule) return;
    setSaving(true);
    setSaveError(null);
    setSaved(false);
    try {
      await updateQuoteSchedule(schedule);
      setSaved(true);
    } catch (error) {
      if (isAbortError(error)) return;
      setSaveError(
        error instanceof ApiError && error.problem?.status === 422
          ? t("quoteScheduleCheck")
          : t("quoteScheduleSaveError"),
      );
    } finally {
      setSaving(false);
    }
  };

  const refresh = async () => {
    setRefreshing(true);
    setRefreshError(null);
    try {
      setLast(await refreshQuotesNow());
    } catch (error) {
      if (isAbortError(error)) return;
      setRefreshError(error instanceof ApiError && error.kind === "conflict" ? "conflict" : "failed");
    } finally {
      setRefreshing(false);
    }
  };

  const zoneOptions = (schedule && !timezones.includes(schedule.timezone) ? [schedule.timezone, ...timezones] : timezones).map(
    (zone) => ({ value: zone, label: zone }),
  );

  return (
    <Section title={t("automaticQuotes")}>
      <div className="settings-form">
        <p className="settings-radio-hint">
          {t("quotesDailyHint")}
        </p>
        {loading ? (
          <Skeleton active paragraph={{ rows: 2 }} />
        ) : loadError || !schedule ? (
          <UnavailableState onRetry={() => setAttempt((count) => count + 1)}>
            {t("quoteSettingsLoadError")}
          </UnavailableState>
        ) : (
          <>
            <div className="quotes-last-refresh" aria-live="polite" aria-atomic="true">
              <span className="form-field-label">{t("lastUpdate")}</span>
              <LastRefresh last={last} />
              {refreshError &&
                (refreshError === "conflict" ? (
                  <Alert
                    type="info"
                    showIcon
                    message={t("updateInProgress")}
                  />
                ) : (
                  <Alert type="error" showIcon message={t("quotesUpdateError")} />
                ))}
            </div>
            <Button onClick={() => void refresh()} loading={refreshing} aria-busy={refreshing}>
              {refreshing ? t("updatingEllipsis") : t("updateNow")}
            </Button>
            {refreshing && <p className="settings-radio-hint">{t("mayTakeSeconds")}</p>}
          </>
        )}
      </div>
      {schedule && !loading && !loadError && (
        <form
          className="settings-form quotes-schedule-form"
          onSubmit={(event) => {
            event.preventDefault();
            void save();
          }}
        >
          <FormField
            label={t("updateAutomatically")}
            htmlFor="quotes-enabled"
            hint={t("updateAutomaticallyHint")}
          >
            <div>
              <Switch
                id="quotes-enabled"
                aria-describedby="quotes-enabled-hint"
                checked={schedule.enabled}
                disabled={saving}
                onChange={(enabled) => edit({ enabled })}
              />
            </div>
          </FormField>
          <FormField label={t("timeLabel")} htmlFor="quotes-time">
            <Input
              id="quotes-time"
              type="time"
              step={60}
              required
              value={schedule.time}
              disabled={!schedule.enabled || saving}
              onChange={(event) => edit({ time: event.target.value })}
            />
          </FormField>
          <FormField label={t("timeZoneLabel")} htmlFor="quotes-timezone">
            <Select
              id="quotes-timezone"
              showSearch
              options={zoneOptions}
              value={schedule.timezone}
              disabled={!schedule.enabled || saving}
              onChange={(timezone: string) => edit({ timezone })}
            />
          </FormField>
          {saveError && <Alert type="error" showIcon message={saveError} />}
          {saved && <Alert type="success" showIcon message={t("scheduleSaved")} />}
          <Button type="primary" htmlType="submit" loading={saving}>
            {t("saveSchedule")}
          </Button>
        </form>
      )}
      <details className="quotes-advanced">
        <summary>{t("quoteProviderAdvanced")}</summary>
        <BrapiTokenForm />
      </details>
    </Section>
  );
}

// Write-only like PluggySettings: the saved token is never echoed back.
function BrapiTokenForm() {
  const { t } = useTranslation();
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const save = async () => {
    setBusy(true);
    setError(null);
    setSaved(false);
    try {
      const response = await apiFetch("/api/settings/quotes", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ brapi_token: token }),
      });
      if (!response.ok) {
        // Surface the server's own sentence (e.g. the demo-mode 403).
        const problem = (await response.json().catch(() => null)) as { detail?: string } | null;
        throw new Error(problem?.detail || t("tokenSaveError"));
      }
      setToken("");
      setSaved(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("saveFailedGeneric"));
    } finally {
      setBusy(false);
    }
  };
  return (
    <form
      className="settings-form"
      onSubmit={(event) => {
        event.preventDefault();
        void save();
      }}
    >
      <p className="settings-radio-hint">
        {t("brapiHint")}
      </p>
      {error && <Alert type="error" message={error} />}
      {saved && <Alert type="success" message={t("tokenSaved")} />}
      <FormField label={t("brapiTokenOptional")} htmlFor="quotes-brapi-token">
        <Input.Password
          id="quotes-brapi-token"
          autoComplete="off"
          value={token}
          onChange={(e) => setToken(e.target.value)}
        />
      </FormField>
      <Button htmlType="submit" loading={busy}>
        {t("saveToken")}
      </Button>
    </form>
  );
}
