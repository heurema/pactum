#!/usr/bin/env bash
# check-no-treesitter.sh: fail if any tree-sitter or codeindex reference
# remains in go.mod, go.sum, or Go source files under internal/ and cmd/.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
FAIL=0

check() {
    local desc="$1"
    local pattern="$2"
    local target="$3"
    if grep -qr "$pattern" "$target" 2>/dev/null; then
        echo "FAIL: $desc — found '$pattern' in $target"
        grep -rn "$pattern" "$target" 2>/dev/null | head -5
        FAIL=1
    fi
}

check "tree-sitter in go.mod"  "tree-sitter"              "$REPO_ROOT/go.mod"
check "tree-sitter in go.sum"  "tree-sitter"              "$REPO_ROOT/go.sum"
check "go-tree-sitter in go.mod" "go-tree-sitter"         "$REPO_ROOT/go.mod"
check "go-tree-sitter in go.sum" "go-tree-sitter"         "$REPO_ROOT/go.sum"
check "codeindex import in Go source" "codeindex"          "$REPO_ROOT/internal"
check "codeindex import in Go source" "codeindex"          "$REPO_ROOT/cmd"

if [ $FAIL -ne 0 ]; then
    echo ""
    echo "check-no-treesitter: FAILED — tree-sitter must be fully removed"
    exit 1
fi

echo "check-no-treesitter: OK"
