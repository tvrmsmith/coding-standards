#!/usr/bin/env bash
#
# The TypeScript branch of lint-changed.sh: each changed file through its own package's ESLint,
# plus the personal layer. Sourced, never executed.
#
# This branch follows ADR 0010's exit codes, not yet its blocking rule. An ESLint error returns 2,
# the shared "a finding survived", rather than ESLint's own 1, and the branch's own guards return
# 1: a missing layering wrapper is the gate breaking, and the hook has to be able to tell that
# from a finding it can go and fix.
#
# **Today an ESLint error blocks and a warning does not**, and no changed-line filter runs here.
# ADR 0010 asks for both severities to block on any line the change touched, so this branch is
# short of that, the same gap the Go branch names for slice 3 of issue 108.

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

# A file staged in one state and left in another on disk must be linted as staged, otherwise the
# hook passes on content the commit will not contain. ESLint takes content on stdin, so unlike the
# other two branches this one can honour it rather than warn about it.
_ts_staged_differs() {
  [ "$mode" = "--staged" ] || return 1
  git diff --quiet -- "$1" && return 1
  return 0
}

# ESLint's own codes translated into the shared convention, which rank_status then folds in. 1
# from ESLint is lint errors and becomes 2, a finding that survived. Anything else is ESLint
# failing to run at all — a fatal config error is its 2 — and becomes 1, the gate breaking.
_ts_shared_code() {
  case $1 in
    0) echo 0 ;;
    1) echo 2 ;;
    *) echo 1 ;;
  esac
}

ts_lint() {
  local file pkg rel eslint_dir eslint_bin eslint_status status=0 pairs=() batch=()

  # 1, not 2: nothing was linted, so this is the gate breaking rather than a surviving finding.
  if [ ! -f "$layer" ]; then
    echo "lint-changed: no layering wrapper at $layer" >&2
    return 1
  fi

  for file in "$@"; do
    # Deleted in the change, or named by --files and never there. ESLint can take staged content
    # on stdin, but only for a path that still exists to resolve a config against.
    [ -e "$file" ] || continue
    if pkg=$(ancestor_with "$file" _ts_has_config); then
      pairs+=("$pkg	$file")
    else
      echo "lint-changed: no ESLint config above $file — skipped" >&2
    fi
  done
  [ ${#pairs[@]} -gt 0 ] || return 0

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
      if _ts_staged_differs "$file"; then
        echo "=== $file (staged content) ==="
        git show ":$file" | (
          cd "$pkg" && ESLINT_USE_FLAT_CONFIG=true "$eslint_bin" \
            --config "$layer" --stdin --stdin-filename "$rel"
        )
        eslint_status=$?
        status=$(rank_status "$status" "$(_ts_shared_code "$eslint_status")")
      else
        batch+=("$rel")
      fi
    done <<<"$(printf '%s\n' "${pairs[@]}" | awk -F'\t' -v p="$pkg" '$1 == p { print $2 }')"

    if [ ${#batch[@]} -gt 0 ]; then
      echo "=== $pkg ==="
      ( cd "$pkg" && ESLINT_USE_FLAT_CONFIG=true "$eslint_bin" --config "$layer" "${batch[@]}" </dev/null )
      eslint_status=$?
      status=$(rank_status "$status" "$(_ts_shared_code "$eslint_status")")
    fi
  done <<<"$(printf '%s\n' "${pairs[@]}" | cut -f1 | sort -u)"

  return $status
}
