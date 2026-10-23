import {
  useLayoutEffect,
  useRef,
  useState,
  type InputHTMLAttributes,
} from "react";
import { formatMoney, parseMoney } from "./moneyText";

interface MoneyInputProps extends Omit<
  InputHTMLAttributes<HTMLInputElement>,
  "value" | "defaultValue" | "onChange" | "size"
> {
  value?: string | null;
  defaultValue?: string;
  onChange?: (value: string | null) => void;
  currencyCode?: string;
  status?: "error";
}
/** Exact decimal editing, including pasted decimal points and fractional cents. */
export function MoneyInput({
  value,
  defaultValue,
  onChange,
  currencyCode = "BRL",
  status,
  style,
  placeholder = "0,00",
  ...props
}: MoneyInputProps) {
  const [internal, setInternal] = useState(defaultValue ?? "");
  const [editing, setEditing] = useState<string | null>(null);
  const current = value === undefined ? internal : (value ?? "");
  const shown = editing ?? formatMoney(current, { userTyping: false });
  const input = useRef<HTMLInputElement>(null);
  const caret = useRef<number | null>(null);
  useLayoutEffect(() => {
    if (caret.current !== null && input.current === document.activeElement)
      input.current?.setSelectionRange(caret.current, caret.current);
    caret.current = null;
  });
  return (
    <span
      className={`ui-input-affix-wrapper ui-money-input ${status ? "ui-input-error" : ""}`}
      style={{ width: "100%", ...style }}
    >
      <span>{currencyCode === "BRL" ? "R$" : currencyCode}</span>
      <input
        {...props}
        ref={input}
        type="text"
        className="ui-input"
        inputMode="decimal"
        placeholder={placeholder}
        value={shown}
        onChange={(event) => {
          const raw = event.target.value;
          const parsed = parseMoney(raw, shown);
          if (parsed !== "" && !/^-?(?:\d+(?:\.\d*)?|\.\d*)$/.test(parsed))
            return;
          const formatted = formatMoney(parsed, { userTyping: true });
          const position = event.target.selectionStart ?? raw.length;
          // Retain the logical cursor position when grouping inserts/removes dots.
          const tailLength = raw.slice(position).replace(/\./g, "").length;
          let next = formatted.length;
          let remaining = tailLength;
          while (next > 0 && remaining > 0) {
            next--;
            if (formatted[next] !== ".") remaining--;
          }
          caret.current = next;
          setEditing(formatted);
          setInternal(parsed);
          onChange?.(parsed === "" ? null : parsed);
        }}
        onBlur={(event) => {
          setEditing(null);
          props.onBlur?.(event);
        }}
      />
    </span>
  );
}
