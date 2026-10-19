import { useState, type ReactNode } from "react";
import { Bar, BarChart, CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";

import type { NetWorthSnapshot } from "../../api/contracts";
import { useReportCurrency, useReportFormat } from "../../hooks/useReportCurrency";
import { formatAxisMoney } from "../../presentation/chartAxis";
import { yAxisWidth } from "../../presentation/chartAxisWidth";
import { balanceChartColor } from "../../presentation/chartColors";
import { changePercent, multiplyDecimals } from "../../presentation/decimal";
import { EvolutionControls, type EvolutionScale, type MetricOption } from "../evolution/EvolutionControls";
import { portfolioStale, reliablePortfolio, scalePoints, wealthPoints, type EvolutionPoint, type InvestmentHistory } from "../evolution/evolution";
import { Section } from "../layout";
import { useCompactScreen } from "../shared/useCompactScreen";

const stockOptions: MetricOption[] = [
  { key: "netWorth", label: "Patrimônio líquido", color: balanceChartColor.realized },
  { key: "cash", label: "Saldo observado", color: balanceChartColor.simulation },
  { key: "investments", label: "Investimentos", color: "#7b4aa1" },
];
const dateLabel = (date: string) => date.split("-").reverse().join("/");
const percentLabel = (value: number | string) => `${Number(value).toLocaleString("pt-BR", { maximumFractionDigits: 2 })}%`;

export function WealthTooltip({ active, label, points, selected, scale }: {
  active?: boolean; label?: string | number; points: EvolutionPoint[]; selected: string[]; scale: EvolutionScale;
}) {
  const format = useReportFormat();
  const point = points.find((candidate) => candidate.date === label);
  if (!active || !point) return null;
  return <div className="timeline-chart-tooltip">
    <strong>{dateLabel(point.date)}</strong>
    {stockOptions.filter((option) => selected.includes(option.key)).map((option) => {
      const value = point.values[option.key];
      const baseline = points[0]?.values[option.key];
      return <div key={option.key}>{option.label}: {value == null ? "Indisponível" : <>
        {scale === "percent" && baseline != null && `${percentLabel(changePercent(value, baseline)!)} · `}{format(value)}
      </>}</div>;
    })}
    {point.notes?.map((note) => <div key={note}>{note}</div>)}
  </div>;
}

export function NetWorthChart({ snapshots, footnote, from, to, history, historyState, onRetryHistory }: {
  snapshots: NetWorthSnapshot[]; footnote?: ReactNode; from?: string; to?: string;
  history?: InvestmentHistory; historyState?: "loading" | "error" | "range";
  onRetryHistory?: () => void;
}) {
  const compact = useCompactScreen();
  const currency = useReportCurrency();
  const format = useReportFormat();
  const [selected, setSelected] = useState<string[]>(["netWorth"]);
  const [scale, setScale] = useState<EvolutionScale>("absolute");
  const first = from ?? snapshots[0]?.captured_at.slice(0, 10) ?? "";
  const last = to ?? snapshots.at(-1)?.captured_at.slice(0, 10) ?? first;
  const points = first && last && first <= last ? wealthPoints(snapshots, first, last, history) : [];
  const stocks = stockOptions.filter((option) => selected.includes(option.key));
  const { data, invalid } = scalePoints(points, stocks.map((option) => option.key), scale);
  const axisWidth = scale === "percent" ? 72 : yAxisWidth(data.flatMap((point) => stocks.map((option) => point[option.key]).filter((value): value is number => typeof value === "number")));
  const hasValues = data.some((point) => stocks.some((option) => point[option.key] != null));
  const investmentUnavailable = historyState === "loading" ? "Carregando histórico" : historyState === "error" ? "Histórico indisponível" : historyState === "range" ? "Escolha até 120 meses, com início até hoje" :
    history && [history.baseline, ...history.periods].some(reliablePortfolio) ? undefined : "Sem histórico suficiente neste período";
  const options = [...stockOptions.map((option) => ({ ...option, unavailable: option.key === "investments" ? investmentUnavailable : undefined })),
    { key: "yield", label: "Rendimentos de investimentos", color: "#7b4aa1", unavailable: investmentUnavailable }];
  const flowData = history?.periods.map((period) => ({ date: period.to, period,
    yield: reliablePortfolio(period) ? Number(period.totals.yield) : null })) ?? [];

  return <Section title="Evolução do patrimônio líquido">
    <EvolutionControls options={options} selected={selected} onSelected={setSelected} scale={scale} onScale={setScale} />
    {historyState === "error" && <p role="status">Não foi possível carregar investimentos. <button type="button" onClick={onRetryHistory}>Tentar novamente</button></p>}
    <p className="net-worth-footnote" aria-live="polite">
      {scale === "percent" ? `Variação desde ${dateLabel(first)}. ` : `Valores em ${currency}. `}
      Dados históricos até {dateLabel(last)}, sem projeções. Lacunas indicam cobertura insuficiente.
    </p>
    {selected.length === 0 && <p role="status">Selecione ao menos uma métrica para ver a evolução.</p>}
    {stocks.length > 0 && invalid.length > 0 && <p role="status">A variação % exige um valor inicial positivo para todas as métricas selecionadas em {dateLabel(first)}. Sem base válida: {stockOptions.filter((option) => invalid.includes(option.key)).map((option) => option.label).join(", ")}.</p>}
    {stocks.length > 0 && hasValues && points.filter((point) => stocks.some((option) => point.values[option.key] != null)).length === 1 && <p role="status">Há apenas uma avaliação disponível neste período. Novos registros permitem acompanhar a evolução.</p>}
    {stocks.length > 0 && invalid.length === 0 && (!hasValues ? <p role="status">Não há avaliações disponíveis neste período.</p> : <>
      <div className="timeline-chart" role="img" aria-label={`${stocks.map((option) => option.label).join(", ")}, de ${dateLabel(first)} a ${dateLabel(last)}, ${scale === "absolute" ? currency : "variação percentual"}, com lacunas nos dias sem avaliação disponível`}>
        <ResponsiveContainer width="100%" height={compact ? 240 : 320}>
          <LineChart data={data} margin={{ top: 12, right: 16, left: 0, bottom: 4 }}>
            <CartesianGrid stroke={balanceChartColor.grid} vertical={false} />
            <XAxis dataKey="date" tickFormatter={(date: string) => dateLabel(date).slice(0, 5)} interval="preserveStartEnd" minTickGap={compact ? 28 : 40} tick={{ fontSize: 12 }} />
            <YAxis padding={{ top: 8, bottom: 8 }} width={axisWidth} tickFormatter={scale === "percent" ? percentLabel : formatAxisMoney} tick={{ fontSize: 12 }} />
            <Tooltip content={<WealthTooltip points={points} selected={selected} scale={scale} />} />
            {stocks.map((option) => <Line key={option.key} dataKey={option.key} name={option.label} type="linear" stroke={option.color} strokeWidth={2} dot={{ r: 2 }} activeDot={{ r: 4 }} connectNulls={false} />)}
          </LineChart>
        </ResponsiveContainer>
      </div>
      <details className="evolution-values"><summary>Ver valores · {scale === "absolute" ? currency : "% e valores originais"}</summary>
        <table><caption>Avaliações observadas; — indica lacuna</caption><thead><tr><th>Data</th>{stocks.map((option) => <th key={option.key}>{option.label}</th>)}</tr></thead>
          <tbody>{points.filter((point) => stocks.some((option) => point.values[option.key] != null) || point.notes?.length).map((point) => <tr key={point.date}>
            <th scope="row">{dateLabel(point.date)}{point.notes?.map((note) => <small key={note}> · {note}</small>)}</th>
            {stocks.map((option) => <td key={option.key}>{point.values[option.key] == null ? "—" : <>
              {scale === "percent" && `${percentLabel(changePercent(point.values[option.key]!, points[0].values[option.key]!)!)} · `}{format(point.values[option.key]!)}
            </>}</td>)}
          </tr>)}</tbody></table>
      </details>
    </>)}
    {selected.includes("yield") && history && <div role="group" aria-label={`Rendimentos por mês em ${currency}`}>
      <h3>Rendimentos por mês · {currency}</h3>
      <p className="net-worth-footnote">Fluxos por período, separados dos saldos. Rendimentos não incluem aportes e retiradas. A variação cambial dentro do período não está incluída. Meses nas extremidades podem ser parciais; histórico insuficiente aparece como lacuna.</p>
      <ResponsiveContainer width="100%" height={200}><BarChart data={flowData}>
        <CartesianGrid stroke={balanceChartColor.grid} vertical={false} />
        <XAxis dataKey="date" tickFormatter={(date: string) => dateLabel(date).slice(3)} minTickGap={28} tick={{ fontSize: 12 }} />
        <YAxis width={yAxisWidth(flowData.flatMap((point) => point.yield === null ? [] : [point.yield]))} tickFormatter={formatAxisMoney} tick={{ fontSize: 12 }} />
        <Tooltip content={({ active, label }) => {
          const period = history.periods.find((period) => period.to === label);
          return active && period ? <div className="timeline-chart-tooltip"><strong>{dateLabel(period.from!)} a {dateLabel(period.to)}</strong><div>Rendimento do período: {reliablePortfolio(period) ? format(period.totals.yield) : "Histórico insuficiente ou parcial"}</div>{portfolioStale(period) && <div>Câmbio desatualizado</div>}</div> : null;
        }} />
        <Bar dataKey="yield" name="Rendimento do período" fill="#7b4aa1" />
      </BarChart></ResponsiveContainer>
      <details className="evolution-values"><summary>Ver rendimentos e retorno Modified Dietz</summary>
        <table><thead><tr><th>Período</th><th>Rendimento ({currency})</th><th>Modified Dietz</th></tr></thead><tbody>{history.periods.map((period) => <tr key={period.to}>
          <th scope="row">{dateLabel(period.from!)} a {dateLabel(period.to)}{portfolioStale(period) && " · Câmbio desatualizado"}</th>
          <td>{reliablePortfolio(period) ? format(period.totals.yield) : "Histórico insuficiente ou parcial"}</td>
          <td>{reliablePortfolio(period) && period.totals.yield_rate !== null ? percentLabel(multiplyDecimals(period.totals.yield_rate, "100")) : "— (sem base válida)"}</td>
        </tr>)}</tbody></table>
      </details>
    </div>}
    <details className="evolution-values"><summary>Cobertura dos dados</summary>
      <p className="net-worth-footnote">Saldo e patrimônio usam registros capturados, com câmbio de cada dia. Dias reconstruídos não incluem investimentos e aparecem como lacunas; registros antigos sem cobertura cambial completa também são omitidos.</p>
      <p className="net-worth-footnote">Investimentos representam o valor da carteira, sem caixa para investir, pela última cotação disponível no dia; isso não implica uma nova cotação em cada ponto. Os pontos representam o início e o fim de cada mês ou período parcial. Períodos com posições sem histórico completo são omitidos, sem somar valores parciais.</p>
      {footnote !== undefined && <p className="net-worth-footnote">{footnote}</p>}
    </details>
  </Section>;
}
