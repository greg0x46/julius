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

## Suite profiling (#68)

Benchmark the four expensive suites independently of #67's worker gains:

```sh
VITEST_MAX_WORKERS=1 npx vitest run src/pages/StatementImportPage.test.tsx src/pages/TransactionsPage.test.tsx src/components/InvestmentAssetSettings.test.tsx src/components/openBankingAndScenarios.test.tsx --reporter=verbose
```

Node 24.14.0, Vitest 4.1.11; three sequential runs, no concurrent checks.
Keep the existing project split, isolation, real Ant Design controls and
async timeouts unchanged. All baseline runs passed 4 files / 35 tests.

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

- Statement import: pagination and short-file warning tests can render
  `StatementPreviewRows` directly. Other page tests still cover upload,
  approval, row decisions, loading, stale responses, retries and mobile actions.
  Plain account-name entry can use paste instead of five updates per fixture.
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
