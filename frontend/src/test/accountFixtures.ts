import type { Account, AccountBill, AccountCard } from "../api/contracts";

export const bankAccountId = "44444444-4444-4444-8444-444444444444";
export const creditAccountId = "55555555-5555-4555-8555-555555555555";

export const bankAccount: Account = {
  id: bankAccountId,
  origin: "synced",
  opening_balance: null,
  archived_at: null,
  has_movements: false,
  external_id: "acc-bank-1",
  source_display_name: "Banco Exemplo",
  institution: "Banco Exemplo",
  institution_name: "Banco Exemplo",
  name: "Conta Corrente",
  number: "12345-6",
  account_type: "BANK",
  account_subtype: "CHECKING_ACCOUNT",
  balance: "2500.00",
  credit_limit: null,
  available_credit_limit: null,
  credit_usage_ratio: null,
  currency_code: "BRL",
  balance_close_date: null,
  balance_due_date: null,
  closing_day: null,
  closing_day_source: null,
  provider_updated_at: "2026-08-01T12:00:00Z",
  updated_at: "2026-08-01T12:00:00Z",
};

export const creditAccount: Account = {
  id: creditAccountId,
  origin: "synced",
  opening_balance: null,
  archived_at: null,
  has_movements: false,
  external_id: "acc-credit-1",
  source_display_name: "Banco Exemplo",
  institution: "Banco Exemplo",
  institution_name: "Banco Exemplo",
  name: "Cartão Platinum",
  number: null,
  account_type: "CREDIT",
  account_subtype: "CREDIT_CARD",
  balance: "1234.56",
  credit_limit: "5000.00",
  available_credit_limit: "3765.44",
  credit_usage_ratio: "0.2469",
  currency_code: "BRL",
  balance_close_date: "2026-04-03T00:00:00Z",
  balance_due_date: "2026-04-10T00:00:00Z",
  closing_day: 3,
  closing_day_source: "informado",
  provider_updated_at: "2026-08-01T12:00:00Z",
  updated_at: "2026-08-01T12:00:00Z",
};

export const accountCard: AccountCard = {
  card_number: "1111",
  transaction_count: 12,
  last_transaction_at: "2026-04-01T12:00:00Z",
};

export const accountBill: AccountBill = {
  id: "66666666-6666-4666-8666-666666666666",
  external_id: "bill-1",
  due_date: "2026-04-10T00:00:00Z",
  closing_date: "2026-04-03T00:00:00Z",
  total_amount: "1234.56",
  currency_code: "BRL",
  minimum_payment_amount: "120.00",
};
