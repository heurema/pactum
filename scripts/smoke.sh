#!/usr/bin/env bash
#
# scripts/smoke.sh — local end-to-end smoke test for Pactum.
#
# Builds the pactum binary from source, creates a throwaway git repository in a
# temp directory, and exercises the safe, deterministic command surface:
#
#   pactum init
#   pactum status
#   pactum run "<task>" --contract-only
#   pactum agents doctor
#
# It never launches a real agent and never calls Codex or Claude. The temp
# repository is removed on exit. Portable to Linux and macOS; bash required.

set -euo pipefail

# Neutralize CDPATH so `cd` never echoes the resolved directory inside command
# substitutions (which would corrupt the paths resolved below).
export CDPATH=

# Resolve the repository root from this script's location so the smoke test can
# be invoked from anywhere (make smoke, ./scripts/smoke.sh, etc.).
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

PACTUM_BIN="${REPO_ROOT}/bin/pactum"
TMP_REPO=""

cleanup() {
  if [ -n "${TMP_REPO}" ] && [ -d "${TMP_REPO}" ]; then
    rm -rf "${TMP_REPO}"
  fi
}
trap cleanup EXIT

step() {
  printf '\n=== %s ===\n' "$1"
}

step "Build pactum"
( cd "${REPO_ROOT}" && go build -o "${PACTUM_BIN}" ./cmd/pactum )
echo "built: ${PACTUM_BIN}"

step "Create throwaway git repository"
TMP_REPO="$(mktemp -d 2>/dev/null || mktemp -d -t pactum-smoke)"
echo "temp repo: ${TMP_REPO}"
cd "${TMP_REPO}"
git init -q
# Local identity only; never touches the user's global git config.
git config user.email "smoke@pactum.local"
git config user.name "Pactum Smoke"

mkdir -p cmd/demo
cat > cmd/demo/main.go <<'EOF'
package main

import "fmt"

func main() {
	fmt.Println("hello from the pactum smoke test")
}
EOF
cat > README.md <<'EOF'
# Smoke Test Repo

Temporary repository used by Pactum's smoke test.
EOF
git add -A
git commit -q -m "smoke baseline"

step "pactum init"
"${PACTUM_BIN}" init

step "pactum status"
"${PACTUM_BIN}" status

step "pactum run (contract-only, no execution)"
"${PACTUM_BIN}" run "smoke test Pactum install" --contract-only

step "pactum agents doctor (PATH check only)"
"${PACTUM_BIN}" agents doctor

printf '\nSMOKE OK: pactum built, initialized, and exercised the safe command surface.\n'
