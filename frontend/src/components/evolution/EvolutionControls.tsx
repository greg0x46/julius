import { useId } from "react";
import { Checkbox, Radio } from "antd";

export type EvolutionScale = "absolute" | "percent";
export type MetricOption = { key: string; label: string; color: string; unavailable?: string };

export function EvolutionControls({ options, selected, onSelected, scale, onScale }: {
  options: MetricOption[];
  selected: string[];
  onSelected: (selected: string[]) => void;
  scale: EvolutionScale;
  onScale: (scale: EvolutionScale) => void;
}) {
  const id = useId();
  return <div className="evolution-controls">
    <div role="group" aria-label="Métricas" className="evolution-metrics">
      {options.map((option) => <span key={option.key}>
        <Checkbox checked={selected.includes(option.key)} disabled={!!option.unavailable} aria-describedby={option.unavailable ? `${id}-${option.key}` : undefined}
          onChange={(event) => onSelected(event.target.checked ? [...selected, option.key] : selected.filter((key) => key !== option.key))}>
          <span className="timeline-chart-swatch" style={{ color: option.color }} aria-hidden="true" /> {option.label}
        </Checkbox>
        {option.unavailable && <small id={`${id}-${option.key}`}> — {option.unavailable}</small>}
      </span>)}
    </div>
    <Radio.Group size="small" aria-label="Escala" value={scale} onChange={(event) => onScale(event.target.value)}>
      <Radio value="absolute">Valor absoluto</Radio>
      <Radio value="percent">Variação %</Radio>
    </Radio.Group>
  </div>;
}
