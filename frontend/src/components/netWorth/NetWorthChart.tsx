import type { ReactNode } from "react";
import { CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";

import { hasNetWorthValue, type NetWorthSnapshot } from "../../api/contracts";
import { formatAxisMoney } from "../../presentation/chartAxis";
import { yAxisWidth } from "../../presentation/chartAxisWidth";
import { balanceChartColor } from "../../presentation/chartColors";
import { useReportFormat } from "../../hooks/useReportCurrency";
import { Section } from "../layout";
import { useCompactScreen } from "../shared/useCompactScreen";

type ChartPoint = { date: string; netWorth: number | null };

// captured_at is a calendar day at midnight UTC; local time would slide it back a day.
function dateLabel(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleDateString("pt-BR", { day: "2-digit", month: "2-digit", timeZone: "UTC" });
}

function toChartData(snapshots: NetWorthSnapshot[]): ChartPoint[] {
  // A day that cannot be valued is a gap in the line, never a zero.
  return snapshots.map((s) => ({ date: dateLabel(s.captured_at), netWorth: hasNetWorthValue(s) ? Number(s.net_worth) : null }));
}

function TooltipContent({
  active,
  payload,
  label,
}: {
  active?: boolean;
  payload?: { value: number }[];
  label?: string;
}) {
  const format = useReportFormat();
  if (!active || !payload || payload.length === 0) return null;
  return (
    <div className="timeline-chart-tooltip">
      <strong>{label}</strong>
      <div>Patrimônio líquido: {format(String(payload[0].value))}</div>
    </div>
  );
}

// A single series never needs a legend (see the dataviz skill) — the section
// title names what the line is. It is drawn in ink, like the "realized" line
// of the balance chart on Início: a net worth that was captured is a fact,
// not a forecast. The Y axis is there because a bare line with no scale says
// "it went up" but never by how much; its ticks are rounded like Início's.
// `footnote` is the one place for what the series does not cover.
export function NetWorthChart({
  snapshots,
  footnote,
}: {
  snapshots: NetWorthSnapshot[];
  footnote?: ReactNode;
}) {
  const compact = useCompactScreen();
  const data = toChartData(snapshots);
  const axisWidth = yAxisWidth(data.flatMap((point) => (point.netWorth === null ? [] : [point.netWorth])));
  const first = data[0]?.date ?? "";
  const last = data[data.length - 1]?.date ?? "";
  return (
    <Section title="Evolução do patrimônio líquido">
      <div className="timeline-chart" role="img" aria-label={`Evolução do patrimônio líquido, de ${first} a ${last}, com lacunas nos dias sem avaliação disponível`}>
        <ResponsiveContainer width="100%" height={compact ? 240 : 320}>
          <LineChart data={data} margin={{ top: 12, right: 16, left: 0, bottom: 4 }}>
            <CartesianGrid stroke={balanceChartColor.grid} vertical={false} />
            <XAxis
              dataKey="date"
              interval="preserveStartEnd"
              minTickGap={compact ? 28 : 40}
              tickMargin={8}
              tickLine={false}
              axisLine={{ stroke: balanceChartColor.grid }}
              tick={{ fontSize: 12, fill: balanceChartColor.axis }}
            />
            <YAxis
              tickFormatter={formatAxisMoney}
              tickLine={false}
              axisLine={false}
              width={axisWidth}
              tick={{ fontSize: 12, fill: balanceChartColor.axis }}
            />
            <Tooltip content={<TooltipContent />} />
            <Line
              connectNulls={false}
              type="monotone"
              dataKey="netWorth"
              name="Patrimônio líquido"
              stroke={balanceChartColor.realized}
              strokeWidth={2}
              dot={false}
              activeDot={{ r: 4 }}
            />
          </LineChart>
        </ResponsiveContainer>
      </div>
      {footnote !== undefined && <p className="net-worth-footnote">{footnote}</p>}
    </Section>
  );
}
