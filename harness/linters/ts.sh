#!/usr/bin/env bash
#
# The TypeScript branch of lint-changed.sh: each changed file through its own package's ESLint,
# plus the personal layer. Sourced, never executed.
#
# ESLint runs per package, since flat config does not cascade and the wrapper loads the package's
# own config from the process cwd. So this lints the packages owning the changed files and hands
# their JSON reports to `lint-changed`, which keeps the findings touching a line the change
# actually wrote and drops the rest. Severity 1 and severity 2 are alike to it, per ADR 0010.
#
# This blocks, on ADR 0010's convention: 2 when a finding survived that filter, 1 when this branch
# could not answer at all — a missing layering wrapper, an ESLint run that blew up, a filter that
# would not build. Past a genuine false positive there is one route, and `lint-changed` prints the
# exact command for it.

layer=${TVRMSMITH_ESLINT_LAYER:-$harness/eslint-layer.js}

ts_owns() {
  case "$1" in
    *.js|*.jsx|*.ts|*.tsx|*.mjs|*.cjs|*.mts|*.cts) return 0 ;;
    *) return 1 ;;
  esac
}

# The package a file belongs to: nearest ancestor holding an ESLint config, since that is the
# directory ESLint has to run from — flat config does not cascade, and the wrapper loads the
# package's config from the process cwd.
_ts_has_config() {
  local name
  for name in eslint.config.js eslint.config.mjs eslint.config.cjs eslint.config.ts \
              .eslintrc.js .eslintrc.cjs .eslintrc.mjs .eslintrc.json .eslintrc; do
    [ -f "$1/$name" ] && return 0
  done
  return 1
}

# ESLint itself always comes from the repo, never from the harness: the package pins the version
# its config was written for, ESLint 8 or 9, and its plugins resolve relative to it. Nothing is
# installed on demand — a package whose dependencies are not installed is skipped, loudly.
_ts_has_eslint_bin() { [ -x "$1/node_modules/.bin/eslint" ]; }

ts_lint() {
  local file pkg rel eslint_dir eslint_bin status=0 present=() pairs=() batch=()
  local report reports=() report_args=() scope_args=() lint_changed filter_status
  local report_dir=$scratch/ts-reports package_count=0

  # 1, not 2: nothing was linted, so this is the gate breaking rather than a surviving finding.
  if [ ! -f "$layer" ]; then
    echo "lint-changed: no layering wrapper at $layer" >&2
    return 1
  fi

  # Deleted in the change, or named by --files and never there. ESLint reads the disk, so there is
  # nothing here for it to lint. The staged-versus-disk question a missing file raises is
  # lint-changed's, and it asks it across every staged path under --staged rather than only the
  # ones that reached a report.
  for file in "$@"; do
    [ -e "$file" ] && present+=("$file")
  done
  [ ${#present[@]} -gt 0 ] || return 0

  for file in "${present[@]}"; do
    if pkg=$(ancestor_with "$file" _ts_has_config); then
      pairs+=("$pkg	$file")
    else
      echo "lint-changed: no ESLint config above $file — skipped" >&2
    fi
  done
  [ ${#pairs[@]} -gt 0 ] || return 0

  lint_changed=$(lint_changed_bin) || return 1
  mkdir -p "$report_dir" || return 1

  case "$mode" in
    --staged) scope_args=(--staged) ;;
    --since)  scope_args=(--since "$ref") ;;
    --files)  scope_args=(--files "$(IFS=,; echo "${present[*]}")") ;;
  esac

  # Read line by line rather than word-split: an unquoted `$(...)` splits a package or file path on
  # every space it holds and then globs each piece, so ESLint would be handed two arguments naming
  # nothing and fail the commit over a file that does not exist.
  while IFS= read -r pkg; do
    [ -n "$pkg" ] || continue
    # ancestor_with starts at the parent of the path it is given, so it takes a path *inside* the
    # package: the package's own node_modules is the first place to look.
    eslint_dir=$(ancestor_with "$pkg/." _ts_has_eslint_bin) || {
      echo "lint-changed: $pkg has no installed eslint — skipped (run its package manager install)" >&2
      continue
    }
    eslint_bin=$repo_root/$eslint_dir/node_modules/.bin/eslint

    batch=()
    while IFS= read -r file; do
      [ -n "$file" ] || continue
      rel=${file#"$pkg"/}
      batch+=("$rel")
    done <<<"$(printf '%s\n' "${pairs[@]}" | awk -F'\t' -v p="$pkg" '$1 == p { print $2 }')"
    [ ${#batch[@]} -gt 0 ] || continue

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
      0|1) reports+=("$report") ;;
      *) status=1; echo "=== $pkg — the lint run failed ===" ;;
    esac
  done <<<"$(printf '%s\n' "${pairs[@]}" | cut -f1 | sort -u)"

  # Every package's report goes into one lint-changed run, named with --report. A process per
  # report was dotnet.sh's first shape and it was wrong: a waiver is one commit's worth of
  # permission, and each process spent whatever matched its own report without knowing another
  # report still blocked the commit, so the waiver was burnt on a commit that never went through.
  #
  # A broken run is reported as 1 whatever the filter said, because a filter that read only some
  # of the packages proves nothing about the one that blew up. rank_status is that rule.
  #
  # The ${#reports[@]} guard is for bash 3.2, where expanding an empty array under `set -u` is an
  # unbound-variable error rather than an empty expansion.
  if [ ${#reports[@]} -gt 0 ]; then
    for report in "${reports[@]}"; do
      report_args+=(--report "$report")
    done
    "$lint_changed" --format eslint --language ts "${scope_args[@]}" "${report_args[@]}" </dev/null
    filter_status=$?
    status=$(rank_status "$status" "$filter_status")
  fi

  return "$status"
}
