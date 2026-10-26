import { Alert } from "../ui";
import { LinePlot } from "../ui/LinePlot";

import type { TimelineEntry, TimelineSeries } from "../../api/contracts";
import { useReportFormat } from "../../hooks/useReportCurrency";
import { balanceChartColor } from "../../presentation/chartColors";

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
function tooltipRows(
  payload: { value: string; name: string }[],
): { value: string; name: string }[] {
  let baseShown = false;
  return payload.filter((entry) => {
    if (entry.value === undefined || entry.value === null) return false;
    if (entry.name !== pastLineName && entry.name !== futureLineName)
      return true;
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
  const ranked = [...entries].sort(
    (a, b) => Math.abs(Number(b.amount)) - Math.abs(Number(a.amount)),
  );
  const shown = ranked.slice(0, maxTooltipEntries);
  const hidden = ranked.length - shown.length;
  return (
    <ul className="timeline-chart-tooltip-entries">
      {shown.map((entry) => (
        <li
          key={`${entry.source}-${entry.source_ref_id}-${entry.description}-${entry.amount}`}
        >
          <span>{entry.description}</span>
          <span>{format(entry.amount)}</span>
        </li>
      ))}
      {hidden > 0 && (
        <li className="timeline-chart-tooltip-more">e mais {hidden}</li>
      )}
    </ul>
  );
}

function TooltipContent({
  active,
  payload,
  label,
  entriesByDate,
}: {
  active?: boolean;
  payload?: { value: string; name: string }[];
  label?: string;
  entriesByDate?: Map<string, TimelineEntry[]>;
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
          {entry.name}: {format(entry.value)}
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

/** Solid, dashed and dot swatches distinguish realized balances, forecasts and the minimum. */
function ChartLegend({ items }: { items: LegendItem[] }) {
  return (
    <ul className="timeline-chart-legend" aria-label="Legenda do gráfico">
      {items.map((item) => (
        <li key={item.key}>
          <span
            className={`timeline-chart-swatch is-${item.kind}`}
            style={{ color: item.color }}
            aria-hidden="true"
          />
          {item.label}
        </li>
      ))}
    </ul>
  );
}

type PointRow = {
  date: string;
  balance: string;
  pastBalance?: string;
  futureBalance?: string;
  simulationBalance?: string;
};

function mergePoints(
  base: TimelineSeries,
  simulation: TimelineSeries | null,
  referenceDate: string,
): PointRow[] {
  const rows = base.points.map((p) => {
    // Points include planned movements due today; starting_balance is cash now.
    const balance = p.balance;
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
  const simulationByDate = new Map(
    simulation.points.map((p) => [p.date, p.balance]),
  );
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
  const data = mergePoints(series, simulation ?? null, referenceDate);
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
  const legend: LegendItem[] = [];
  if (hasPast)
    legend.push({
      key: "past",
      label: pastLineName,
      color: balanceChartColor.realized,
      kind: "solid",
    });
  if (hasFuture)
    legend.push({
      key: "future",
      label: futureLineName,
      color: balanceChartColor.projected,
      kind: "dashed",
    });
  if (simulation)
    legend.push({
      key: "simulation",
      label: simulationLineName,
      color: balanceChartColor.simulation,
      kind: "dashed",
    });
  legend.push({
    key: "lowest",
    label: "Menor saldo",
    color: balanceChartColor.lowest,
    kind: "dot",
  });

  return (
    <div
      className="timeline-chart"
      role="group"
      aria-label="Saldo ao longo do tempo"
    >
      {activeSeries.first_negative && (
        <Alert
          type="warning"
          showIcon
          message={`Se nada mudar, o saldo pode ficar negativo em ${formatDate(activeSeries.first_negative)}.`}
          style={{ marginBottom: 12 }}
        />
      )}
      <LinePlot
        dates={data.map((row) => row.date)}
        height={height}
        label="Saldo ao longo do tempo"
        formatDate={tickFormatterFor(data.length)}
        referenceDate={referenceDate}
        lowest={{
          date: activeSeries.lowest_balance.date,
          value: activeSeries.lowest_balance.balance,
        }}
        series={[
          ...(hasPast
            ? [
                {
                  key: "past",
                  label: pastLineName,
                  color: balanceChartColor.realized,
                  step: true,
                  values: data.map((row) => row.pastBalance),
                },
              ]
            : []),
          ...(hasFuture
            ? [
                {
                  key: "future",
                  label: futureLineName,
                  color: balanceChartColor.projected,
                  step: true,
                  dashed: "5 4",
                  values: data.map((row) => row.futureBalance),
                },
              ]
            : []),
          ...(simulation
            ? [
                {
                  key: "simulation",
                  label: simulationLineName,
                  color: balanceChartColor.simulation,
                  step: true,
                  dashed: "2 3",
                  values: data.map((row) => row.simulationBalance),
                },
              ]
            : []),
        ]}
        onSelect={
          onSelectDay
            ? (index) => {
                if (data[index].date <= referenceDate)
                  onSelectDay(data[index].date);
              }
            : undefined
        }
        tooltip={(index) => (
          <TooltipContent
            active
            label={data[index].date}
            entriesByDate={entriesByDate}
            payload={[
              ...(data[index].pastBalance !== undefined
                ? [{ name: pastLineName, value: data[index].pastBalance! }]
                : []),
              ...(data[index].futureBalance !== undefined
                ? [{ name: futureLineName, value: data[index].futureBalance! }]
                : []),
              ...(data[index].simulationBalance !== undefined
                ? [
                    {
                      name: simulationLineName,
                      value: data[index].simulationBalance!,
                    },
                  ]
                : []),
            ]}
          />
        )}
      />
      <ChartLegend items={legend} />
    </div>
  );
}
