import { SearchOutlined } from "../ui/icons";
import { Input } from "../ui";

interface SearchFieldProps {
  id: string;
  /** Read by screen readers; visually the placeholder does the labelling. */
  label: string;
  placeholder?: string;
  value: string;
  onChange: (value: string) => void;
}

/** The text search of a ListToolbar: one input, full width on a phone. */
export function SearchField({ id, label, placeholder, value, onChange }: SearchFieldProps) {
  return (
    <div className="list-toolbar-search">
      <label htmlFor={id} className="visually-hidden">
        {label}
      </label>
      <Input
        id={id}
        aria-label={label}
        allowClear
        prefix={<SearchOutlined aria-hidden="true" />}
        placeholder={placeholder ?? label}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
    </div>
  );
}
