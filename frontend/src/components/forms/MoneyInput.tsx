import { InputNumber, type InputNumberProps } from "antd";
import { useRef } from "react";

import { formatMoney, parseMoney } from "./moneyText";

type MoneyInputProps = Omit<
  InputNumberProps<number>,
  "prefix" | "decimalSeparator" | "precision" | "inputMode" | "controls" | "stringMode" | "formatter" | "parser"
>;

/**
 * The one money field: an InputNumber with the "R$" prefix, pt-BR grouping
 * and decimal comma ("12.850,00"), two decimals and the numeric keypad on
 * phones (`inputMode="decimal"`; rc-input-number forwards unknown props to
 * its `<input>`). Steppers are off — nobody steps a price — which also frees
 * the width for the figure. The value stays a plain number.
 *
 * Reading the text is `parseMoney`: a comma is the decimal mark, but a lone
 * "." before one or two final digits is read as one too, so a pasted
 * "1234.56" is 1234.56 and not 123456 (see moneyText.ts for the full rule).
 */
export function MoneyInput({ style, placeholder = "0,00", currencyCode = "BRL", ...rest }: MoneyInputProps & { currencyCode?: string }) {
  // What the field showed last: parseMoney needs it to tell a dot the field
  // itself put there (backspacing "12.850") from one the user typed or pasted.
  const shown = useRef("");
  return (
    <InputNumber<number>
      {...rest}
      prefix={currencyCode === "BRL" ? "R$" : currencyCode}
      precision={2}
      // rc-input-number feeds the parser's string straight into its decimal
      // type (it must stay a string so "12." survives while typing); antd's
      // typings only know the numeric result.
      formatter={(value, info) => {
        shown.current = formatMoney(value, info);
        return shown.current;
      }}
      parser={((text: string | undefined) => parseMoney(text, shown.current)) as unknown as (text: string | undefined) => number}
      inputMode="decimal"
      controls={false}
      placeholder={placeholder}
      style={{ width: "100%", ...style }}
    />
  );
}
