import { Skeleton } from "antd";

import type { Investment } from "../../api/contracts";
import { investmentTypeLabel, investmentYield, yieldUnavailable } from "../../presentation/investmentLabels";
import { EmptyState, ListRow, Section } from "../layout";
import { Money } from "../shared/Money";

/**
 * The yield under the saldo: a signed result when the backend states one, or
 * the short reason it does not. The longer explanation is on the investment's
 * own page, where it reads as text instead of a hover-only tooltip.
 */
function YieldFigure({ investment }: { investment: Investment }) {
  const estimate = investmentYield(investment);
  if (estimate === null) return <span className="investment-row-yield">{yieldUnavailable(investment).label}</span>;
  return (
    <span className="investment-row-yield">
      Rendimento <Money value={estimate.value} tone="result" />
    </span>
  );
}

/**
 * Investimentos sincronizados as flat rows: name 500 with institution and
 * type as the one meta line, saldo atual 600 on the right and the yield
 * under it. Each row opens the investment.
 */
export function InvestmentList({
  investments,
  isLoading,
  onOpen,
}: {
  investments: Investment[];
  isLoading: boolean;
  onOpen: (investment: Investment) => void;
}) {
  return (
    <Section title="Investimentos sincronizados">
      {isLoading ? (
        <div role="status" aria-label="Carregando investimentos">
          <Skeleton active paragraph={{ rows: 2 }} title={false} />
        </div>
      ) : investments.length === 0 ? (
        <EmptyState
          title="Nenhum investimento sincronizado ainda"
          hint="Quando uma instituição conectada enviar investimentos, eles aparecem aqui."
        />
      ) : (
        <ul className="list-rows" aria-label="Investimentos sincronizados">
          {investments.map((investment) => (
            <ListRow
              key={investment.id}
              title={investment.name ?? "Sem nome"}
              meta={[investment.source_display_name, investmentTypeLabel(investment.investment_type)]
                .filter(Boolean)
                .join(" · ")}
              trailing={investment.balance !== null ? <Money value={investment.balance} tone="balance" /> : "—"}
              status={<YieldFigure investment={investment} />}
              onClick={() => onOpen(investment)}
              ariaLabel={`Abrir ${investment.name ?? "investimento sem nome"}`}
            />
          ))}
        </ul>
      )}
    </Section>
  );
}
