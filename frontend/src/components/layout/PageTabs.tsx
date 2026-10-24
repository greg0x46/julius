import { Segmented } from "../ui";

interface PageTabsProps<Value extends string> {
  label: string;
  value: Value;
  options: { value: Value; label: string }[];
  onChange: (value: Value) => void;
}

/**
 * The page's top-level views ("Todas / Dívidas / A receber"). A view changes
 * what the whole content area shows, which is why it sits between the header
 * and the collection controls rather than among the filters.
 */
export function PageTabs<Value extends string>({ label, value, options, onChange }: PageTabsProps<Value>) {
  return (
    <Segmented
      aria-label={label}
      className="page-tabs-control"
      value={value}
      options={options}
      onChange={(next) => onChange(next as Value)}
    />
  );
}
