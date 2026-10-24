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
  // reads as broken, not "just started". Wait for at least two valued days.
  const hasSeries = netWorth.series.filter(hasNetWorthValue).length > 1;
  const hasStaleRates = netWorth.series.some((point) => point.fx?.stale);
  const hasUnavailablePoints = netWorth.series.some((point) => point.valuation_status === "unavailable");

  // One footnote, under the chart it qualifies.
  const footnote = `O histórico é recalculado a cada leitura a partir das transações, operações e cotações, então acompanha qualquer correção.${
    hasStaleRates ? " Há dias avaliados com cotações desatualizadas." : ""
  }${hasUnavailablePoints ? " Dias sem cotação, sem câmbio ou sem histórico suficiente aparecem como lacunas." : ""}`;

  return (
    <Page title="Patrimônio" description="O que você já tem, em caixa e investimentos, ao longo do tempo" compactMobileHeader>
      {netWorth.isLoading && (
        // Hero strip, then the chart: the shape of the loaded page.
        <div className="net-worth-page" role="status" aria-label="Carregando patrimônio">
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
            : "Não foi possível carregar o patrimônio"}
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
              <NetWorthBreakdownCard point={netWorth.latest} />
            ) : (
              <EmptyState
                title="Patrimônio indisponível"
                hint="Faltam cotações de câmbio para avaliar todos os saldos em reais."
              />
            )}
            {hasSeries ? (
              <NetWorthChart
                points={netWorth.series}
                footnote={`Valores em ${netWorth.latest.currency_code ?? "BRL"}, com câmbio de cada dia. ${footnote}`}
              />
            ) : (
              <Section title="Evolução do patrimônio">
                <EmptyState
                  title="Ainda não há histórico para mostrar"
                  hint="O gráfico aparece quando há movimentações em pelo menos dois dias."
                />
              </Section>
            )}
          </div>
        </ReportValuation>
      )}
    </Page>
  );
}
