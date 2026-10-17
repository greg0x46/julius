import { Alert } from "antd";
import { type ReactNode } from "react";

import type { ReportValuation as Valuation } from "../../api/contracts";
import { ReportCurrencyContext } from "../../hooks/useReportCurrency";

export function ReportValuation({ valuation, children }: { valuation: Valuation; children: ReactNode }) {
  return (
    <ReportCurrencyContext.Provider value={valuation.currency_code ?? "BRL"}>
      {valuation.fx?.stale && (
        <Alert
          type="warning"
          showIcon
          message="Cotação desatualizada"
          description={`Valores em ${valuation.currency_code} usam a última cotação disponível (${valuation.fx.effective_on.slice(0, 10)}, ${valuation.fx.source}).`}
        />
      )}
      {children}
    </ReportCurrencyContext.Provider>
  );
}
