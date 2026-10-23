import type { ReactNode } from "react";
import { LinePlot } from "../ui/LinePlot";

import { hasNetWorthValue, type NetWorthPoint } from "../../api/contracts";
import { balanceChartColor } from "../../presentation/chartColors";
import { useReportFormat } from "../../hooks/useReportCurrency";
import { Section } from "../layout";
import { useCompactScreen } from "../shared/useCompactScreen";

type ChartPoint = { date: string; netWorth: string | null };

// date is a calendar day at midnight UTC; local time would slide it back a day.
function dateLabel(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleDateString("pt-BR", {
    day: "2-digit",
    month: "2-digit",
    timeZone: "UTC",
  });
}

function toChartData(points: NetWorthPoint[]): ChartPoint[] {
  // A day that cannot be valued is a gap in the line, never a zero.
  return points.map((s) => ({
    date: s.date.slice(0, 10),
    netWorth: hasNetWorthValue(s) ? s.total : null,
  }));
}

function TooltipContent({
  active,
  payload,
  label,
}: {
  active?: boolean;
  payload?: { value: string }[];
  label?: string;
}) {
  const format = useReportFormat();
  if (!active || !payload || payload.length === 0) return null;
  return (
    <div className="timeline-chart-tooltip">
      <strong>{label}</strong>
      <div>Patrimônio: {format(payload[0].value)}</div>
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
  points,
  footnote,
}: {
  points: NetWorthPoint[];
  footnote?: ReactNode;
}) {
  const compact = useCompactScreen();
  const data = toChartData(points);
  const first = data[0] ? dateLabel(data[0].date) : "";
  const last = data.length ? dateLabel(data[data.length - 1].date) : "";
  return (
    <Section title="Evolução do patrimônio">
      <div
        className="timeline-chart"
        role="group"
        aria-label={`Evolução do patrimônio, de ${first} a ${last}, com lacunas nos dias sem avaliação disponível`}
      >
        <LinePlot
          dates={data.map((row) => row.date)}
          height={compact ? 240 : 320}
          label={`Evolução do patrimônio, de ${first} a ${last}, com lacunas nos dias sem avaliação disponível`}
          formatDate={dateLabel}
          series={[
            {
              key: "netWorth",
              label: "Patrimônio",
              color: balanceChartColor.realized,
              values: data.map((row) => row.netWorth),
            },
          ]}
          tooltip={(index) => (
            <TooltipContent
              active
              label={dateLabel(data[index].date)}
              payload={
                data[index].netWorth !== null
                  ? [{ value: data[index].netWorth! }]
                  : []
              }
            />
          )}
        />
      </div>
      {footnote !== undefined && (
        <p className="net-worth-footnote">{footnote}</p>
      )}
    </Section>
  );
}
