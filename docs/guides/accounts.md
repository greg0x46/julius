# Accounts, cards and transfers

[All documentation](../README.md) · [Transactions](transactions.md)

## Connect an institution

1. Configure your Pluggy credentials in Settings (`/configuracoes`).
2. Create an authorized bank connection in the Pluggy dashboard, then copy its
   Item ID into **Add connection** under Settings → Open Banking.
3. Sync the connection and review its run. Open Accounts and cards
   (`/contas-e-cartoes`) to inspect the imported accounts, balances and cards.

Balances on connected accounts come from the institution. Adding a manual
transaction there helps track a missing movement but does not change the
reported bank balance. Synchronization preserves your category, inclusion and
reconciliation decisions. Pause a connection to stop its automatic updates.
For file-based accounts, use [statement import](transactions.md#import-a-statement).

## Track an account manually

1. In Accounts and cards, choose **New account**. Enter a name, currency and
   opening balance, then save.
2. Add [manual transactions](transactions.md#record-a-manual-transaction) to
   record later movements in that account's currency.
3. Open the account to rename it or record a signed balance adjustment when
   you need to correct its balance without recording income or expense.

The balance is opening balance plus posted, undeleted movements and adjustments.
Ignoring or recategorizing a movement does not change that balance. Once any
movement or adjustment exists, the currency and opening balance are locked,
including after a movement is deleted. Archive an unused manual account to
prevent new entries; its balance and history remain. It can be reactivated.
Manual bank accounts do not create credit cards.

## Review a credit card

Open a card from Accounts and cards to inspect its transactions, bills,
closing day and due dates. Set or correct the closing day in its detail;
clearing a manual override restores the available inferred/provider value.

Eligible pending purchases already count as expenses and in the open bill;
posting them later does not create another expense. Manual card entries also
count in the calculated bill. Paying the bill is a separate bank/card transfer,
not another purchase: review its payment category so it stays out of
income/expense totals. Card balances are not spendable bank cash.
[Transaction period preferences](transactions.md#review-a-period) control which
month recognizes purchases; cash projections use bill due dates.

## Record a transfer between your own accounts

- **Existing movements:** in Transactions, open either movement and its
  account-transfer section. Select the opposite movement on another bank
  account, optionally select a separate fee, review and confirm the link.
- **Manual accounts:** choose **New transfer** in Transactions. Select two
  active manual accounts, the amount sent and received in each native currency,
  date and optional explicit fee. Saving creates and links the movements.

Both principal movements must be included, pending or posted, and not already
linked to a transfer or investment reconciliation. Credit cards cannot use
this pairing workflow. Same-currency principal amounts must match; foreign
transfers preserve both amounts. For example, synthetic BRL 540 sent for USD 100
received displays an execution rate of 1 USD = 5.4 BRL. This is your transaction's
rate, not a market quote; Julius never infers a fee from an exchange spread.

Linked principal leaves income/expense totals but still moves account cash.
A separate fee follows normal expense rules. Unlink before editing a linked
manual movement. Unlinking keeps all movements and restores their existing
category/inclusion rules; ignoring or deleting any participant removes the link.
A transfer category can exclude a movement from totals without pairing it.

## Read amounts across currencies

Account balances and transaction rows retain their native currencies. The
Accounts and cards BRL summaries exclude foreign accounts. Consolidated
transaction reports convert eligible amounts to BRL using reporting-day FX;
Settings → General lets you express that BRL result in a reference currency
using a current rate. Changing this preference never rewrites original amounts.
Goals and planning amounts remain BRL.

Currency choices use Frankfurter coverage. You may enter another valid ISO
currency for native tracking, but that does not guarantee conversion support.
Missing pairs, unavailable dates or provider failures leave consolidated reports
incomplete or unavailable; native amounts remain usable. Stored older rates may
be reused with a stale-rate notice. Synchronizing a bank cannot add FX coverage.
