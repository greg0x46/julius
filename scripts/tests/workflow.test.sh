#!/usr/bin/env bash
# Static checks of .github/workflows/ci.yml and .github/dependabot.yml: A11 (a push to stage runs the tests but the
# release job does not publish), main-only gated release, minimal permissions, no secrets, and the release-asset
# contract the production deploy relies on. Evaluates the `if:` conditions for concrete events instead of grepping.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

if ! python3 -c 'import yaml' 2>/dev/null; then
  if [[ "${REQUIRE_YAML:-}" == 1 ]]; then echo "workflow.test.sh: PyYAML is required"; exit 1; fi
  echo "SKIP workflow.test.sh: python3 with PyYAML not available (set REQUIRE_YAML=1 to make this an error)"
  exit 0
fi

out=$(python3 -I - "$SRC" <<'PY'
import re, sys, yaml

src = sys.argv[1]
checks = []
def check(name, cond, detail=""):
    checks.append((name, bool(cond), detail))

wf = yaml.safe_load(open(f"{src}/.github/workflows/ci.yml"))
raw = open(f"{src}/.github/workflows/ci.yml").read()
on = wf.get(True, wf.get("on"))  # PyYAML (YAML 1.1) reads the bare key `on` as True
jobs = wf["jobs"]

def evaluate(cond, ctx):
    """Tiny evaluator for `a == 'x' && b == 'y'` (the only shape allowed; anything else fails the test)."""
    if cond is None:
        return True
    parts = [p.strip() for p in str(cond).split("&&")]
    for p in parts:
        m = re.fullmatch(r"([a-z_.]+) == '([^']*)'", p)
        if not m:
            raise ValueError(f"unsupported condition: {cond!r}")
        if ctx.get(m.group(1)) != m.group(2):
            return False
    return True

check("push triggers main and stage", on["push"]["branches"] == ["main", "stage"], on["push"])
check("pull_request targets main and stage", on["pull_request"]["branches"] == ["main", "stage"], on["pull_request"])
check("no other triggers (no pull_request_target/workflow_run/schedule/tags)", set(on) == {"push", "pull_request"}, list(on))
check("jobs: backend, frontend, release", list(jobs) == ["backend", "frontend", "release"], list(jobs))
check("required status names are unchanged", jobs["backend"]["name"] == "Backend (Go)" and jobs["frontend"]["name"] == "Frontend (Node)")
check("release needs both test jobs", sorted(jobs["release"]["needs"]) == ["backend", "frontend"], jobs["release"].get("needs"))

events = {
    "push to stage":        {"github.event_name": "push", "github.ref": "refs/heads/stage"},
    "push to main":         {"github.event_name": "push", "github.ref": "refs/heads/main"},
    "PR into stage":        {"github.event_name": "pull_request", "github.ref": "refs/pull/1/merge"},
    "PR (promotion) to main": {"github.event_name": "pull_request", "github.ref": "refs/pull/2/merge"},
    "push of a release/* branch": {"github.event_name": "push", "github.ref": "refs/heads/release/v1.0.0"},
}
for name, ctx in events.items():
    on_push = ctx["github.event_name"] == "push"
    ref_ok = (not on_push) or ctx["github.ref"].removeprefix("refs/heads/") in on["push"]["branches"]
    runs_tests = ref_ok and all(evaluate(jobs[j].get("if"), ctx) for j in ("backend", "frontend"))
    publishes = runs_tests and evaluate(jobs["release"].get("if"), ctx)
    expect_tests = name != "push of a release/* branch"
    expect_publish = name == "push to main"
    check(f"A11 {name}: tests {'run' if expect_tests else 'do not run'}", runs_tests == expect_tests)
    check(f"A11 {name}: release {'publishes' if expect_publish else 'does not publish'}", publishes == expect_publish)

check("release is gated exactly by push on refs/heads/main",
      jobs["release"]["if"] == "github.event_name == 'push' && github.ref == 'refs/heads/main'", jobs["release"].get("if"))
check("test jobs have no job-level if (they always run for stage)", "if" not in jobs["backend"] and "if" not in jobs["frontend"])
check("no step on stage-reachable jobs creates tags/releases", "gh release" not in str(jobs["backend"]) + str(jobs["frontend"]))

check("workflow default permissions: contents read only", wf["permissions"] == {"contents": "read"}, wf.get("permissions"))
check("only the release job has job permissions",
      "permissions" not in jobs["backend"] and "permissions" not in jobs["frontend"])
check("release permissions are the minimal set already needed",
      jobs["release"]["permissions"] == {"contents": "write", "id-token": "write", "attestations": "write"}, jobs["release"]["permissions"])
check("no secrets referenced", "secrets." not in raw and "secrets:" not in raw)
check("only the built-in token is used", set(re.findall(r"\$\{\{\s*(github\.token)\s*\}\}", raw)) == {"github.token"})
check("every action is pinned to a commit SHA",
      all(re.fullmatch(r"[^@]+@[0-9a-f]{40}", u) for u in re.findall(r"uses:\s*(\S+)", raw)), re.findall(r"uses:\s*(\S+)", raw))
check("main and stage runs are never cancelled", "cancel-in-progress: ${{ github.event_name == 'pull_request' }}" in raw)
check("release queue is still serialised, never cancelled",
      jobs["release"]["concurrency"] == {"group": "release", "cancel-in-progress": False})

steps = jobs["release"]["steps"]
names = [s.get("name") or s.get("uses", "").split("@")[0] for s in steps]
def idx(prefix):
    return next(i for i, n in enumerate(names) if n.startswith(prefix))
run = {s.get("name"): s.get("run", "") for s in steps}
check("version comes from scripts/next-version.sh", "tag=$(scripts/next-version.sh)" in run["Select version"])
check("changelog step runs after version and before build/publish",
      idx("Select version") < idx("Release notes from CHANGELOG.md") < idx("Build") < idx("Publish release"), names)
notes_step = next(s for s in steps if s.get("name") == "Release notes from CHANGELOG.md")
check("notes step is conditional on a version", notes_step["if"] == "steps.version.outputs.tag != ''")
check("notes come from scripts/changelog-section.sh and fail on mismatch",
      "scripts/changelog-section.sh --latest" in notes_step["run"] and "exit 1" in notes_step["run"] and 'scripts/changelog-section.sh "$TAG"' in notes_step["run"])
check("release notes are not recomputed with next-version.sh --notes", "--notes" not in raw.replace("--notes-file", ""))
build_step = next(s for s in steps if s.get("name") == "Build")
check("release frontend metadata uses selected tag and exact build commit",
      build_step.get("env") == {"VITE_APP_VERSION": "${{ steps.version.outputs.tag }}", "VITE_APP_COMMIT": "${{ github.sha }}"})
frontend_build = next(s for s in jobs["frontend"]["steps"] if s.get("run") == "npm run build")
check("stage frontend build is explicit; other check builds say dev",
      frontend_build.get("env") == {"VITE_APP_VERSION": "${{ github.ref == 'refs/heads/stage' && 'stage' || 'dev' }}", "VITE_APP_COMMIT": "${{ github.sha }}"})
publish = run["Publish release"]
check("publish uses the notes file", "--notes-file notes.md" in publish)
check("publish keeps target, title and latest", '--target "$GITHUB_SHA"' in publish and '--title "$TAG"' in publish and "--latest" in publish)
for asset in ("julius-linux-amd64", "julius-linux-amd64.sigstore.json", "SHA256SUMS"):
    check(f"release asset kept: {asset}", re.search(rf"^\s+{re.escape(asset)}\s*\\?$", publish, re.M) is not None)
check("binary is renamed and checksummed as before",
      "mv julius julius-linux-amd64" in run["Build"] and "sha256sum julius-linux-amd64 > SHA256SUMS" in run["Build"])
attest = next(s for s in steps if s.get("name") == "Attest provenance")
check("provenance attestation kept", attest["uses"].startswith("actions/attest-build-provenance@") and attest["with"]["subject-path"] == "julius-linux-amd64")
check("release job still checks out full history", steps[0]["with"]["fetch-depth"] == 0)
check("backend job runs the release script tests", any(s.get("run") == "scripts/tests/run.sh" for s in jobs["backend"]["steps"]))

dep = yaml.safe_load(open(f"{src}/.github/dependabot.yml"))
ups = dep["updates"]
check("dependabot: github-actions, gomod, npm", [u["package-ecosystem"] for u in ups] == ["github-actions", "gomod", "npm"])
check("dependabot: every version update targets stage", all(u.get("target-branch") == "stage" for u in ups), [u.get("target-branch") for u in ups])
check("dependabot: weekly schedule and grouping kept", all(u["schedule"]["interval"] == "weekly" and u.get("groups") for u in ups))
by = {u["package-ecosystem"]: u["commit-message"] for u in ups}
check("dependabot: workflows use build(ci)", by["github-actions"]["prefix"] == "build(ci)" and "include" not in by["github-actions"], by["github-actions"])
check("dependabot: go modules use fix(deps)", by["gomod"] == {"prefix": "fix", "include": "scope"}, by["gomod"])
check("dependabot: npm prod uses fix(deps), dev build(deps-dev)", by["npm"] == {"prefix": "fix", "prefix-development": "build", "include": "scope"}, by["npm"])

for name, ok_, detail in checks:
    print(("PASS " if ok_ else "FAIL ") + name + ("" if ok_ else f"  [{detail}]"))
PY
) || true
while IFS= read -r line; do
  case "$line" in
    "PASS "*) pass "${line#PASS }" ;;
    "FAIL "*) fail "${line#FAIL }" ;;
    *) fail "python: $line" ;;
  esac
done <<<"$out"
[[ -n "$out" ]] || fail "workflow checks produced no output"
finish
