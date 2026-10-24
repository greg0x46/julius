import { Input } from "../ui";
import { useId } from "react";

/** Digits typed by the user are cents: "1050" reads as R$ 10,50. */
function digitsToDecimal(digits: string): string | null {
  const trimmed = digits.replace(/^0+/, "");
  if (trimmed === "") return null;
  const padded = trimmed.padStart(3, "0");
  return `${padded.slice(0, -2)}.${padded.slice(-2)}`;
}

/** "10.5" | "10" | "10.50" → "1050"; the decimal strings filters carry. */
function decimalToDigits(value: string | null): string {
  if (value === null || value === "") return "";
  const [integer = "0", fraction = ""] = value.split(".");
  return `${integer}${fraction.padEnd(2, "0").slice(0, 2)}`.replace(/^0+(?=\d)/, "");
}

function formatDigits(digits: string): string {
  if (digits === "") return "";
  const padded = digits.padStart(3, "0");
  const whole = padded.slice(0, -2).replace(/\B(?=(\d{3})+(?!\d))/g, ".");
  return `R$\u00a0${whole},${padded.slice(-2)}`;
}

/**
 * One money field of the range: a cents mask, so every keystroke keeps the
 * value a well-formed amount and a phone shows the numeric keypad.
 */
function MoneyInput({
  id,
  label,
  value,
  onChange,
  describedBy,
  invalid,
}: {
  id: string;
  label: string;
  value: string | null;
  onChange: (value: string | null) => void;
  describedBy?: string;
  invalid?: boolean;
}) {
  return (
    <Input
      id={id}
      aria-label={label}
      aria-describedby={describedBy}
      aria-invalid={invalid || undefined}
      status={invalid ? "error" : undefined}
      inputMode="numeric"
      autoComplete="off"
      placeholder="R$ 0,00"
      allowClear
      value={formatDigits(decimalToDigits(value))}
      onChange={(event) => onChange(digitsToDecimal(event.target.value.replace(/\D/g, "")))}
    />
  );
}

/**
 * The "De … Até" pair of a value filter. Values in and out are the decimal
 * strings the API takes ("10.50" or null); formatting and the numeric
 * keypad are this component's business, whether the pair is consistent is
 * the caller's — it passes the message back as `error`.
 */
export function MoneyRangeFilter({
  id,
  min,
  max,
  onChange,
  error = null,
}: {
  id: string;
  min: string | null;
  max: string | null;
  onChange: (min: string | null, max: string | null) => void;
  error?: string | null;
}) {
  const errorId = useId();
  return (
    <div className="money-range">
      <div className="money-range-fields">
        <label className="money-range-field">
          <span className="money-range-label">De</span>
          <MoneyInput
            id={`${id}-min`}
            label="Valor mínimo"
            value={min}
            onChange={(next) => onChange(next, max)}
            describedBy={error ? errorId : undefined}
            invalid={error !== null}
          />
        </label>
        <label className="money-range-field">
          <span className="money-range-label">Até</span>
          <MoneyInput
            id={`${id}-max`}
            label="Valor máximo"
            value={max}
            onChange={(next) => onChange(min, next)}
            describedBy={error ? errorId : undefined}
            invalid={error !== null}
          />
        </label>
      </div>
      {error && (
        <p id={errorId} role="alert" className="money-range-error">
          {error}
        </p>
      )}
    </div>
  );
}
