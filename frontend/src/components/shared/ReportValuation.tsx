import { useTranslation } from "../../i18n/context";
import { Alert } from "../ui";
import { type ReactNode } from "react";

import type { ReportValuation as Valuation } from "../../api/contracts";
import { ReportCurrencyContext } from "../../hooks/useReportCurrency";

export function ReportValuation({ valuation, children }: { valuation: Valuation; children: ReactNode }) {
  const { t } = useTranslation();
  return (
    <ReportCurrencyContext.Provider value={valuation.currency_code ?? "BRL"}>
      {valuation.fx?.stale && (
        <Alert
          type="warning"
          showIcon
          message={t("outdatedRate")}
          description={t("outdatedRateDetails").replace("{currency}", valuation.currency_code ?? "BRL").replace("{date}", valuation.fx.effective_on.slice(0, 10)).replace("{source}", valuation.fx.source)}
        />
      )}
      {children}
    </ReportCurrencyContext.Provider>
  );
}
