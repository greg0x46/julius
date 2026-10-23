import type { NetWorthBreakdown, NetWorthPoint } from "../../api/contracts";
import { SummaryStrip } from "../layout";
import { Money } from "../shared/Money";

// Wealth is what has already been realized: cash plus investments. A card, a
// debt or a bill to come is not wealth and has no line here (see
// internal/networth). The page title already names the figure, so the hero is
// labelled "Hoje"; the two parts read as plain balances (red only if negative).
function valuationNote(point: NetWorthPoint): string {
  const rates = Object.keys(point.valuation?.rates ?? {});
  const parts: string[] = [];
  if (rates.length > 0) {
    parts.push(`Saldos em ${rates.join(", ")} convertidos para reais com o câmbio de ${new Date(point.valuation!.valued_at).toLocaleDateString("pt-BR", { timeZone: "UTC" })}.`);
  }
  if (point.valuation_status === "stale") parts.push("Há cotações desatualizadas nesta avaliação.");
  return parts.join(" ");
}

export function NetWorthBreakdownCard({ point }: { point: NetWorthPoint & NetWorthBreakdown }) {
  const extra = valuationNote(point);
  return (
    <SummaryStrip
      label="Hoje"
      value={<Money value={point.total} tone="balance" size="hero" />}
      note={`Caixa + Investimentos. Cartão, dívidas e contas futuras não entram: só o que já foi realizado.${extra ? ` ${extra}` : ""}`}
      items={[
        { label: "Caixa", value: <Money value={point.cash_balance} tone="balance" /> },
        { label: "Investimentos", value: <Money value={point.investment_balance} tone="balance" /> },
      ]}
    />
  );
}
