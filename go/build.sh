#!/usr/bin/env bash
#
# Builds the personal golangci-lint binary into go/bin/tvrmsmith-gcl.
#
# A module plugin is compiled into golangci-lint itself, so there is no "install the plugin"
# step: the binary *is* the plugin. `golangci-lint custom` reads .custom-gcl.yml next to this
# script, fetches the pinned golangci-lint source, adds the local plugin module and builds both.
#
# Idempotent, and the second run is fast — the Go build cache does the work. Rerun it after
# changing a rule, and after raising the pinned version in .custom-gcl.yml.
set -euo pipefail

cd "$(dirname "$0")"

pinned=$(sed -n 's/^version: *//p' .custom-gcl.yml | head -1)
[ -n "$pinned" ] || { echo "build: no version pinned in .custom-gcl.yml" >&2; exit 1; }

# `golangci-lint custom` is a subcommand of an installed golangci-lint, and it builds the
# version it is told to rather than its own. So the bootstrap binary only has to exist; the
# pinned version is what ends up in go/bin.
if ! command -v golangci-lint >/dev/null 2>&1; then
  echo "build: no golangci-lint on PATH — install one to run its 'custom' subcommand:" >&2
  echo "  go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$pinned" >&2
  exit 1
fi

echo "building golangci-lint $pinned with the personal plugin"
golangci-lint custom

built=bin/tvrmsmith-gcl
[ -x "$built" ] || { echo "build: golangci-lint custom did not produce $built" >&2; exit 1; }

# Prove the plugin is linked in rather than that the build exited zero: a plugin that failed to
# register leaves a perfectly good binary that silently enables nothing. `help linters` cannot
# answer this — it takes no --config, and a custom linter only exists once a config declares it
# — so the proof is a real run over test/smoke, which holds one deliberate violation.
found=$(cd test/smoke && "../../$built" run --config ../../golangci.yml --issues-exit-code 0 \
  --output.text.print-issued-lines=false --output.text.colors=false . 2>&1)
case "$found" in
  *tvrmsmith-comment-block-length*) ;;
  *) echo "build: $built reported nothing on the deliberate violation in test/smoke:" >&2
     printf '%s\n' "$found" | sed 's/^/  /' >&2
     exit 1 ;;
esac

echo "ok  $(cd "$(dirname "$built")" && pwd)/$(basename "$built")"
