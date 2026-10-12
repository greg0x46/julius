import type {
  InvestmentAccountKind,
  InvestmentOperationKind,
  InvestmentPositionSource,
  InvestmentValuationBasis,
} from "../api/contracts";

export const investmentAccountKindLabel: Record<InvestmentAccountKind, string> = {
  manual: "Manual",
  integrated: "Integrada",
  // Kept for data written by the first preview of the investment workspace.
  synced: "Integrada",
};

export const investmentOperationKindLabel: Record<InvestmentOperationKind, string> = {
  initial_balance: "Saldo inicial",
  deposit: "Aporte em caixa",
  withdrawal: "Resgate da caixa",
  buy: "Compra",
  sell: "Venda",
  income: "Rendimento",
  fee: "Taxa",
  tax: "Imposto",
  valuation: "Cotação manual",
  transfer_out: "Transferência enviada",
  transfer_in: "Transferência recebida",
};

export const investmentPositionSourceLabel: Record<InvestmentPositionSource, string> = {
  manual: "Manual",
  synced: "Integrada",
};

export const investmentValuationBasisLabel: Record<InvestmentValuationBasis, string> = {
  manual_valuation: "Cotação manual",
  cost_basis: "Custo médio",
  provider_balance: "Saldo informado pela instituição",
  market_quote: "Cotação de mercado",
};
