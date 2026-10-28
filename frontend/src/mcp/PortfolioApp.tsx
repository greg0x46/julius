import { useState } from "react";
import type { InvestmentPeriod } from "../api/contracts/investmentPeriod";
import { InvestmentPeriodSummary } from "../components/investments/InvestmentPeriodSummary";
import { InvestmentPositionsPeriod } from "../components/investments/InvestmentPositionsPeriod";
import { EmptyState, ListToolbar, SearchField, Section } from "../components/layout";
import { ReportValuation } from "../components/shared/ReportValuation";
import { Alert, Switch } from "../components/ui";
import { formatDateOnly } from "../presentation/dates";

export type PortfolioState = { data: InvestmentPeriod } | { error: true } | null;

export function PortfolioApp({ state }: { state: PortfolioState }) {
  const [search, setSearch] = useState("");
  const [showClosed, setShowClosed] = useState(false);
  const data = state && "data" in state ? state.data : null;
  return (
    <main className="mcp-portfolio">
      <h1>Carteira de investimentos</h1>
      {!state && <p role="status">Aguardando os dados da carteira…</p>}
      {state && "error" in state && <Alert type="error" message="Não foi possível exibir a carteira." description="Consulte o resultado da ferramenta para ver os dados disponíveis." />}
      {data && <>
        <p role="status">{data.from ? formatDateOnly(data.from) : "Desde o início"} até {formatDateOnly(data.to)}</p>
        <ReportValuation valuation={data}>
          <InvestmentPeriodSummary data={data} positions={data.groups.flatMap((group) => group.positions)} updating={false} />
          <Section title="Posições" className="investment-positions-section">
            <ListToolbar label="Controles das posições"
              start={<SearchField id="investment-search" label="Buscar ativo" placeholder="Nome ou ticker" value={search} onChange={setSearch} />}
              end={<label className="list-toolbar-switch"><Switch aria-label="Mostrar encerradas" size="small" checked={showClosed} onChange={setShowClosed} /><span>Mostrar encerradas</span></label>}
            />
            <InvestmentPositionsPeriod data={data} search={search} showClosed={showClosed}
              hrefOf={() => null} positionMenu={() => []} groupMenu={() => []}
              empty={<EmptyState title="Nenhuma posição neste período" hint="Não há posição com valor ou movimento no período." />}
            />
          </Section>
        </ReportValuation>
      </>}
    </main>
  );
}
