import { hasNetWorthValue } from "../api/contracts";
import { ApiError } from "../api/problems";
import { ReportValuation } from "../components/shared/ReportValuation";
import { Skeleton } from "antd";

import { UnavailableState } from "../components/AsyncState";
import { EmptyState, Page, Section } from "../components/layout";
import { NetWorthBreakdownCard } from "../components/netWorth/NetWorthBreakdownCard";
import { NetWorthChart } from "../components/netWorth/NetWorthChart";
import { useNetWorth } from "../hooks/useNetWorth";

export function NetWorthPage() {
  const netWorth = useNetWorth();
  // A single point has no evolution to plot — a line chart with one dot
  // reads as broken, not "just started". Wait for at least two days of
  // history before showing the chart.
  const hasSeries = netWorth.series.filter(hasNetWorthValue).length > 1;
  const hasStaleRates = netWorth.series.some((snapshot) => snapshot.fx?.stale);
  const hasBackfilledPoints = netWorth.series.some((s) => s.is_backfilled);
  const hasLegacyPoints = netWorth.series.some((s) => s.valuation_status === "legacy");
  const hasUnavailablePoints = netWorth.series.some((s) => s.valuation_status === "unavailable");

  // One footnote, under the chart it qualifies, for what used to be a
  // paragraph above the figures plus an info Alert below the chart.
  const footnote = `Dias sem registro são reconstruídos a partir das transações, então a série pode não cobrir todo o passado.${
    hasBackfilledPoints ? " Esses dias não incluem investimentos: não há como saber quanto deles já existia." : ""
  }${hasStaleRates ? " Há dias avaliados com cotações desatualizadas." : ""}${
    hasLegacyPoints ? " Registros antigos cobrem apenas contas e investimentos em reais, sem outras moedas." : ""
  }${hasUnavailablePoints ? " Dias sem cotação ou sem histórico suficiente aparecem como lacunas." : ""}`;

  return (
    <Page title="Patrimônio líquido" description="Ativos menos passivos ao longo do tempo" compactMobileHeader>
      {netWorth.isLoading && (
        // Hero strip, then the chart: the shape of the loaded page.
        <div className="net-worth-page" role="status" aria-label="Carregando patrimônio líquido">
          <Skeleton active title={{ width: "30%" }} paragraph={{ rows: 2 }} />
          <Skeleton.Node active style={{ width: "100%", height: 260 }}>
            <span />
          </Skeleton.Node>
        </div>
      )}
      {netWorth.error && !netWorth.isLoading && (
        <UnavailableState onRetry={() => netWorth.refetch()}>
          {netWorth.error instanceof ApiError
            ? netWorth.error.message
            : "Não foi possível carregar o patrimônio líquido"}
        </UnavailableState>
      )}

      {!netWorth.isLoading && !netWorth.error && !netWorth.latest && (
        <EmptyState
          title="Ainda não há patrimônio para mostrar"
          hint="Conecte um banco ou importe um extrato: o patrimônio é calculado a partir das suas contas."
        />
      )}

      {!netWorth.isLoading && !netWorth.error && netWorth.latest && (
        <ReportValuation valuation={netWorth.latest}>
          <div className="net-worth-page">
            {hasNetWorthValue(netWorth.latest) ? (
              <NetWorthBreakdownCard snapshot={netWorth.latest} />
            ) : (
              <EmptyState
                title="Patrimônio indisponível"
                hint="Faltam cotações de câmbio para avaliar todos os saldos em reais."
              />
            )}
            {hasSeries ? (
              <NetWorthChart
                snapshots={netWorth.series}
                footnote={`Valores em ${netWorth.latest.currency_code ?? "BRL"}, com câmbio de cada dia. ${footnote}`}
              />
            ) : (
              <Section title="Evolução do patrimônio líquido">
                <EmptyState
                  title="Ainda não há histórico para mostrar"
                  hint="O gráfico aparece a partir do segundo dia com dados registrados."
                />
              </Section>
            )}
          </div>
        </ReportValuation>
      )}
    </Page>
  );
}
