import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import * as transfers from "../../api/transfers";
import * as transactions from "../../api/transactions";
import type { TransactionItem } from "../../api/contracts";
import { QueryTestProvider } from "../../test/QueryTestProvider";
import { transactionResult } from "../../test/transactionFixtures";
import { TransactionOverviewScreen } from "./TransactionOverviewScreen";

vi.mock("../../api/transfers");
vi.mock("../../api/transactions");

const corrected: TransactionItem = {
  ...transactionResult.items[0],
  movement_type: null,
  classification: "unclassified",
  totals_eligibility: { included: false, reason: "unclassified" },
};

function show() {
  const navigate = vi.fn();
  render(
    <QueryTestProvider>
      <TransactionOverviewScreen
        item={corrected}
        categories={[]}
        categoryPending={false}
        inclusionPending={false}
        deleteError={null}
        navigate={navigate}
      />
    </QueryTestProvider>,
  );
  return navigate;
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(transfers.getTransactionTransfer).mockResolvedValue(null);
  vi.mocked(transactions.getTransactionReconciliation).mockResolvedValue({ current: null, options: [] });
});

describe("TransactionOverviewScreen transfer access", () => {
  it("opens retained transfer details after a provider correction makes the movement unclassified", async () => {
    const user = userEvent.setup();
    vi.mocked(transfers.getTransactionTransfer).mockResolvedValue({
      id: "transfer-id",
      source: { transaction_id: corrected.id, account_id: corrected.account.id, account_name: "Conta BRL", currency_code: "BRL", amount: "-540" },
      destination: { transaction_id: "destination-id", account_id: "destination-account", account_name: "Conta USD", currency_code: "USD", amount: "100" },
      effective_rate: "5.4",
      fee: null,
    });
    const navigate = show();
    const action = screen.getByRole("button", { name: /Transferência entre contas/ });
    await waitFor(() => expect(action).toBeEnabled());
    await user.click(action);
    expect(navigate).toHaveBeenCalledWith({ kind: "transfer" });
  });

  it("keeps new pairing disabled for an unclassified movement", async () => {
    show();
    await screen.findByText("Nenhum vínculo");
    expect(screen.getByRole("button", { name: /Transferência entre contas/ })).toBeDisabled();
  });
});
