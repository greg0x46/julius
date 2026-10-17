import type { NetWorthBreakdown, NetWorthSnapshot } from "../../api/contracts";
import { SummaryStrip } from "../layout";
import { Money } from "../shared/Money";

// A debt payable and a card's current bill-cycle total are summed as
// distinct liabilities, never deduplicated — see internal/networth's
// Breakdown doc comment for why.
//
// Receivables (money owed to the user) have no field here at all: they're a
// future possibility, not a realized asset — see internal/networth's
// Breakdown doc comment. The formula hint says so out loud.
//
// The page title already names the figure, so the hero is labelled "Hoje"
// (the latest snapshot). The two assets read as plain balances (red only if
// one is negative); the two liabilities arrive as magnitudes and are shown
// with the "-" they subtract with, in ink.
//
// Tone rule for liabilities (here and wherever else a liability is listed):
// a liability is neutral, never red. Red is reserved for "this is wrong
// now" — a negative balance, a negative result, an overdue payable. Owing a
// card bill or a loan is the normal state of a liability; its "-" already
// says it subtracts. (Contas e cartões follows the same rule for card debt.)
function Liability({ value }: { value: string }) {
  const zero = /^-?0*(\.0*)?$/.test(value);
  return zero ? (
    <Money value={value} tone="balance" />
  ) : (
    <Money value={value} tone="flow" direction="outflow" />
  );
}

function valuationNote(snapshot: NetWorthSnapshot): string {
  const rates = Object.keys(snapshot.valuation?.rates ?? {});
  const parts = [snapshot.is_backfilled ? "Estimativa reconstruída a partir das transações; não inclui investimentos históricos." : ""];
  if (snapshot.valuation_status === "legacy") {
    parts.push("Registro anterior à avaliação de outras moedas: cobre apenas contas e investimentos em reais.");
  } else if (rates.length > 0) {
    parts.push(`Saldos em ${rates.join(", ")} convertidos para reais com o câmbio de ${new Date(snapshot.valuation!.valued_at).toLocaleDateString("pt-BR", { timeZone: "UTC" })}.`);
  }
  if (snapshot.valuation_status === "stale") parts.push("Há cotações desatualizadas nesta avaliação.");
  return parts.filter(Boolean).join(" ");
}

export function NetWorthBreakdownCard({ snapshot }: { snapshot: NetWorthSnapshot & NetWorthBreakdown }) {
  const extra = valuationNote(snapshot);
  return (
    <SummaryStrip
      label="Hoje"
      value={<Money value={snapshot.net_worth} tone="balance" size="hero" />}
      note={`Caixa + Investimentos − Cartão de crédito − Dívidas. Valores a receber não entram.${extra ? ` ${extra}` : ""}`}
      items={[
        { label: "Caixa", value: <Money value={snapshot.cash_balance} tone="balance" /> },
        { label: "Investimentos", value: <Money value={snapshot.investment_balance} tone="balance" /> },
        { label: "Cartão de crédito", value: <Liability value={snapshot.credit_card_balance} /> },
        { label: "Dívidas", value: <Liability value={snapshot.payables_debt} /> },
      ]}
    />
  );
}
