# Transactions and recurring payments

[All documentation](../README.md) · [Accounts](accounts.md) ·
[Financial planning](financial-planning.md)

## Review a period

Open Transactions (`/transacoes`), choose a period, and filter by account, card,
category, description, direction or origin. Open a row to inspect its money,
category, inclusion and reconciliation. Rows keep their native currency;
consolidated totals disclose missing conversions and native currency totals.

In Settings → General, choose whether card purchases count in the purchase
month or the payment month. Payment-month reporting uses the linked bill's due
date; without one, it estimates one month after the transaction date, clamped to
month end. Bank transactions keep their transaction date. Installments are the
provider's individual rows, not a new schedule Julius creates from one purchase.

## Record a manual transaction

1. Choose **New manual entry** in Transactions and select an existing active
   account. Create a [manual account](accounts.md#track-an-account-manually)
   first if needed.
2. Enter the description, date, direction and amount in the account's currency;
   optionally select a category. Save and review the row.
3. Use the row's edit/delete actions to correct a manual entry. Imported
   transactions cannot be edited or deleted through these actions.

On a manual bank account, posted movements change its derived balance. On a
connected account they do not change its provider balance. Manual credit-card
entries do affect the calculated bill. Undo transfer or investment links before
editing a linked manual transaction.

## Categorize and control inclusion

Create or edit categories under Settings → Categories. In a transaction's detail,
choose a category; manual choices take priority over automation, learned choices
and provider mappings. For later similar descriptions and movement types,
Julius learns from your latest manual choice; it does not recategorize all older
transactions. Installment categorization can propagate to related installments.

Use **Ignore** for a duplicate or another movement you want out of reporting.
It remains in history and does not rewrite a bank balance. Restoring inclusion
makes the movement follow normal eligibility again; it does not restore removed
reconciliation links automatically. Pending and posted movements can count;
unsupported statuses, missing/zero money and unclassified movements cannot.
A missing category alone does not exclude an otherwise eligible movement.

Use a transfer category or [link an own-account transfer](accounts.md#record-a-transfer-between-your-own-accounts)
to keep principal out of income/expense totals. For investment contributions or
withdrawals, [reconcile the investment movement](investments.md#avoid-counting-a-bank-movement-twice):
only its linked portion leaves those totals, while the full bank movement
continues to affect cash.

## Automate repeat decisions

In Settings → Automation, create a rule, choose its conditions and an action:
categorize, ignore, or reconcile a recurring commitment. Review its activation
state and save. Use retroactive application when you deliberately want to apply
it to existing transactions; otherwise it applies to new entries and changed
sync entries. Rules do not overwrite a manual category decision.

A reconciliation rule can also apply a category with the same conditions. Review
that optional category before saving; clearing it leaves reconciliation alone.
Deleting a rule does not undo decisions it already applied.

## Track recurring income or expenses

1. Open Recurrences (`/recorrencias`) and add a commitment with direction,
   positive BRL amount, category, monthly/annual schedule and date range;
   optionally restrict it to an account.
2. Create a reconciliation rule in Automation targeting that commitment, or
   reconcile occurrences manually from the recurrence's expanded list.
3. Review occurrences for the period. Link the actual transaction when it
   arrives; the reconciled occurrence stops projecting, avoiding a duplicate.

For example, a synthetic BRL 80 monthly subscription remains planned until a
matching actual charge is recognized. An automatic rule searches the occurrence's
month and respects any account restriction. Manual reconciliation allows a
nearby transaction, including across a month boundary, with compatible direction,
BRL money and non-ignored inclusion. Category equality is not required.

Detach an incorrect match to keep the occurrence projected, choose another
transaction, or return it to automatic matching. Manual decisions take priority
over rules. Ignoring a linked transaction releases its link. These schedules
record expectations; they do not execute payments.

## Discover recurring commitments

Prerequisites: posted BRL history with at least four monthly payments for the
same merchant and account, or three consecutive annual payments. Installments,
transfers, ignored transactions and irregular patterns are excluded. Have an
active category available for each commitment.

1. Open Recurrences (`/recorrencias`) and choose **Discover recurrences**. Inspect
   each estimate, schedule, confidence, explanation and supporting transactions.
   Confidence describes a historical pattern, not a guarantee of future payments.
2. Choose **Review**. Edit the name, direction, positive BRL amount, category,
   account, monthly/annual calendar and date range. Select an active category
   when the suggestion has none.
3. Choose **Update preview** to generate reconciliation conditions and see their
   historical matches. Edit conditions if needed and update the preview again.
   All conditions must match. Check unmatched evidence, possible false positives
   and overlaps with existing rules or recurrences. Ambiguity requires explicit
   acknowledgment; cross-month day windows must be corrected before creation.
4. Choose **Create recurrence and rule**, or **Finish review** and select reviewed
   suggestions for **Create selected**. Each accepted item creates its schedule
   and active reconciliation rule together. Failed items remain for correction;
   successful items appear in the recurrence list.
5. Choose **Dismiss** for an unwanted pattern. It stays hidden after reloading,
   even when more transactions appear for that pattern.

Edits require a fresh preview. If historical data or competing commitments change,
update discovery and review again. Retrying a successful approval creates no
second commitment. Discovery and previews create nothing; approval changes no
existing transaction, category, balance or manual reconciliation decision. The
new schedule enters projections through the normal recurrence behavior. Historical
matching is a backtest, not a request to backdate the proposed start date.

## Import a statement

1. In Accounts and cards, choose **Import statement**. Download the template
   if needed: currently the accepted format is Flash CSV, in BRL, with date,
   time, movement, value, payment method and balance columns.
2. Upload the CSV (up to 2 MiB and 10,000 rows) and choose an existing file
   account or name a new one. Previewing writes nothing.
3. Review the period, new/duplicate/invalid rows, balance and warnings. For each
   balance conflict, choose to keep the existing row or import another movement.
4. Confirm. Importing only valid rows requires an explicit choice. Review the
   resulting account and import history.

Re-uploading the same file or overlapping periods skips recognized duplicates
within that file account. Matches against Pluggy or manual entries are warnings,
not automatic merges: importing them can still duplicate a real movement.
New rows receive learned categories and automation. Existing decisions survive.

The balance advances only with a newer, unambiguous valid balance. Invalid or
conflicting rows can prevent a balance update even when valid movements import.
Continuity gaps are warnings; Julius does not invent missing transactions.
File accounts are separate from Pluggy connections and do not sync automatically.
