import {
  createContext,
  useContext,
  useId,
  type ChangeEvent,
  type InputHTMLAttributes,
  type ReactNode,
} from "react";
const Context = createContext<{
  name?: string;
  value?: string;
  disabled?: boolean;
  change?: (event: ChangeEvent<HTMLInputElement>) => void;
}>({});
export function RadioControl({
  children,
  value,
  ...props
}: InputHTMLAttributes<HTMLInputElement>) {
  const context = useContext(Context);
  return (
    <label className="ui-radio-wrapper">
      <input
        {...props}
        type="radio"
        value={value}
        name={context.name ?? props.name}
        checked={context.value === String(value)}
        disabled={props.disabled || context.disabled}
        onChange={context.change ?? props.onChange}
      />
      {children}
    </label>
  );
}
export function RadioGroup({
  value,
  onChange,
  children,
  disabled,
  ...props
}: {
  value?: string | null;
  optionType?: string;
  onChange: (event: ChangeEvent<HTMLInputElement>) => void;
  children: ReactNode;
  disabled?: boolean;
  "aria-label"?: string;
  className?: string;
}) {
  const name = useId();
  return (
    <Context.Provider
      value={{ name, value: value ?? undefined, disabled, change: onChange }}
    >
      <div
        {...props}
        role="radiogroup"
        className={`ui-radio-group ${props.className ?? ""}`}
      >
        {children}
      </div>
    </Context.Provider>
  );
}
