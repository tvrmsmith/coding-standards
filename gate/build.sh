#!/usr/bin/env bash
# Builds metric-gate as per-platform static binaries into gate/bin.
#
# ADR 0001 ships the gate as static binaries so it links against no language
# toolchain and its behaviour is fixed to its version rather than to whichever
# runtime a shim resolved. CGO_ENABLED=0 is what makes the result static, and
# -trimpath keeps the build reproducible across machines.
set -euo pipefail

cd "$(dirname "$0")"

out=bin
mkdir -p "$out"

platforms=(
  darwin/amd64
  darwin/arm64
  linux/amd64
  linux/arm64
  windows/amd64
)

for platform in "${platforms[@]}"; do
  os=${platform%/*}
  arch=${platform#*/}
  name="metric-gate-${os}-${arch}"
  if [ "$os" = windows ]; then
    name="${name}.exe"
  fi
  echo "building ${out}/${name}"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
    go build -trimpath -ldflags "-s -w" -o "${out}/${name}" ./cmd/metric-gate
done
