import { Skeleton } from "antd";
import { useParams } from "react-router-dom";

import { isUuid } from "../api/contracts";
import { DetailPage, InvalidDetailPage, Page } from "../components/layout";
import { InvestmentHeaderCard } from "../components/investments/InvestmentHeaderCard";
import { InvestmentPositionOperations } from "../components/investments/InvestmentPositionOperations";
import { InvestmentTimeline } from "../components/investments/InvestmentTimeline";
import { useInvestmentDetail } from "../hooks/useInvestmentDetail";

const pageTitle = "Investimento";
const backTo = "/investimentos";
const backLabel = "Voltar para investimentos";

/** The loaded page's blocks — summary strip, then a list — so nothing jumps when the record arrives. */
function InvestmentDetailSkeleton() {
  return (
    <div role="status" aria-label="Carregando investimento">
      <Skeleton active title={{ width: "35%" }} paragraph={{ rows: 2 }} />
      <Skeleton active title={{ width: "25%" }} paragraph={{ rows: 4 }} />
    </div>
  );
}

function ValidInvestmentDetail({ id }: { id: string }) {
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
      recordTitle={(snapshot) => snapshot.name ?? "Sem nome"}
      state={investment.state}
      retry={investment.retry}
      notFoundMessage="Investimento não encontrado"
      notFoundDescription="Não existe um investimento com este identificador."
      unavailableMessage="Não foi possível consultar este investimento agora."
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
  const { id = "" } = useParams();
  return isUuid(id) ? (
    <ValidInvestmentDetail id={id} />
  ) : (
    <InvalidDetailPage
      title={pageTitle}
      backTo={backTo}
      backLabel={backLabel}
      invalidTitle="Endereço de investimento inválido"
    />
  );
}
