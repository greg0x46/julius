import { createContext, useContext } from "react";
import { formatReportMoney } from "../presentation/money";

export const ReportCurrencyContext = createContext("BRL");
export function useReportCurrency() {
  return useContext(ReportCurrencyContext);
}
export function useReportFormat() {
  const code = useReportCurrency();
  return (value: string) => formatReportMoney(value, code);
}
