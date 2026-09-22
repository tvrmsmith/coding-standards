#!/usr/bin/env bash
#
# Run one test unit the way no-mistakes' Test step demands, for the units .no-mistakes.yaml declares.
#
#   no-mistakes-unit.sh root <directory>
#   no-mistakes-unit.sh go
#   no-mistakes-unit.sh dotnet
#   no-mistakes-unit.sh node <package-dir>
#   no-mistakes-unit.sh select <directory>
#
# select prints the packages `root <directory>` would test, one per line, and runs nothing.
#
# The step refuses a green exit that proves nothing, so every unit writes a Cobertura or LCOV
# profile and a JUnit or TRX report into $NO_MISTAKES_COVERAGE_DIR. Before this file existed the
# step's agent wrote a new command every run, and each improvised one failed a different way.
#
# Runs from the repository root, where no-mistakes starts every unit command.
set -euo pipefail

root=$(pwd)

# Pinned rather than @latest, so a release of either tool cannot change a run's verdict.
gotestsum=gotest.tools/gotestsum@v1.13.0
cobertura=github.com/boumenot/gocover-cobertura@v1.5.0

# The changed paths, one per line. An empty result with a nonzero count means no-mistakes dropped
# the list for size, so the caller treats it as "everything changed".
changed_files() {
  printf '%s\n' "${NO_MISTAKES_CHANGED_FILES:-}" | sed '/^$/d'
}

list_is_complete() {
  local listed
  listed=$(changed_files | wc -l | tr -d ' ')
  [ "$listed" -eq "${NO_MISTAKES_CHANGED_FILE_COUNT:-0}" ]
}

# The root-module packages whose tests reach a changed file under the unit's directory. A file maps
# to the package directory holding it, or the nearest one above it inside the unit. A package is
# selected when it is that package or depends on it, test imports included, so a change under
# internal also runs gate/test, which drives it end to end. lint/test and gate/test build their
# binaries with `go build` rather than importing them, so each is added whenever a package it builds
# (lint-changed, or metric-gate and the stub extractor) is selected.
#
# Falls back to every package under the unit when a changed file there sits in no package, and to
# the whole module when no-mistakes dropped the changed list.
root_packages() {
  local unit=$1
  if ! list_is_complete; then
    echo ./...
    return
  fi
  local dirs touched=() file dir best
  dirs=$(go list -f '{{.Dir}}' "./$unit/..." | sed "s#^$root/##") || return
  while IFS= read -r file; do
    [ "${file#"$unit"/}" = "$file" ] && continue
    best=""
    while IFS= read -r dir; do
      if [ "${file#"$dir"/}" != "$file" ] && [ ${#dir} -gt ${#best} ]; then
        best=$dir
      fi
    done <<<"$dirs"
    if [ -z "$best" ]; then
      echo "./$unit/..."
      return
    fi
    touched+=("./$best")
  done < <(changed_files)
  local imports
  imports=$(go list "${touched[@]}" | tr '\n' ' ') || return
  # -test lists each package's test variants, whose deps carry the test imports. The variant names
  # ("p [p.test]", "p_test [p.test]", "p.test") all reduce to the package p.
  go list -test -f '{{.ImportPath}} {{join .Deps " "}}' ./... |
    awk -v want="$imports" '
      BEGIN { n = split(want, w, " "); for (i = 1; i <= n; i++) hit[w[i]] = 1 }
      { for (i = 1; i <= NF; i++) if ($i in hit) { print $1; next } }' |
    sed -E 's/ \[.*//; s/\.test$//; s/_test$//' |
    awk '{ print }
      sub("/lint/cmd/lint-changed$", "/lint/test") ||
      sub("/gate/(cmd/metric-gate|test/stub)$", "/gate/test")' | sort -u
}

# Tests the packages in the current directory's module and writes the unit's JUnit report and
# Cobertura profile, both named for the module. -coverpkg=./... instruments the whole module, so a
# package holding only tests (gate/test) still writes a profile, and the code gate/test
# drives across packages is credited.
go_test() {
  local name=$1 tmp
  shift
  tmp=$(mktemp -d)
  # shellcheck disable=SC2064 # tmp is local, so it has to expand before the function returns
  trap "rm -rf '$tmp'" EXIT
  go run "$gotestsum" --format pkgname --junitfile "$out/$name.junit.xml" -- \
    -count=1 -coverpkg=./... -coverprofile="$tmp/cover.out" "$@"
  go run "$cobertura" <"$tmp/cover.out" >"$out/$name.cobertura.xml"
}

unit_root() {
  local unit=${1:?usage: no-mistakes-unit.sh root <directory>} packages
  packages=$(root_packages "$unit")
  echo "no-mistakes-unit: testing $(echo "$packages" | tr "\n" " ")"
  # shellcheck disable=SC2086 # packages is a word list
  go_test "$(echo "$unit" | tr / -)" $packages
}

# The plugin runs whole, and go/test runs beside it, because go/test lints its fixtures with the
# golangci binary go/build.sh compiles from the plugin. The plugin's profile is what certifies a
# go/test change, since go/test holds only tests. The binary is rebuilt every run because a stale one
# silently tests old plugin code.
unit_go() {
  (cd go/plugin && go_test go-plugin ./...)
  local pinned
  pinned=$(sed -n 's/^version: *//p' go/.custom-gcl.yml | head -1)
  go install "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$pinned"
  ./go/build.sh
  (cd go/test && go_test go-test .)
}

# dotnet/tests/Consumer is left out because it exists to emit diagnostics, not to be run.
unit_dotnet() {
  local project
  for project in Tvrmsmith.Analyzers.Tests Tvrmsmith.MetricGate.CSharp.Tests; do
    dotnet test "dotnet/tests/$project/$project.csproj" \
      --collect "Code Coverage;Format=cobertura" \
      --logger "trx;LogFileName=$project.trx" \
      --results-directory "$out"
  done
}

# harness depends on eslint-config through `link:`, which reads that directory's own node_modules,
# and eslint-config links eslint-plugin the same way, so both install first, as ci.yml does.
unit_node() {
  local package=$1 dep
  for dep in packages/eslint-plugin-tvrmsmith packages/eslint-config-tvrmsmith; do
    [ "$dep" = "$package" ] && break
    (cd "$root/$dep" && pnpm install --frozen-lockfile --prefer-offline)
  done
  cd "$root/$package"
  pnpm install --frozen-lockfile --prefer-offline
  node --test --experimental-test-coverage \
    --test-reporter=spec --test-reporter-destination=stdout \
    --test-reporter=junit --test-reporter-destination="$out/junit.xml" \
    --test-reporter=lcov --test-reporter-destination="$out/lcov.info" \
    test/*.test.js
}

unit=${1:?usage: no-mistakes-unit.sh root|go|dotnet|node|select ...}
shift
if [ "$unit" = select ]; then
  root_packages "${1:?usage: no-mistakes-unit.sh select <directory>}"
  exit
fi
out=${NO_MISTAKES_COVERAGE_DIR:?set by no-mistakes to the directory the profile and report go in}
case $unit in
  root) unit_root "$@" ;;
  go) unit_go ;;
  dotnet) unit_dotnet ;;
  node) unit_node "$@" ;;
  *) echo "no-mistakes-unit: unknown unit kind '$unit'" >&2; exit 2 ;;
esac
