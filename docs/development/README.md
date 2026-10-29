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
Run the commands in [AGENTS.md](../../AGENTS.md); PostgreSQL tests require a
disposable database because they reset its schema.

See [frontend testing](../../frontend/TESTING.md) and the
[database migration reference](../../internal/db/migrations/README.md) for
focused validation. Legacy [context inventories and proposals](../../.specs/README.md)
remain during migration; proposals describe unimplemented work only.

Return to the [documentation index](../README.md).
