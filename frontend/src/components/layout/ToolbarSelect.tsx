import { Select } from "antd";

interface ToolbarSelectProps<Value extends string> {
  id: string;
  label: string;
  value: Value;
  options: { value: Value; label: string }[];
  onChange: (value: Value) => void;
}

/**
 * A labelled select for the shaping side of a ListToolbar. The label is
 * spoken and shown as a prefix on desktop ("Agrupar: Semana"); on a phone
 * only the chosen value fits, so the label goes visually hidden.
 */
export function ToolbarSelect<Value extends string>({ id, label, value, options, onChange }: ToolbarSelectProps<Value>) {
  return (
    <div className="list-toolbar-select">
      <label htmlFor={id}>{label}</label>
      <Select
        id={id}
        aria-label={label}
        value={value}
        options={options}
        popupMatchSelectWidth={false}
        onChange={onChange}
      />
    </div>
  );
}

export function GroupBySelect<Value extends string>(props: Omit<ToolbarSelectProps<Value>, "label">) {
  return <ToolbarSelect label="Agrupar por" {...props} />;
}

export function SortSelect<Value extends string>(props: Omit<ToolbarSelectProps<Value>, "label">) {
  return <ToolbarSelect label="Ordenar por" {...props} />;
}
