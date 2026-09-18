#!/usr/bin/env bash
#
# The TypeScript branch of lint-changed.sh: each changed file through its own package's ESLint,
# plus the personal layer. Sourced, never executed.
#
# Exit status is ESLint's: **errors fail, warnings do not**. Deliberate — the custom rule and all
# nine "you might not need an Effect" rules ship at warn precisely because they propose
# restructures, and they land on legacy code in batches. A warn that blocks a commit is an error
# wearing a disguise.

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
_ts_eslint_bin() {
  local dir=$1
  while :; do
    [ -x "$dir/node_modules/.bin/eslint" ] && { echo "$dir/node_modules/.bin/eslint"; return 0; }
    [ "$dir" = "." ] || [ "$dir" = "/" ] && return 1
    dir=$(dirname "$dir")
  done
}

# A file staged in one state and left in another on disk must be linted as staged, otherwise the
# hook passes on content the commit will not contain. ESLint takes content on stdin, so unlike the
# other two branches this one can honour it rather than warn about it.
_ts_staged_differs() {
  [ "$mode" = "--staged" ] || return 1
  git diff --quiet -- "$1" && return 1
  return 0
}

ts_lint() {
  local file pkg rel eslint_bin status=0 pairs=() batch=()

  if [ ! -f "$layer" ]; then
    echo "lint-changed: no layering wrapper at $layer" >&2
    return 2
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

  for pkg in $(printf '%s\n' "${pairs[@]}" | cut -f1 | sort -u); do
    eslint_bin=$(_ts_eslint_bin "$pkg") || {
      echo "lint-changed: $pkg has no installed eslint — skipped (run its package manager install)" >&2
      continue
    }
    eslint_bin=$repo_root/$eslint_bin

    batch=()
    for file in $(printf '%s\n' "${pairs[@]}" | awk -F'\t' -v p="$pkg" '$1 == p { print $2 }'); do
      rel=${file#"$pkg"/}
      if _ts_staged_differs "$file"; then
        echo "=== $file (staged content) ==="
        git show ":$file" | (
          cd "$pkg" && ESLINT_USE_FLAT_CONFIG=true "$eslint_bin" \
            --config "$layer" --stdin --stdin-filename "$rel"
        ) || status=1
      else
        batch+=("$rel")
      fi
    done

    if [ ${#batch[@]} -gt 0 ]; then
      echo "=== $pkg ==="
      ( cd "$pkg" && ESLINT_USE_FLAT_CONFIG=true "$eslint_bin" --config "$layer" "${batch[@]}" ) || status=1
    fi
  done

  return $status
}
