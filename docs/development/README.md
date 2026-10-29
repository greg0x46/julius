# Development references

Read [contributor conventions](../../AGENTS.md) for commands and workflow, then:

1. [Architecture](architecture.md): domain engines, boundaries and cross-cutting principles.
2. [Financial invariants](financial-invariants.md): current rules, owners and pinning tests.
3. [Monetary precision](monetary-precision.md): exact arithmetic, rounding and presentation.
4. [API contracts](api-contracts.md): wire formats, states and calculation ownership.
5. [Context decisions](context-decisions.md): shared policies, exceptions and links to feature inventories.

Update these references in the same change as behavior or test names. The
invariant matrix is checked by `TestMatrixReferencesExistingTests`; shared
Go/TypeScript fixtures check monetary precision and financial API contracts.
For a focused reference check, run from the repository root:

```sh
go test -p 1 -count=1 ./internal/invariants ./internal/httpapi ./internal/money
cd frontend
npm test -- --run src/api/financialContracts.test.ts src/presentation/money.test.ts
```

The Go command checks matrix citations and domain/API regressions. The frontend
command checks the shared contract and monetary fixtures. Renaming a cited test
must keep the matrix guard passing; changing a wire contract requires reviewing
both sides of its fixture. Also check relative Markdown links when moving pages.

Run the full commands in [AGENTS.md](../../AGENTS.md); PostgreSQL tests require a
disposable database because they reset its schema.

See [frontend testing](../../frontend/TESTING.md) and the
[database migration reference](../../internal/db/migrations/README.md) for
focused validation. Legacy [context inventories and proposals](../../.specs/README.md)
remain during migration; proposals describe unimplemented work only.

Return to the [documentation index](../README.md).
