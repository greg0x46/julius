import { Alert, Button, Input, Select, Skeleton, Switch, Typography } from "antd";
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

function refreshSummary(last: QuoteLastRefresh): string {
  if (last.status === "failed") return "Não foi possível concluir a atualização. Tente novamente mais tarde.";
  if (last.assets === 0) return "Nenhum ativo com cotação automática para atualizar.";
  const priced = `${last.fetched} de ${last.assets} ativos cotados`;
  switch (last.status) {
    case "success":
      return `${priced}.`;
    case "partial":
      return last.deferred > 0
        ? `${priced}; ${last.failures} falharam; ${last.deferred} adiados porque o provedor limitou as consultas. Tente novamente mais tarde.`
        : `${priced}; ${last.failures} falharam.`;
    case "deferred":
      return `${priced}; ${last.deferred} adiados porque o provedor limitou as consultas. Tente novamente mais tarde.`;
  }
}

function LastRefresh({ last }: { last: QuoteLastRefresh | null }) {
  if (last === null) return <span>Nenhuma atualização ainda.</span>;
  const tone =
    last.status === "failed"
      ? "danger"
      : last.assets === 0 || last.status === "success"
        ? undefined
        : "warning";
  return (
    <>
      <span>
        {formatOptionalDateTime(last.finished_at)} · {last.trigger === "manual" ? "Manual" : "Agendada"}
      </span>
      <Typography.Text type={tone}>{refreshSummary(last)}</Typography.Text>
    </>
  );
}

export function QuotesSettings() {
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
          ? "Confira o horário e o fuso horário."
          : "Não foi possível salvar o agendamento.",
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
    <Section title="Cotações automáticas">
      <div className="settings-form">
        <p className="settings-radio-hint">
          Os preços das posições com cotação automática são atualizados uma vez por dia no horário escolhido.
        </p>
        {loading ? (
          <Skeleton active paragraph={{ rows: 2 }} />
        ) : loadError || !schedule ? (
          <UnavailableState onRetry={() => setAttempt((count) => count + 1)}>
            Não foi possível carregar as configurações de cotação.
          </UnavailableState>
        ) : (
          <>
            <div className="quotes-last-refresh" aria-live="polite" aria-atomic="true">
              <span className="form-field-label">Última atualização</span>
              <LastRefresh last={last} />
              {refreshError &&
                (refreshError === "conflict" ? (
                  <Alert
                    type="info"
                    showIcon
                    message="Já existe uma atualização em andamento. Aguarde alguns instantes e confira de novo."
                  />
                ) : (
                  <Alert type="error" showIcon message="Não foi possível atualizar as cotações agora." />
                ))}
            </div>
            <Button onClick={() => void refresh()} loading={refreshing} aria-busy={refreshing}>
              {refreshing ? "Atualizando…" : "Atualizar agora"}
            </Button>
            {refreshing && <p className="settings-radio-hint">Isso pode levar alguns segundos.</p>}
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
            label="Atualizar automaticamente"
            htmlFor="quotes-enabled"
            hint="Desligado, os preços só mudam quando você atualizar manualmente."
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
          <FormField label="Horário" htmlFor="quotes-time">
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
          <FormField label="Fuso horário" htmlFor="quotes-timezone">
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
          {saved && <Alert type="success" showIcon message="Agendamento salvo." />}
          <Button type="primary" htmlType="submit" loading={saving}>
            Salvar agendamento
          </Button>
        </form>
      )}
      <details className="quotes-advanced">
        <summary>Provedor de cotações (avançado)</summary>
        <BrapiTokenForm />
      </details>
    </Section>
  );
}

// Write-only like PluggySettings: the saved token is never echoed back.
function BrapiTokenForm() {
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
      if (!response.ok) throw new Error("Não foi possível salvar o token.");
      setToken("");
      setSaved(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Não foi possível salvar.");
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
        Por padrão, a cotação automática de posições manuais usa o Yahoo Finance como fonte principal; a brapi (B3) e
        a CoinGecko (criptomoedas) são usadas quando ele falha. O token da brapi só importa nesse fallback da B3: sem
        ele, apenas um conjunto restrito de tickers responde. O valor salvo não é exibido novamente.
      </p>
      {error && <Alert type="error" message={error} />}
      {saved && <Alert type="success" message="Token salvo." />}
      <FormField label="Token da brapi (opcional)" htmlFor="quotes-brapi-token">
        <Input.Password
          id="quotes-brapi-token"
          autoComplete="off"
          value={token}
          onChange={(e) => setToken(e.target.value)}
        />
      </FormField>
      <Button htmlType="submit" loading={busy}>
        Salvar token
      </Button>
    </form>
  );
}
