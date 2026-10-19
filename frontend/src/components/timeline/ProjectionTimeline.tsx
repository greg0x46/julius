import { useState } from "react";
import { EvolutionControls, type EvolutionScale } from "../evolution/EvolutionControls";
import { changePercent } from "../../presentation/decimal";
import { Alert } from "antd";
import {
  CartesianGrid,
  Line,
  LineChart,
  ReferenceDot,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";

import type { TimelineEntry, TimelineSeries } from "../../api/contracts";
import { useReportCurrency, useReportFormat } from "../../hooks/useReportCurrency";
import { formatAxisMoney } from "../../presentation/chartAxis";
import { yAxisWidth } from "../../presentation/chartAxisWidth";
import { balanceChartColor } from "../../presentation/chartColors";
import { colors } from "../../theme/tokens";
import { useCompactScreen } from "../shared/useCompactScreen";

function formatDate(value: string): string {
  const [year, month, day] = value.split("-");
  return `${day}/${month}/${year}`;
}

// Axis ticks only need to place a day in the month: the year is in the period
// control and in the tooltip.
function formatShortDate(value: string): string {
  const [, month, day] = value.split("-");
  return `${day}/${month}`;
}

const pastLineName = "Saldo realizado";
const futureLineName = "Saldo projetado";
const simulationLineName = "Saldo (simulação)";

// The past and future halves of the base line share the reference day, so
// without this the tooltip would list the same balance twice on that one day.
function tooltipRows(payload: { value: number; name: string }[]): { value: number; name: string }[] {
  let baseShown = false;
  return payload.filter((entry) => {
    if (entry.value === undefined || entry.value === null) return false;
    if (entry.name !== pastLineName && entry.name !== futureLineName) return true;
    if (baseShown) return false;
    baseShown = true;
    return true;
  });
}

// A balance alone doesn't explain itself: the day it drops, what the reader
// wants is which entries moved it. Only a handful fit before the tooltip
// becomes a wall, so the largest ones win and the rest are counted.
const maxTooltipEntries = 4;

function DayEntries({ entries }: { entries: TimelineEntry[] }) {
  const format = useReportFormat();
  if (entries.length === 0) return null;
  const ranked = [...entries].sort((a, b) => Math.abs(Number(b.amount)) - Math.abs(Number(a.amount)));
  const shown = ranked.slice(0, maxTooltipEntries);
  const hidden = ranked.length - shown.length;
  return (
    <ul className="timeline-chart-tooltip-entries">
      {shown.map((entry) => (
        <li key={`${entry.source}-${entry.source_ref_id}-${entry.description}-${entry.amount}`}>
          <span>{entry.description}</span>
          <span
            style={{
              color: Number(entry.amount) < 0 ? colors.error : undefined,
            }}
          >
            {format(entry.amount)}
          </span>
        </li>
      ))}
      {hidden > 0 && <li className="timeline-chart-tooltip-more">e mais {hidden}</li>}
    </ul>
  );
}

export function TimelineTooltip({
  active,
  payload,
  label,
  entriesByDate,
  scale = "absolute",
  originalValues,
  referenceDate,
}: {
  active?: boolean;
  payload?: { value: number; name: string }[];
  label?: string;
  entriesByDate?: Map<string, TimelineEntry[]>;
  scale?: EvolutionScale;
  originalValues?: Map<string, { balance: string; simulation?: string }>;
  referenceDate?: string;
}) {
  const format = useReportFormat();
  if (!active || !payload || payload.length === 0 || !label) return null;
  const rows = tooltipRows(payload);
  if (rows.length === 0) return null;
  return (
    <div className="timeline-chart-tooltip">
      <strong>{formatDate(label)}</strong>
      {rows.map((entry) => (
        <div key={entry.name}>
          {label === referenceDate && (entry.name === pastLineName || entry.name === futureLineName) ? "Saldo no fim do dia (inclui previsões)" : entry.name}: {scale === "percent" ? `${entry.value.toLocaleString("pt-BR", { maximumFractionDigits: 2 })}% · ` : ""}{format(originalValues?.get(label)?.[entry.name === simulationLineName ? "simulation" : "balance"] ?? String(entry.value))}
        </div>
      ))}
      <DayEntries entries={entriesByDate?.get(label) ?? []} />
    </div>
  );
}

// A window can span a full year of daily points, where "dd/mm" ticks collide;
// past roughly four months only the month (and its year) is worth reading.
function tickFormatterFor(pointCount: number): (value: string) => string {
  if (pointCount <= 120) return formatShortDate;
  return (value: string) => {
    const [year, month] = value.split("-");
    return `${month}/${year.slice(2)}`;
  };
}

type LegendItem = {
  key: string;
  label: string;
  color: string;
  kind: "solid" | "dashed" | "dot";
};

/**
 * The legend is HTML rather than recharts' own: its swatches have to show the
 * one difference that matters — a solid line is what happened, a dashed one is
 * a forecast — and the lowest-balance marker is named here instead of on the
 * plot, where a label would collide with the line or clip at the chart edge.
 */
function ChartLegend({ items }: { items: LegendItem[] }) {
  return (
    <ul className="timeline-chart-legend" aria-label="Legenda do gráfico">
      {items.map((item) => (
        <li key={item.key}>
          <span className={`timeline-chart-swatch is-${item.kind}`} style={{ color: item.color }} aria-hidden="true" />
          {item.label}
        </li>
      ))}
    </ul>
  );
}

type PointRow = {
  date: string;
  balance: number;
  pastBalance?: number;
  futureBalance?: number;
  simulationBalance?: number;
};

function mergePoints(base: TimelineSeries, simulation: TimelineSeries | null, referenceDate: string): PointRow[] {
  const rows = base.points.map((p) => {
    // Points include planned movements due today; starting_balance is cash now.
    const balance = Number(p.balance);
    // The reference day belongs to both halves so the solid past line and the
    // dashed future line meet instead of leaving a gap.
    return {
      date: p.date,
      balance,
      pastBalance: p.date <= referenceDate ? balance : undefined,
      futureBalance: p.date >= referenceDate ? balance : undefined,
    } as PointRow;
  });
  if (!simulation) return rows;
  const simulationByDate = new Map(simulation.points.map((p) => [p.date, Number(p.balance)]));
  return rows.map((row) => ({
    ...row,
    simulationBalance: simulationByDate.get(row.date),
  }));
}

// ProjectionTimeline draws Series.Points as one continuous line —
// Realizado→Confirmado→Projetado→Hipotético — never a separate chart per
// tier, since the balance itself is one continuous number regardless of
// how certain each contributing entry is. The only split is at the
// reference date, where the same line turns dashed (and from ink to brand
// green): everything before it already happened, everything after it is a
// forecast. When a simulation is active, a second line joins it — always Base
// vs. Simulação as a whole, never one line per scenario even with several
// active (seção 20).
export function ProjectionTimeline({
  series,
  simulation,
  referenceDate,
  height = 280,
  onSelectDay,
}: {
  series: TimelineSeries;
  simulation?: TimelineSeries | null;
  referenceDate: string;
  height?: number;
  /**
   * Called with the clicked day. Only days up to the reference date are
   * reported: later ones are projections with no transaction behind them yet.
   */
  onSelectDay?: (date: string) => void;
}) {
  const compact = useCompactScreen();
  const currency = useReportCurrency();
  const format = useReportFormat();
  const [selected, setSelected] = useState<string[]>(["cash"]);
  const [scale, setScale] = useState<EvolutionScale>("absolute");
  const baseline = series.points[0]?.balance;
  const simulationBaseline = simulation?.points[0]?.balance;
  const invalidBaseline = scale === "percent" && (baseline == null || changePercent(baseline, baseline) === null ||
    (simulation && (simulationBaseline == null || simulation.points[0]?.date !== series.points[0]?.date || changePercent(simulationBaseline, simulationBaseline) === null)));
  const simulationValues = new Map(simulation?.points.map((point) => [point.date, point.balance]));
  const originalValues = new Map(series.points.map((point) => [point.date, { balance: point.balance,
    simulation: simulationValues.get(point.date) }]));
  const chartValue = (value: string, base = baseline) => scale === "absolute" ? Number(value) : Number(base == null ? null : changePercent(value, base));
  const data = mergePoints(series, simulation ?? null, referenceDate).map((point) => scale === "absolute" ? point : {
    ...point,
    balance: chartValue(originalValues.get(point.date)!.balance),
    pastBalance: point.pastBalance === undefined ? undefined : chartValue(originalValues.get(point.date)!.balance),
    futureBalance: point.futureBalance === undefined ? undefined : chartValue(originalValues.get(point.date)!.balance),
    simulationBalance: originalValues.get(point.date)?.simulation === undefined ? undefined : chartValue(originalValues.get(point.date)!.simulation!, simulationBaseline),
  });
  const axisWidth = scale === "percent" ? 72 : yAxisWidth(
    data.flatMap((row) => [row.balance, row.simulationBalance].filter((v): v is number => v !== undefined)),
  );
  const todayPoint = series.points.find((p) => p.date === referenceDate);
  const activeSeries = simulation ?? series;
  const hasPast = data.some((row) => row.date < referenceDate);
  // A window with no future day at all still gets the forward line, so a
  // one-point series is never drawn as nothing.
  const hasFuture = data.some((row) => row.date > referenceDate) || !hasPast;
  const entriesByDate = new Map<string, TimelineEntry[]>();
  for (const entry of activeSeries.entries) {
    const day = entriesByDate.get(entry.date);
    if (day) day.push(entry);
    else entriesByDate.set(entry.date, [entry]);
  }
  const handleClick = (state: { activeLabel?: string | number }) => {
    const date = state?.activeLabel;
    if (onSelectDay && typeof date === "string" && date <= referenceDate) onSelectDay(date);
  };

  const legend: LegendItem[] = [];
  if (hasPast) legend.push({ key: "past", label: pastLineName, color: balanceChartColor.realized, kind: "solid" });
  if (hasFuture) legend.push({ key: "future", label: futureLineName, color: balanceChartColor.projected, kind: "dashed" });
  if (simulation) legend.push({ key: "simulation", label: simulationLineName, color: balanceChartColor.simulation, kind: "dashed" });
  legend.push({ key: "lowest", label: "Menor saldo", color: balanceChartColor.lowest, kind: "dot" });

  return (
    <div className="timeline-chart" role="group" aria-label="Saldo ao longo do tempo">
      <EvolutionControls options={[{ key: "cash", label: "Saldo", color: balanceChartColor.realized }]}
        selected={selected} onSelected={setSelected} scale={scale} onScale={setScale} />
      <p className="net-worth-footnote">{scale === "absolute" ? `Valores em ${currency}.` : `Variação desde ${series.points[0]?.date ? formatDate(series.points[0].date) : "o início do período"}.`} Linha contínua: realizado; tracejada: previsão. Valores convertidos à cotação de referência atual, sem representar câmbio histórico.</p>
      {selected.length === 0 ? <p role="status">Selecione ao menos uma métrica para ver a evolução.</p> : invalidBaseline ?
        <p role="status">A variação % exige um valor inicial positivo para todas as métricas selecionadas no início do período.</p> : <>
      {activeSeries.first_negative && (
        <Alert
          type="warning"
          showIcon
          message={`Se nada mudar, o saldo pode ficar negativo em ${formatDate(activeSeries.first_negative)}.`}
          style={{ marginBottom: 12 }}
        />
      )}
      <ResponsiveContainer width="100%" height={height}>
        <LineChart
          data={data}
          // Room above the plot for the "Hoje" label and on the right so the
          // last tick is never clipped.
          margin={{ top: 20, right: 16, left: 0, bottom: 4 }}
          onClick={onSelectDay ? handleClick : undefined}
          style={onSelectDay ? { cursor: "pointer" } : undefined}
        >
          <CartesianGrid stroke={balanceChartColor.grid} vertical={false} />
          <XAxis
            dataKey="date"
            tickFormatter={tickFormatterFor(data.length)}
            interval="preserveStartEnd"
            minTickGap={compact ? 28 : 40}
            tickMargin={8}
            tickLine={false}
            axisLine={{ stroke: balanceChartColor.grid }}
            tick={{ fontSize: 12, fill: balanceChartColor.axis }}
          />
          <YAxis
            padding={{ top: 8, bottom: 8 }}
            tickFormatter={scale === "percent" ? (value: number) => `${value.toLocaleString("pt-BR", { maximumFractionDigits: 2 })}%` : formatAxisMoney}
            tickLine={false}
            axisLine={false}
            width={axisWidth}
            tick={{ fontSize: 12, fill: balanceChartColor.axis }}
          />
          <Tooltip content={<TimelineTooltip entriesByDate={entriesByDate} scale={scale} originalValues={originalValues} referenceDate={referenceDate} />} />
          {/* Zero is the line that matters on this chart — where the balance
              crosses it is the whole point of looking. */}
          <ReferenceLine y={0} stroke={balanceChartColor.lowest} strokeOpacity={0.5} />
          {todayPoint && (
            <ReferenceLine
              x={todayPoint.date}
              stroke={balanceChartColor.axis}
              strokeDasharray="2 3"
              label={{ value: "Hoje", position: "top", fontSize: 12, fill: balanceChartColor.axis }}
            />
          )}
          <ReferenceDot
            x={activeSeries.lowest_balance.date}
            y={chartValue(activeSeries.lowest_balance.balance, simulation ? simulationBaseline : baseline)}
            r={5}
            fill={balanceChartColor.lowest}
            stroke="#fff"
            strokeWidth={2}
          />
          {hasPast && (
            <Line
              type="stepAfter"
              dataKey="pastBalance"
              name={pastLineName}
              stroke={balanceChartColor.realized}
              strokeWidth={2}
              dot={data.length === 1 ? { r: 3 } : false}
              connectNulls={false}
            />
          )}
          {hasFuture && (
            <Line
              type="stepAfter"
              dataKey="futureBalance"
              name={futureLineName}
              stroke={balanceChartColor.projected}
              strokeWidth={2}
              strokeDasharray="5 4"
              dot={data.length === 1 ? { r: 3 } : false}
              connectNulls={false}
            />
          )}
          {simulation && (
            <Line
              type="stepAfter"
              dataKey="simulationBalance"
              name={simulationLineName}
              stroke={balanceChartColor.simulation}
              strokeWidth={2}
              strokeDasharray="2 3"
              dot={data.length === 1 ? { r: 3 } : false}
            />
          )}
        </LineChart>
      </ResponsiveContainer>
      <ChartLegend items={legend} />
      <details className="evolution-values"><summary>Ver valores · {scale === "absolute" ? currency : "% e valores originais"}</summary>
        <table><caption>Saldo realizado até hoje; previsão a partir de hoje. Hoje inclui compromissos ainda previstos.</caption><thead><tr><th>Data</th><th>Saldo</th>{simulation && <th>Simulação</th>}</tr></thead>
          <tbody>{series.points.map((point) => <tr key={point.date}>
            <th scope="row">{onSelectDay && point.date <= referenceDate ? <button type="button" onClick={() => onSelectDay(point.date)}>{formatDate(point.date)}</button> : formatDate(point.date)}{point.date >= referenceDate && " · previsão"}</th>
            <td>{scale === "percent" && `${Number(changePercent(point.balance, baseline!)!).toLocaleString("pt-BR", { maximumFractionDigits: 2 })}% · `}{format(point.balance)}</td>
            {simulation && <td>{originalValues.get(point.date)?.simulation == null ? "—" : <>{scale === "percent" && `${Number(changePercent(originalValues.get(point.date)!.simulation!, simulationBaseline!)!).toLocaleString("pt-BR", { maximumFractionDigits: 2 })}% · `}{format(originalValues.get(point.date)!.simulation!)}</>}</td>}
          </tr>)}</tbody></table>
      </details>
      </>}
    </div>
  );
}
