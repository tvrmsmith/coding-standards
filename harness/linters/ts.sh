#!/usr/bin/env bash
#
# The TypeScript branch of lint-changed.sh: each changed file through its own package's ESLint,
# plus the personal layer. Sourced, never executed.
#
# ESLint runs per package, since flat config does not cascade and the wrapper loads the package's
# own config from the process cwd. So this lints the packages owning the changed files and hands
# their JSON reports up for the dispatcher's one `lint-changed` run, which keeps the findings
# touching a line the change actually wrote and drops the rest. Severity 1 and severity 2 are alike
# to it, per ADR 0010.
#
# This blocks, on ADR 0010's convention: the filter answers 2 when a finding survived it, and this
# branch returns 1 when it could not answer at all — a missing layering wrapper, an ESLint run that
# blew up, a filter that would not build. Past a genuine false positive there is one route, and
# `lint-changed` prints the exact command for it.

layer=${TVRMSMITH_ESLINT_LAYER:-$harness/eslint-layer.js}

ts_owns() {
  case "$1" in
    *.js|*.jsx|*.ts|*.tsx|*.mjs|*.cjs|*.mts|*.cts) return 0 ;;
    *) return 1 ;;
  esac
}

# ESLint itself always comes from the repo, never from the harness: the package pins the version
# its config was written for, ESLint 8 or 9, and its plugins resolve relative to it. Nothing is
# installed on demand. A package whose dependencies are not installed fails the commit rather than
# passing it, since every changed file in it would otherwise go unlinted and report clean.
_ts_has_eslint_bin() { [ -x "$1/node_modules/.bin/eslint" ]; }

ts_lint() {
  local file pkg eslint_dir eslint_bin status=0 present=() batch=()
  local report reports=() groups=$scratch/ts-owners
  local report_dir=$scratch/ts-reports package_count=0

  # 1, not 2: nothing was linted, so this is the gate breaking rather than a surviving finding.
  if [ ! -f "$layer" ]; then
    echo "lint-changed: no layering wrapper at $layer" >&2
    return 1
  fi

  # Deleted in the change, or named by --files and never there. ESLint reads the disk, so there is
  # nothing here for it to lint. The staged-versus-disk question a missing file raises is
  # lint-changed's, and it asks it across every staged path under --staged rather than only the
  # ones that reached a report, which is why nothing here returns early: add_reports at the bottom
  # has to be reached even when no package produced a report at all, or the dispatcher runs no
  # filter to ask it.
  for file in "$@"; do
    [ -e "$file" ] && present+=("$file")
  done

  # The package a file belongs to: nearest ancestor holding an ESLint config, since that is the
  # directory ESLint has to run from — flat config does not cascade, and the wrapper loads the
  # package's config from the process cwd. Each group is a package and its changed files relative to
  # it. owner_groups builds the filter first, so one that will not build fails the branch before a
  # single ESLint run, whose every report it would leave unreadable.
  owner_groups ts "$groups" ${present[@]+"${present[@]}"} || return 1
  mkdir -p "$report_dir" || return 1

  # With no package at all the loop reads nothing, which is the path that leaves the dispatcher's
  # filter to ask the divergence question on its own.
  while IFS= read -r -d '' pkg; do
    batch=()
    while IFS= read -r -d '' file && [ -n "$file" ]; do
      batch+=("$file")
    done

    # ancestor_with starts at the parent of the path it is given, so it takes a path *inside* the
    # package: the package's own node_modules is the first place to look.
    # 1, not 2, and not a skip: this package's changed files reached no linter at all, so a clean
    # answer over them would be a false clean, the one verdict a gate must never produce. The
    # missing-layer branch above and lint_changed_bin's missing-go branch refuse for the same reason.
    eslint_dir=$(ancestor_with "$pkg/." _ts_has_eslint_bin) || {
      status=1
      echo "lint-changed: $pkg has no installed eslint (run its package manager install)" >&2
      continue
    }
    eslint_bin=$repo_root/$eslint_dir/node_modules/.bin/eslint

    # One report per package, named by count rather than by the package path, which can hold any
    # byte a directory name can.
    package_count=$((package_count + 1))
    report=$report_dir/$package_count.json

    # Two flags are load-bearing, so each gets its reason.
    #
    # --format json onto stdout, captured into $report, which is the --report shape lint-changed
    # wants. ESLint's stdout under this formatter is the JSON document and nothing else, so unlike
    # golangci there is no human summary to redirect around.
    #
    # --no-warn-ignored because a file matched by the package's own flat-config `ignores` is
    # otherwise reported as {"ruleId":null,"severity":1,"message":"File ignored because of a
    # matching ignore pattern"} with no line at all, at exit 0. lint-changed reads a null ruleId
    # with no line as an UNPARSED finding that ignores scope, so every commit touching an ignored
    # file would block, clearable only by a path-less waiver. The flag suppresses it at source.
    #
    # ESLint's stderr is let through rather than captured: it carries the layering wrapper's own
    # notes and the real diagnostics of a run that went wrong, and a reader needs those where they
    # happen rather than folded into a failure message after the fact.
    ( cd "$pkg" && ESLINT_USE_FLAT_CONFIG=true "$eslint_bin" \
      --config "$layer" --format json --no-warn-ignored "${batch[@]}" ) </dev/null >"$report"
    case $? in
      # 0 and 1 both mean ESLint ran: 1 is its code for having reported an error, and the verdict
      # over a finding is lint-changed's now, not ESLint's. Anything else — a fatal config error is
      # its 2 — is the gate breaking, and it wrote no report to read.
      #
      # The subshell also returns 1 when `cd` fails or the binary cannot exec, and that writes no
      # report either. Reading an empty file as a report would have lint-changed answer "eslint:
      # malformed JSON: EOF", blaming the report format for a cd that failed, so an empty report
      # is named for what it is instead.
      0|1)
        if [ -s "$report" ]; then
          reports+=("$report")
        else
          status=1
          echo "=== $pkg — eslint produced no report ===" >&2
        fi
        ;;
      *) status=1; echo "=== $pkg — the lint run failed ===" >&2 ;;
    esac
  done <"$groups"

  # Every package that did produce a report still hands it on, beside a 1 for the one that blew
  # up: the dispatcher reports 1 whatever the filter says, because a filter that read only some of
  # the packages proves nothing about the rest, but the findings it did read still reach stdout.
  add_reports eslint ${reports[@]+"${reports[@]}"}

  return "$status"
}
