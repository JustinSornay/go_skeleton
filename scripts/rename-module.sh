#!/usr/bin/env bash
# Renames the Go module path everywhere (go.mod, imports, docs).
# Usage: scripts/rename-module.sh github.com/acme/my-api
set -euo pipefail

new="${1:?usage: $0 <new/module/path>}"
cd "$(dirname "$0")/.."
old="$(go list -m)"

if [ "$old" = "$new" ]; then
  echo "module is already $new"
  exit 0
fi

go mod edit -module "$new"
grep -rlF --exclude-dir=.git --exclude-dir=bin "$old" . | while IFS= read -r f; do
  OLD="$old" NEW="$new" perl -pi -e 's/\Q$ENV{OLD}\E/$ENV{NEW}/g' "$f"
done
gofmt -w .
go build ./...
echo "module renamed: $old -> $new"
