# Helpers for the release-script tests (source it). No framework: each test file makes its checks with
# ok/eq/has/lacks and ends with `finish`, whose exit code is 0 only if every check passed.
# Needs git and bash only. Every repository is a throwaway one under a temp dir; the real repo is never written to.
set -Eeuo pipefail

TESTS="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPTS="$(cd "$TESTS/.." && pwd)"
SRC="$(cd "$SCRIPTS/.." && pwd)"
passed=0 failed=0

pass() { passed=$((passed + 1)); echo "  ok   $1"; }
fail() { failed=$((failed + 1)); echo "  FAIL $1"; shift; (($# == 0)) || printf '       %s\n' "$@"; }
ok()   { local name="$1"; shift; if "$@"; then pass "$name"; else fail "$name"; fi; }             # name command...
eq()   { if [[ "$2" == "$3" ]]; then pass "$1"; else fail "$1" "expected: $2" "actual:   $3"; fi; } # name expected actual
has()  { if grep -qF -- "$2" <<<"$3"; then pass "$1"; else fail "$1" "missing: $2" "in: ${3:0:600}"; fi; }
lacks() { if grep -qF -- "$2" <<<"$3"; then fail "$1" "unexpected: $2" "in: ${3:0:600}"; else pass "$1"; fi; }
section() { echo "$1"; }
finish() {
  echo "$(basename "$0"): $passed passed, $failed failed"
  ((failed == 0))
}

# Isolated environment: scratch dir T, empty HOME, no global git config, fixed identity and dates.
setup() {
  T=$(mktemp -d)
  trap 'rm -rf "$T"' EXIT
  mkdir -p "$T/home"
  export HOME="$T/home" GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null
  export GIT_AUTHOR_NAME=test GIT_AUTHOR_EMAIL=test@example.invalid GIT_COMMITTER_NAME=test GIT_COMMITTER_EMAIL=test@example.invalid
  export GIT_AUTHOR_DATE=2026-01-01T00:00:00Z GIT_COMMITTER_DATE=2026-01-01T00:00:00Z
  unset CHANGELOG_DATE CHANGELOG_FILE JULIUS_REPO_URL
}

# new_repo [dir] : an empty repository on branch main, becomes the cwd.
new_repo() {
  REPO="${1:-$T/repo}"
  rm -rf "$REPO"
  git init -q -b main "$REPO"
  cd "$REPO"
}

# c <subject> [body] : a commit with that message touching its own file (so `git revert` has something to undo;
# the history is synthetic and the content is irrelevant).
CN=0
c() {
  CN=$((CN + 1))
  echo "$CN" >"f$CN.txt"
  git add "f$CN.txt"
  if (($# > 1)); then git commit -q -m "$1" -m "$2"; else git commit -q -m "$1"; fi
}
head_sha() { git rev-parse HEAD; }

# Runs a command, capturing stdout in OUT, stderr in ERR and the status in RC (never aborts the test).
run() {
  RC=0
  OUT=$("$@" 2>"$T/err") || RC=$?
  ERR=$(<"$T/err")
}
