# Frontend tests

`npm test` runs Vitest with the config in `vite.config.ts`. CI runs the full
suite with `npm test -- --run`.

## Projects

The suite is split into two Vitest projects that run in the same pool:

- `dom`: jsdom plus `src/test/setup.dom.ts` (jest-dom matchers,
  `matchMedia`/`ResizeObserver` stubs, Testing Library cleanup). Runs every
  `*.test.tsx` and the `.ts` tests listed in `domTsTests`.
- `node`: plain Node environment, no setup file. Runs every other
  `*.test.ts` (API contracts, transport, presentation helpers, pure math).
  Node already provides `fetch`, `Response`, `File`, `Blob`, `FormData` and
  `Headers`.

A new `.ts` test that touches `window`, `document`, storage or jest-dom
matchers fails in `node` with a `ReferenceError` (or an unknown matcher).
Add its path to `domTsTests` in `vite.config.ts`, or name it `.tsx` if it
renders components.

`restoreMocks`, `clearMocks`, `pool: "threads"`, `testTimeout` and
`maxWorkers` are set once at the root and inherited by both projects
(`extends: true`). Each test file still runs isolated (`isolate` defaults to
`true`).

Run a single project:

```sh
npx vitest --run --project node
npx vitest --run --project dom
```

## Worker count

`maxWorkers` is 2. Because the projects inherit it, the `--maxWorkers` CLI
flag does not override it; use `VITEST_MAX_WORKERS=<n> npm test -- --run`
to try another value.

With Testing Library's default 1 s `asyncUtilTimeout`,
`src/components/investments/InvestmentPositionForm.test.tsx` failed in most
runs at 3 or more workers (table below): under CPU contention its first
`findByRole` timed out while antd rendered the drawer. `setup.dom.ts` now
raises `asyncUtilTimeout` to 5 s for every `findBy*`/`waitFor`, so slow
renders on a loaded runner wait longer instead of failing. The table was
measured before that change. 2 workers gives most of the gain and stays the
default.

## Benchmark

Local machine, 8 CPUs, Node 24, Vitest 4.1.11, sequential runs. Time is
Vitest's reported `Duration`. Peak RSS was not recorded (no `time -v`
available in the agent environment). Every run had 54 files / 419 tests.

| Config | Run 1 | Run 2 | Run 3 | Median | Failures |
| --- | --- | --- | --- | --- | --- |
| Baseline: jsdom everywhere, 1 worker | 208.62 s | 207.99 s | 208.14 s | 208.14 s | 0/3 |
| Split, 1 worker | 200.12 s | 199.83 s | 199.38 s | 199.83 s | 0/3 |
| Split, 2 workers (chosen) | 105.88 s | 109.59 s | 109.77 s | 109.59 s | 0/3 |
| Split, 3 workers | 81.50 s | 84.62 s | 84.57 s | 84.57 s | 3/3 |
| Split, 4 workers | 71.66 s | 73.49 s | 75.25 s | 73.49 s | 2/3 |

The chosen config cuts the median by 47% (208.14 s to 109.59 s). The split
alone saves about 8 s of jsdom setup (Vitest's `environment` time drops from
18.7 s to 11.1 s); the rest comes from the second worker. The 2-worker
config also passed an extra run under concurrent lint/build load (117.53 s,
not counted in the table).

With the 5 s timeout, the 2-worker config passed again locally (108.59 s).
Only local numbers were measured; GitHub Actions timings (`frontend` job on
`ubuntu-24.04`, 4 vCPU) are not recorded here and no benchmark workflow was
added.

## Financial API contracts

`src/api/financialContracts.test.ts` consumes the synthetic Go DTO fixture at
`internal/httpapi/testdata/financial-contracts.json`. The backend's
`TestFinancialContracts` pins its JSON to the actual response DTOs/converters;
frontend parsers must preserve every field, null and decimal string. Both run
in CI through the ordinary Go/Vitest suites. Tests include pending/posted and
nullable payloads, tiny/large decimals and money JSON round-trips. No DOM or
running server is needed. See [the contract inventory](../.specs/contratos-api.md).

After deliberately changing a DTO, review compatibility and update the fixture
from the repository root:

```sh
go test ./internal/httpapi -run '^TestFinancialContracts$' -count=1 -args -update-financial-contracts
cd frontend && npm run typecheck && npm test -- --run src/api/financialContracts.test.ts
```

Review the JSON diff; do not regenerate just to silence a failure. A change to
money type, nullability, date shape, required fields or enums needs matching
consumer changes and documentation. Cached-response optional fields have
explicit parser behavior; they must not invent business values from missing
data. For a quick failure check, temporarily change a DTO JSON tag and run the
Go contract test without the update flag: it must fail; restore it afterwards.

## Suite profiling (#68)

Benchmark the four expensive suites independently of #67's worker gains:

```sh
VITEST_MAX_WORKERS=1 npx vitest run src/pages/StatementImportPage.test.tsx src/pages/TransactionsPage.test.tsx src/components/InvestmentAssetSettings.test.tsx src/components/openBankingAndScenarios.test.tsx --reporter=verbose
```

Node 24.14.0, Vitest 4.1.11; three sequential runs, no concurrent checks.
The existing project split, isolation, real Ant Design controls and async
timeouts are unchanged. All baseline runs passed 4 files / 35 tests.

| Baseline | Run 1 | Run 2 | Run 3 | Median |
| --- | --- | --- | --- | --- |
| Duration | 97.56 s | 97.72 s | 97.26 s | 97.56 s |
| Imports | 22.66 s | 22.50 s | 22.46 s | 22.50 s |
| Tests | 72.64 s | 72.99 s | 72.57 s | 72.64 s |

The test fixtures are small and API calls are mocked, so these timings are
local rendering/query/interaction costs, not network latency. The import
paths reach the Ant Design and icon barrels, shared layout exports and (for
scenarios) ProTable. File isolation reloads this graph for each suite; these
four files alone spend about 22.5 s importing it.

A separate diagnostic run used Node's CPU profiler (excluded from the timing
medians): prefix the same Vitest command with
`node --cpu-prof --cpu-prof-dir=<temporary-directory> node_modules/vitest/vitest.mjs`
instead of `npx vitest`. Inspect each worker profile, not just the main thread.
The following sampled stack totals overlap (role queries and pointer checks
call `getComputedStyle`); they are not additive:

| Suite | Baseline test median | Style computation | Role/name queries | React rendering | Pointer checks | Node module loading |
| --- | --- | --- | --- | --- | --- | --- |
| StatementImportPage | 25.767 s | 22.53 s | 11.22 s | 3.17 s | 6.39 s | 4.72 s |
| TransactionsPage | 18.422 s | 16.98 s | 15.62 s | 1.92 s | 0.65 s | 4.60 s |
| InvestmentAssetSettings | 18.857 s | 18.41 s | 10.21 s | 3.90 s | 2.71 s | 4.68 s |
| openBankingAndScenarios | 9.597 s | 8.70 s | 1.70 s | 3.06 s | 3.96 s | 9.39 s |

Styles include jsdom `getComputedStyle`/style-rule matching; roles include
Testing Library role queries and accessible-name computation. Imports include
Node module-loader stacks, not Vite transform time. The scenario suite loads
ProTable as well as the shared controls, explaining its larger import cost.

Review decisions:

- Statement import: keep the page tests. A measured variant rendered
  `StatementPreviewRows` directly for pagination/warnings and pasted account
  names. The focused cases saved about 0.66 s, but the following conflict/retry
  case regressed by about 1.4 s. Restoring typing did not fix that regression,
  so the entire statement test change was discarded.
- Transactions: retain page integration for selection/refetch/write behavior
  and invalid-field focus. Scope panel queries to the real drawer instead of
  repeatedly computing names for all page/filter buttons.
- Investment assets: retain real selects and submitted payload assertions;
  scope queries to the drawer and use label queries for labelled text fields.
- Connections/scenarios: retain drawer destruction/reopening and the expanded
  scenario form. Paste plain ID/description fixtures, scope submit queries to
  their drawer/form, and keep numeric keyboard entry for `MoneyInput`.

No artificial delays, timeout increases, disabled accessibility/pointer checks,
or mocked UI widgets are needed. Module loading remains a separate cost;
changing production imports or adding bundler configuration is deferred to
keep this change limited to the measured test bottlenecks.

### Rejected intermediate optimization

The first variant combined focused statement preview rendering/plain-text
paste with the retained drawer/form changes. Three one-worker runs passed
all 35 tests, with durations 93.05 / 93.45 / 93.28 s (median 93.28 s), imports
22.32 / 22.29 / 22.53 s, and tests 68.49 / 68.93 / 68.50 s. Despite the overall
improvement, StatementImportPage regressed from a median 25.767 to 26.589 s.
A diagnostic run after restoring account-name typing still took 93.31 s,
with the conflict case at 3.316 s versus the baseline median 1.944 s. The
statement changes were therefore reverted before final validation.

### Retained changes

The final variant keeps the original StatementImportPage tests and only
changes the other three suites. Each of these runs passed 4 files / 35 tests:

| Final | Run 1 | Run 2 | Run 3 | Median |
| --- | --- | --- | --- | --- |
| Duration | 91.58 s | 93.52 s | 91.80 s | 91.80 s |
| Imports | 22.45 s | 22.57 s | 22.60 s | 22.57 s |
| Tests | 66.90 s | 68.72 s | 66.97 s | 66.97 s |

| Suite | Before test median | Final test median |
| --- | --- | --- |
| StatementImportPage | 25.767 s | 25.284 s |
| TransactionsPage | 18.422 s | 15.734 s |
| openBankingAndScenarios | 9.597 s | 9.332 s |
| InvestmentAssetSettings | 18.857 s | 16.785 s |

At the same worker count, total duration falls by 5.9% (97.56 to 91.80 s),
and test execution by 7.8% (72.64 to 66.97 s). Imports remain about the same;
this does not claim to solve the full-suite module initialization cost.
Timings are local, not GitHub Actions measurements, and should not be
combined with #67's concurrency gains. No full-suite before/after speedup
is claimed from the four-file benchmark.

CI now relies on `npm run build` (`tsc -b && vite build`) for strict type
validation, removing the separate `npm run typecheck` step. Both referenced
TypeScript projects still have `strict: true`, and the app project includes
the tests. The standalone typecheck command remains available locally.

Final full-suite validation at the unchanged two-worker default passed
68 files / 505 tests (163.59 s) while other repository checks were running.
This is a coverage/stability check, not a timing comparison with the isolated
one-worker benchmark above.
