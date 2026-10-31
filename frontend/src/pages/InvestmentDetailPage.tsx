import { useTranslation } from "../i18n/context";
import { Skeleton } from "../components/ui";
import { useParams } from "react-router-dom";

import { isUuid } from "../api/contracts";
import { DetailPage, InvalidDetailPage, Page } from "../components/layout";
import { InvestmentHeaderCard } from "../components/investments/InvestmentHeaderCard";
import { InvestmentPositionOperations } from "../components/investments/InvestmentPositionOperations";
import { InvestmentTimeline } from "../components/investments/InvestmentTimeline";
import { useInvestmentDetail } from "../hooks/useInvestmentDetail";

const backTo = "/investimentos";

/** The loaded page's blocks — summary strip, then a list — so nothing jumps when the record arrives. */
function InvestmentDetailSkeleton() {
  const { t } = useTranslation();
  return (
    <div role="status" aria-label={t("loadingInvestment")}>
      <Skeleton active title={{ width: "35%" }} paragraph={{ rows: 2 }} />
      <Skeleton active title={{ width: "25%" }} paragraph={{ rows: 4 }} />
    </div>
  );
}

function ValidInvestmentDetail({ id }: { id: string }) {
  const { t } = useTranslation();
  const pageTitle = t("investmentTitle");
  const backLabel = t("backToInvestments");
  const investment = useInvestmentDetail(id);

  if (investment.state.freshness === "loading") {
    return (
      <Page title={pageTitle} backTo={backTo} backLabel={backLabel} compactMobileHeader>
        <InvestmentDetailSkeleton />
      </Page>
    );
  }

  return (
    <DetailPage
      title={pageTitle}
      backTo={backTo}
      backLabel={backLabel}
      recordTitle={(snapshot) => snapshot.name ?? t("unnamed")}
      state={investment.state}
      retry={investment.retry}
      notFoundMessage={t("investmentNotFound")}
      notFoundDescription={t("investmentNotFoundDescription")}
      unavailableMessage={t("investmentUnavailable")}
    >
      {(snapshot) => (
        <>
          <InvestmentHeaderCard investment={snapshot} transactions={investment.transactions} />

          <InvestmentTimeline
            transactions={investment.transactions}
            isLoading={investment.transactionsLoading}
            error={investment.transactionsError}
            onRetry={investment.retryTransactions}
          />

          <InvestmentPositionOperations investmentId={id} />
        </>
      )}
    </DetailPage>
  );
}

export function InvestmentDetailPage() {
  const { t } = useTranslation();
  const { id = "" } = useParams();
  return isUuid(id) ? (
    <ValidInvestmentDetail id={id} />
  ) : (
    <InvalidDetailPage
      title={t("investmentTitle")}
      backTo={backTo}
      backLabel={t("backToInvestments")}
      invalidTitle={t("invalidInvestmentAddress")}
    />
  );
}
