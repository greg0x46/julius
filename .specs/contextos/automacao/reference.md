# Automation

> Living reference — what exists and works today. Keep it updated when the
> feature changes. Engines behind it: Automation, built on the rules engine
> (sections 2 and 3 of `.specs/motores-de-dominio.md`).

## What it is

Condition-based rules that categorize or ignore transactions automatically and
can be applied retroactively to existing ones. It is also the link between a
rule and the reconciliation of a recurring commitment (see Recurrences).

## Backend

`internal/automation` — `Rule`/`Action`/`Condition`, matching via
`internal/rules` (pure, stateless composable matching core:
`Condition`/`Operator`/`LogicOperator`/`Matches`), application (`apply.go`) with
actions `ignore`, `set_category`, `reconcile`. The `reconcile` action ties the
rule to a recurring commitment without touching the transaction directly;
reconcile-only rules are skipped by the normal apply flow
(`isReconcileOnlyRule`). Combined `set_category` + `reconcile` rules still
apply categorization on new entries, changed sync entries and retroactive
passes, without overriding manual category decisions. Apply candidates include
absolute effective money (account currency first) and occurrence day, so
amount/day conditions work in both apply paths as well as reconciliation.
Both apply paths decode occurrence timestamps and exact amounts through
`internal/db/types` before matching, including native driver timestamps.
Reconciliation candidates also carry the same description, account name and
institution, and card number, so both actions evaluate the complete shared
conditions.

During sync, `NewTransactionHook` uses the entry's transaction `Querier` to
apply rules and the payable-unlink hook. It opens no inner transaction: a
failure in any action rolls back the entry, its decisions and its events
together. It runs on inserts and on updates with a different hash; already
completed `unchanged` records do not re-apply rules.

`set_category` accepts any active category, including `kind='transfer'` ones —
that is how transfers are removed from totals in bulk without `ignore` (see
Categories). The recurrence form's category picker still hides transfer
categories: a recurring commitment projects income or expense.

The `reconcile` rule is no longer the final word: the user can detach an
occurrence or reconcile it with another transaction, and that decision beats
the rule (`recurrences.Reconciler`). The rule still applies to every occurrence
without a manual decision.

## HTTP routes

`GET/POST /api/automation-rules`,
`PUT/PATCH/DELETE /api/automation-rules/{id}`,
`GET /api/automation-rules/condition-options`.

## Frontend

`/automacoes` — the reconcile form includes an optional, searchable and clearable
"Categoria a aplicar" selector. For new rules it defaults to the selected or
new commitment's category until explicitly edited or cleared. Existing rules
load exactly their saved actions, including reconcile-only rules. Selecting a
category adds `set_category` to the same rule; clearing removes it, preserving
`reconcile` and its conditions. Categorization can be applied retroactively
from the same form. The list displays both action descriptions.

Automatic reconciliation retains its existing eligibility checks, including
the commitment's category; selecting a different action category does not
relax those checks.

## Monetary precision

Amount references and percentage tolerances are parsed and compared as exact
decimals, without floating-point conversion. Comparison is inclusive and does
not round either side ([policy](../../precisao-monetaria.md)).
