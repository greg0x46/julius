import { Segmented } from "../ui";

export interface SegmentedOption<Value extends string> {
  value: Value;
  label: string;
  disabled?: boolean;
}

/** Mutually exclusive native radios styled as a segmented control. */
export function SegmentedControl<Value extends string>({
  id,
  label,
  value,
  options,
  onChange,
  disabled = false,
}: {
  id?: string;
  /** Accessible name of the group; the visible label is the caller's. */
  label: string;
  value: Value;
  options: SegmentedOption<Value>[];
  onChange: (value: Value) => void;
  disabled?: boolean;
}) {
  return (
    <Segmented
      id={id}
      block
      aria-label={label}
      className="segmented-control"
      value={value}
      options={options}
      disabled={disabled}
      onChange={(next) => onChange(next as Value)}
    />
  );
}
