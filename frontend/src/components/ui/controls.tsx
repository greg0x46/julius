import {
  useId,
  useRef,
  useState,
  type ButtonHTMLAttributes,
  type CSSProperties,
  type InputHTMLAttributes,
  type HTMLAttributes,
  type ReactNode,
  type Ref,
  type TextareaHTMLAttributes,
} from "react";
import { calendar, type CalendarDate } from "../../presentation/calendar";

export interface ButtonProps extends Omit<
  ButtonHTMLAttributes<HTMLButtonElement>,
  "type"
> {
  ref?: Ref<HTMLButtonElement>;
  type?: "primary" | "default" | "text" | "link" | "dashed";
  htmlType?: "button" | "submit" | "reset";
  size?: "small" | "middle" | "large";
  danger?: boolean;
  loading?: boolean;
  icon?: ReactNode;
  block?: boolean;
}
export function Button({
  type = "default",
  htmlType = "button",
  size = "middle",
  danger,
  loading,
  icon,
  block,
  className = "",
  children,
  disabled,
  ...rest
}: ButtonProps) {
  return (
    <button
      {...rest}
      type={htmlType}
      className={`ui-btn ui-btn-${type} ui-btn-${size} ${danger ? "ui-btn-dangerous" : ""} ${block ? "ui-btn-block" : ""} ${icon && children == null ? "ui-btn-icon-only" : ""} ${className}`}
      disabled={disabled || loading}
      aria-busy={loading || undefined}
    >
      {loading ? <span className="ui-spinner" aria-hidden="true" /> : icon}
      {children}
    </button>
  );
}
export type InputRef = HTMLInputElement;
type InputProps = Omit<
  InputHTMLAttributes<HTMLInputElement>,
  "size" | "prefix"
> & {
  ref?: Ref<HTMLInputElement>;
  size?: string;
  status?: string;
  prefix?: ReactNode;
  suffix?: ReactNode;
  allowClear?: boolean;
  onPressEnter?: () => void;
};
export function TextInput({
  size,
  status,
  prefix,
  suffix,
  allowClear,
  onPressEnter,
  className = "",
  ref,
  ...props
}: InputProps) {
  const input = useRef<HTMLInputElement>(null);
  const field = (
    <input
      {...props}
      ref={(node) => {
        input.current = node;
        if (typeof ref === "function") ref(node);
        else if (ref) ref.current = node;
      }}
      className={`ui-input ${status ? `ui-input-${status}` : ""} ${className}`}
      data-size={size}
      onKeyDown={(event) => {
        props.onKeyDown?.(event);
        if (event.key === "Enter" && !event.nativeEvent.isComposing)
          onPressEnter?.();
      }}
    />
  );
  const clear =
    allowClear && props.value && !props.disabled ? (
      <button
        type="button"
        aria-label="Limpar campo"
        onClick={() => {
          const element = input.current;
          if (!element) return;
          Object.getOwnPropertyDescriptor(
            HTMLInputElement.prototype,
            "value",
          )?.set?.call(element, "");
          element.dispatchEvent(new Event("input", { bubbles: true }));
          element.focus();
        }}
      >
        ×
      </button>
    ) : null;
  return prefix || suffix || allowClear ? (
    <span className="ui-input-affix-wrapper">
      {prefix && <span className="ui-input-prefix">{prefix}</span>}
      {field}
      {suffix}
      {clear}
    </span>
  ) : (
    field
  );
}
export function Password(props: InputProps) {
  const [visible, setVisible] = useState(false);
  return (
    <span className="ui-input-affix-wrapper">
      <TextInput {...props} type={visible ? "text" : "password"} />
      <button
        type="button"
        aria-label={visible ? "Ocultar senha" : "Mostrar senha"}
        onClick={() => setVisible(!visible)}
      >
        {visible ? "Ocultar" : "Mostrar"}
      </button>
    </span>
  );
}
export function TextArea({
  autoSize,
  ...props
}: TextareaHTMLAttributes<HTMLTextAreaElement> & {
  autoSize?: boolean | { minRows?: number; maxRows?: number };
}) {
  return (
    <textarea
      {...props}
      rows={props.rows ?? (typeof autoSize === "object" ? autoSize.minRows : 3)}
      className={`ui-input ${props.className ?? ""}`}
    />
  );
}
export interface InputNumberProps<
  T extends string | number = number,
> extends Omit<InputProps, "value" | "defaultValue" | "onChange" | "prefix"> {
  value?: T | null;
  defaultValue?: T;
  onChange?: (value: T | null) => void;
  prefix?: ReactNode;
  precision?: number;
  controls?: boolean;
  stringMode?: boolean;
  decimalSeparator?: string;
  formatter?: (
    value: T | undefined,
    info: { userTyping: boolean; input: string },
  ) => string;
  parser?: (text: string | undefined) => T;
}
export function InputNumber<T extends string | number = number>({
  value,
  defaultValue,
  onChange,
  prefix,
  precision,
  controls,
  stringMode,
  decimalSeparator,
  formatter,
  parser,
  ...props
}: InputNumberProps<T>) {
  const [draft, setDraft] = useState<string | null>(null);
  const current = value ?? defaultValue;
  const text =
    draft ??
    (formatter
      ? formatter(current ?? undefined, { userTyping: false, input: "" })
      : String(current ?? "").replace(".", decimalSeparator ?? "."));
  void controls;
  return (
    <TextInput
      {...props}
      type="text"
      role={stringMode ? "textbox" : "spinbutton"}
      inputMode={props.inputMode ?? "decimal"}
      prefix={prefix}
      value={text}
      onChange={(event) => {
        const raw = event.target.value;
        setDraft(raw);
        const parsed = parser ? parser(raw) : raw.replace(",", ".");
        if (String(parsed).trim() === "") {
          onChange?.(null);
          return;
        }
        if (!/^-?(?:\d+(?:\.\d*)?|\.\d+)$/.test(String(parsed))) return;
        if (precision === 0 && !Number.isInteger(Number(parsed))) return;
        onChange?.((stringMode ? parsed : Number(parsed)) as T);
      }}
      onBlur={(event) => {
        setDraft(null);
        props.onBlur?.(event);
      }}
    />
  );
}
export function DatePicker({
  value,
  onChange,
  allowClear = true,
  format,
  ...props
}: Omit<InputProps, "value" | "onChange"> & {
  value?: CalendarDate | null;
  onChange?: (value: CalendarDate | null) => void;
  allowClear?: boolean;
  format?: string;
}) {
  void format;
  return (
    <TextInput
      {...props}
      type="date"
      min="0001-01-01"
      max="9999-12-31"
      required={!allowClear}
      value={value?.format("YYYY-MM-DD") ?? ""}
      onChange={(event) => {
        if (!event.target.value) {
          if (allowClear) onChange?.(null);
          return;
        }
        const date = calendar(event.target.value);
        if (date.isValid()) onChange?.(date);
      }}
    />
  );
}
export function CheckboxControl(
  props: InputHTMLAttributes<HTMLInputElement> & {
    children?: ReactNode;
    indeterminate?: boolean;
    ref?: Ref<HTMLInputElement>;
  },
) {
  const { children, indeterminate, ref, ...rest } = props;
  return (
    <label className="ui-checkbox-wrapper">
      <input
        {...rest}
        type="checkbox"
        ref={(node) => {
          if (node) node.indeterminate = !!indeterminate;
          if (typeof ref === "function") ref(node);
          else if (ref) ref.current = node;
        }}
      />
      <span>{children}</span>
    </label>
  );
}
export function CheckboxGroup({
  options,
  value,
  onChange,
  disabled,
  ...aria
}: {
  disabled?: boolean;
  "aria-labelledby"?: string;
  options: { value: string; label: ReactNode; disabled?: boolean }[];
  value: string[];
  onChange: (values: string[]) => void;
}) {
  return (
    <div {...aria} className="ui-checkbox-group">
      {options.map((option) => (
        <CheckboxControl
          key={option.value}
          disabled={disabled || option.disabled}
          checked={value.includes(option.value)}
          onChange={(event) =>
            onChange(
              event.target.checked
                ? [...value, option.value]
                : value.filter((item) => item !== option.value),
            )
          }
        >
          {option.label}
        </CheckboxControl>
      ))}
    </div>
  );
}
export function Switch({
  checked,
  onChange,
  loading,
  disabled,
  size,
  checkedChildren,
  unCheckedChildren,
  ...props
}: Omit<ButtonHTMLAttributes<HTMLButtonElement>, "onChange"> & {
  checked?: boolean;
  onChange?: (checked: boolean) => void;
  loading?: boolean;
  size?: string;
  checkedChildren?: ReactNode;
  unCheckedChildren?: ReactNode;
}) {
  void size;
  return (
    <button
      {...props}
      type="button"
      className={`ui-switch ${props.className ?? ""}`}
      role="switch"
      aria-checked={!!checked}
      disabled={disabled || loading}
      aria-busy={loading}
      onClick={(event) => {
        props.onClick?.(event);
        onChange?.(!checked);
      }}
    >
      <span />
      {checked ? checkedChildren : unCheckedChildren}
    </button>
  );
}
export function Segmented<T extends string>({
  options,
  value,
  onChange,
  block,
  size,
  disabled,
  ...props
}: {
  options: { value: T; label: ReactNode; disabled?: boolean }[];
  value: T;
  onChange: (value: T) => void;
  block?: boolean;
  size?: string;
  disabled?: boolean;
} & HTMLAttributes<HTMLDivElement>) {
  const id = useId();
  void block;
  void size;
  return (
    <div
      {...props}
      className={`ui-segmented ${props.className ?? ""}`}
      role="radiogroup"
    >
      {options.map((option) => (
        <label key={option.value} data-active={value === option.value}>
          <input
            type="radio"
            name={id}
            checked={value === option.value}
            disabled={disabled || option.disabled}
            onChange={() => onChange(option.value)}
          />
          {option.label}
        </label>
      ))}
    </div>
  );
}
export type ControlStyle = CSSProperties;
