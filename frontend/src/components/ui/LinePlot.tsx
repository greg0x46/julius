import { formatDateOnly } from "../../presentation/dates";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { formatAxisMoney } from "../../presentation/chartAxis";
import { yAxisWidth } from "../../presentation/chartAxisWidth";
import { balanceChartColor } from "../../presentation/chartColors";

export interface PlotSeries {
  key: string;
  label: string;
  color: string;
  dashed?: string;
  step?: boolean;
  values: (string | null | undefined)[];
}
interface LinePlotProps {
  dates: string[];
  series: PlotSeries[];
  height: number;
  formatDate: (date: string) => string;
  tooltip: (index: number) => ReactNode;
  onSelect?: (index: number) => void;
  referenceDate?: string;
  lowest?: { date: string; value: string };
  label: string;
}
export function LinePlot({
  dates,
  series,
  height,
  formatDate,
  tooltip,
  onSelect,
  referenceDate,
  lowest,
  label,
}: LinePlotProps) {
  const [selectedDate, setSelectedDate] = useState<string | null>(null);
  const selectedIndex = selectedDate == null ? -1 : dates.indexOf(selectedDate);
  const selected = selectedIndex >= 0 ? selectedIndex : null;
  const container = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(800);
  useEffect(() => {
    const element = container.current;
    if (!element) return;
    const update = () => {
      const measured = element.getBoundingClientRect().width;
      if (measured > 0) setWidth(Math.max(280, measured));
    };
    update();
    const observer = new ResizeObserver(update);
    observer.observe(element);
    return () => observer.disconnect();
  }, []);
  const values = series
    .flatMap((line) => line.values.filter((value) => value != null).map(Number))
    .filter(Number.isFinite);
  const min = Math.min(0, ...values);
  const max = Math.max(0, ...values);
  const span = max - min || 1;
  const left = yAxisWidth(values);
  const right = 20;
  const top = 28;
  const bottom = height - 32;
  const x = (index: number) =>
    dates.length <= 1
      ? (left + width - right) / 2
      : left + (index / (dates.length - 1)) * (width - left - right);
  const y = (value: number) => bottom - ((value - min) / span) * (bottom - top);
  const pathFor = (line: PlotSeries) => {
    let connected = false;
    return line.values
      .map((value, index) => {
        if (value == null || !Number.isFinite(Number(value))) {
          connected = false;
          return "";
        }
        const command = connected
          ? line.step
            ? `H${x(index)}V${y(Number(value))}`
            : `L${x(index)},${y(Number(value))}`
          : `M${x(index)},${y(Number(value))}`;
        connected = true;
        return command;
      })
      .join(" ");
  };
  const tickEvery = Math.max(
    1,
    Math.ceil(dates.length / Math.max(2, Math.floor(width / 120))),
  );
  const refIndex = referenceDate ? dates.indexOf(referenceDate) : -1;
  const lowIndex = lowest ? dates.indexOf(lowest.date) : -1;
  return (
    <div ref={container} className="ui-line-plot">
      <svg
        role="img"
        aria-label={label}
        viewBox={`0 0 ${width} ${height}`}
        width="100%"
        height={height}
        onPointerLeave={() => setSelectedDate(null)}
      >
        {Array.from({ length: 5 }, (_, index) => {
          const value = min + (span * index) / 4;
          return (
            <g key={index}>
              <line
                x1={left}
                x2={width - right}
                y1={y(value)}
                y2={y(value)}
                stroke={balanceChartColor.grid}
              />
              <text
                x={left - 8}
                y={y(value)}
                textAnchor="end"
                dominantBaseline="middle"
                fontSize={12}
                fill={balanceChartColor.axis}
              >
                {formatAxisMoney(value)}
              </text>
            </g>
          );
        })}
        <line
          x1={left}
          x2={width - right}
          y1={y(0)}
          y2={y(0)}
          stroke={balanceChartColor.lowest}
          opacity={0.5}
        />
        {dates.map((date, index) =>
          index % tickEvery === 0 || index === dates.length - 1 ? (
            <text
              key={date}
              x={x(index)}
              y={height - 8}
              textAnchor="middle"
              fontSize={12}
              fill={balanceChartColor.axis}
            >
              {formatDate(date)}
            </text>
          ) : null,
        )}
        {refIndex >= 0 && (
          <g>
            <line
              x1={x(refIndex)}
              x2={x(refIndex)}
              y1={top}
              y2={bottom}
              stroke={balanceChartColor.axis}
              strokeDasharray="2 3"
            />
            <text x={x(refIndex)} y={16} fontSize={12} textAnchor="middle">
              Hoje
            </text>
          </g>
        )}
        {series.map((line) => (
          <g key={line.key}>
            <path
              data-series={line.key}
              d={pathFor(line)}
              fill="none"
              stroke={line.color}
              strokeWidth={2}
              strokeDasharray={line.dashed}
            />
            {line.values.map((value, index) =>
              value != null &&
              (dates.length === 1 ||
                (line.values[index - 1] == null &&
                  line.values[index + 1] == null)) ? (
                <circle
                  key={index}
                  cx={x(index)}
                  cy={y(Number(value))}
                  r={3}
                  fill={line.color}
                />
              ) : null,
            )}
          </g>
        ))}
        {lowest && lowIndex >= 0 && (
          <circle
            cx={x(lowIndex)}
            cy={y(Number(lowest.value))}
            r={5}
            fill={balanceChartColor.lowest}
            stroke="#fff"
            strokeWidth={2}
          />
        )}
        {selected != null && (
          <line
            x1={x(selected)}
            x2={x(selected)}
            y1={top}
            y2={bottom}
            stroke={balanceChartColor.axis}
            opacity={0.5}
          />
        )}
        {dates.map((date, index) => (
          <rect
            key={date}
            x={Math.max(
              left,
              x(index) -
                (width - left - right) / Math.max(dates.length - 1, 1) / 2,
            )}
            y={top}
            width={(width - left - right) / Math.max(dates.length - 1, 1)}
            height={bottom - top}
            fill="transparent"
            onPointerEnter={() => setSelectedDate(date)}
            onClick={() => {
              setSelectedDate(date);
              onSelect?.(index);
            }}
          />
        ))}
      </svg>
      <div className="ui-chart-controls">
        <label>
          Consultar data{" "}
          <select
            aria-label="Consultar data do gráfico"
            value={selected ?? ""}
            onChange={(event) =>
              setSelectedDate(
                event.target.value === ""
                  ? null
                  : (dates[Number(event.target.value)] ?? null),
              )
            }
          >
            <option value="">Selecione</option>
            {dates.map((date, index) => (
              <option key={date} value={index}>
                {formatDateOnly(date)}
              </option>
            ))}
          </select>
        </label>
        {onSelect && selected != null && (
          <button type="button" onClick={() => onSelect(selected)}>
            Ver transações do dia
          </button>
        )}
      </div>
      {selected != null && (
        <div className="ui-chart-tooltip" role="status">
          {tooltip(selected)}
        </div>
      )}
    </div>
  );
}
