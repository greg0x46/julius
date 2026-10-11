import { hasNetWorthValue } from "../api/contracts";
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
  const hasBackfilledPoints = netWorth.series.some((s) => s.is_backfilled);

  // One footnote, under the chart it qualifies, for what used to be a
  // paragraph above the figures plus an info Alert below the chart.
  const footnote = `Valores em reais, usando o câmbio disponível até a data de cada avaliação. Dias sem taxas de câmbio ou sem contexto de avaliação não são exibidos.${
    hasBackfilledPoints ? " Dias reconstruídos não incluem investimentos históricos." : ""
  }`;

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
          Não foi possível carregar o patrimônio líquido
        </UnavailableState>
      )}

      {!netWorth.isLoading && !netWorth.error && !netWorth.latest && (
        <EmptyState
          title="Ainda não há patrimônio para mostrar"
          hint="Conecte um banco ou importe um extrato: o patrimônio é calculado a partir das suas contas."
        />
      )}

      {!netWorth.isLoading && !netWorth.error && netWorth.latest && (
        <div className="net-worth-page">
          {hasNetWorthValue(netWorth.latest) ? <NetWorthBreakdownCard snapshot={netWorth.latest} /> : (
            <EmptyState
              title={netWorth.latest.valuation_status === "unavailable" ? "Patrimônio indisponível" : "Registro anterior sem contexto de avaliação"}
              hint={netWorth.latest.valuation_status === "unavailable" ? "Faltam taxas de câmbio para avaliar todos os saldos em reais." : "Este registro não informa a moeda nem as taxas usadas."}
            />
          )}
          {hasSeries ? (
            <NetWorthChart snapshots={netWorth.series} footnote={footnote} />
          ) : (
            <Section title="Evolução do patrimônio líquido">
              <EmptyState
                title="Ainda não há histórico avaliado para mostrar"
                hint="O gráfico aparece a partir do segundo dia com avaliação disponível."
              />
              <p className="net-worth-footnote">{footnote}</p>
            </Section>
          )}
        </div>
      )}
    </Page>
  );
}
