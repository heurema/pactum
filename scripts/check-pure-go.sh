#!/usr/bin/env bash
# Verify that pactum remains a pure-Go binary (no tree-sitter / CGO deps).
# Runs four checks in sequence; any failure sets FAIL=1 and the final exit is 1.
set -euo pipefail

REPO_ROOT="$(CDPATH= cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

FAIL=0
fail() { echo "FAIL: $*" >&2; FAIL=1; }
ok()   { echo "ok:   $*"; }

# Shared cleanup: temp dirs are registered here and removed on exit.
CLEANUP_DIRS=()
PACTUM_BIN=""
cleanup() {
  rm -rf "${CLEANUP_DIRS[@]}"
  [[ -n "$PACTUM_BIN" ]] && rm -f "$PACTUM_BIN"
}
trap cleanup EXIT

# ===========================================================================
# (a) SOURCE GUARD
# Banned patterns must not appear in Go source files, go.mod, or go.sum.
# ===========================================================================
echo "=== (a) Source guard ==="

SOURCE_BANNED=(
  "github\\.com/tree-sitter"
  "go-tree-sitter"
  "tree-sitter"
  "internal/codeindex"
  "\\bcodeindex\\b"
  "code-items\\.jsonl"
  "\\bcode_items\\b"
  "\\bcode_item\\b"
)
for pat in "${SOURCE_BANNED[@]}"; do
  hits=$(grep -rE "$pat" --include='*.go' internal/ cmd/ 2>/dev/null || true)
  if [[ -n "$hits" ]]; then
    fail "banned pattern \"$pat\" in Go sources"
  else
    ok "Go: \"$pat\" absent"
  fi
  mod_hits=$(grep -E -e "$pat" go.mod go.sum 2>/dev/null || true)
  if [[ -n "$mod_hits" ]]; then
    fail "banned pattern \"$pat\" in module files"
  else
    ok "module files: \"$pat\" absent"
  fi
done

# ===========================================================================
# (b) DOC GUARD
# Removed user-facing features must not appear in the listed doc files.
# Historical/design docs that archive past investigation rationale are excluded.
# ===========================================================================
echo "=== (b) Doc guard ==="

DOC_FILES=(
  README.md
  docs/install.md
  docs/workspace.md
  docs/flow.md
  docs/agent-skill.md
  "assets/agent-skills/pactum/SKILL.md"
  "assets/agent-skills/pactum/references/workflow.md"
)

DOC_BANNED=(
  "github\\.com/tree-sitter"
  "go-tree-sitter"
  "tree-sitter"
  "internal/codeindex"
  "\\bcodeindex\\b"
  "code-items\\.jsonl"
  "\\bcode_items\\b"
  "\\bcode_item\\b"
  "--symbol"
)

for pat in "${DOC_BANNED[@]}"; do
  hits=$(grep -rE -e "$pat" "${DOC_FILES[@]}" 2>/dev/null || true)
  if [[ -n "$hits" ]]; then
    fail "banned doc pattern \"$pat\" found"
  else
    ok "docs: \"$pat\" absent"
  fi
done

# ===========================================================================
# Build the binary once; reused by (c) and (d).
# ===========================================================================
echo "=== Building pactum binary ==="
PACTUM_BIN=$(mktemp /tmp/pactum.XXXXXX)
if ! CGO_ENABLED=0 go build -o "$PACTUM_BIN" ./cmd/pactum 2>&1; then
  fail "CGO_ENABLED=0 go build ./cmd/pactum failed"
  exit 1
fi
ok "CGO_ENABLED=0 go build ./cmd/pactum succeeded"

# ===========================================================================
# (c) BACKWARD-COMPAT CHECK
# A config file containing map.code_index: auto must load without a yaml error
# and the command must exit 0.
# ===========================================================================
echo "=== (c) Backward-compat: map.code_index config ==="

TMPDIR_BC=$(mktemp -d)
CLEANUP_DIRS+=("$TMPDIR_BC")

git -C "$TMPDIR_BC" init --quiet
# Prime the workspace with the default config, then overwrite it with the legacy field.
(cd "$TMPDIR_BC" && CGO_ENABLED=0 "$PACTUM_BIN" init . >/dev/null 2>&1)
cat > "$TMPDIR_BC/.heurema/pactum/config.yaml" <<'YAML'
version: v1alpha1
agents:
  - name: claude
    model: claude-opus-4-8
map:
  max_file_bytes: 500000
  code_index: auto
out_of_scope: block
YAML
bc_out=$(cd "$TMPDIR_BC" && CGO_ENABLED=0 "$PACTUM_BIN" status 2>&1) && bc_exit=0 || bc_exit=$?
if echo "$bc_out" | grep -qE 'unknown field|yaml:'; then
  fail "map.code_index: auto produced a yaml decode error: $bc_out"
elif [[ $bc_exit -ne 0 ]]; then
  fail "pactum status exited $bc_exit with map.code_index config (expected 0): $bc_out"
else
  ok "map.code_index: auto loads without error (exit 0)"
fi

# ===========================================================================
# (d) SMOKE TEST
# Run pactum init, verify expected map artifacts, verify search works.
# ===========================================================================
echo "=== (d) Smoke test: pure-Go init and search ==="

TMPDIR_SMOKE=$(mktemp -d)
CLEANUP_DIRS+=("$TMPDIR_SMOKE")

git -C "$TMPDIR_SMOKE" init --quiet
printf 'package main\n\nfunc main() {}\n' > "$TMPDIR_SMOKE/main.go"

smoke_out=$(cd "$TMPDIR_SMOKE" && CGO_ENABLED=0 "$PACTUM_BIN" init . 2>&1) && smoke_exit=0 || smoke_exit=$?
if [[ $smoke_exit -ne 0 ]]; then
  fail "pactum init failed (exit $smoke_exit): $smoke_out"
fi

MAP_DIR="$TMPDIR_SMOKE/.heurema/pactum/map"
if [[ -f "$MAP_DIR/files.jsonl" ]]; then
  ok "files.jsonl present"
else
  fail "files.jsonl not created by pactum init"
fi
if [[ -f "$MAP_DIR/search.sqlite" ]]; then
  ok "search.sqlite present"
else
  fail "search.sqlite not created by pactum init"
fi
if [[ -f "$MAP_DIR/code-items.jsonl" ]]; then
  fail "code-items.jsonl must not be created by pactum init"
else
  ok "code-items.jsonl absent"
fi

search_out=$(cd "$TMPDIR_SMOKE" && CGO_ENABLED=0 "$PACTUM_BIN" search main --kind file 2>&1) && search_exit=0 || search_exit=$?
if [[ $search_exit -eq 0 ]]; then
  ok "pactum search main --kind file succeeded"
else
  fail "pactum search main --kind file failed (exit $search_exit): $search_out"
fi

# ===========================================================================
echo ""
if [[ $FAIL -ne 0 ]]; then
  echo "RESULT: FAIL" >&2
  exit 1
fi
echo "RESULT: all checks passed"
