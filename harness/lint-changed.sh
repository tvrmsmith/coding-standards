#!/usr/bin/env bash
#
# Lint changed files only, each through its own package's ESLint plus the personal layer.
#
#   lint-changed.sh --staged            # what a commit would contain (the pre-commit hook)
#   lint-changed.sh --since main        # everything changed against a ref
#   lint-changed.sh --files a.ts b.tsx  # an explicit list
#
# Changed-files-only is not an optimisation, it is the thing that makes adoption possible:
# any mature repo surfaces a flood of pre-existing violations if whole packages are linted,
# and a flood is indistinguishable from noise.
#
# Exit status is ESLint's: **errors fail, warnings do not**. Deliberate — the custom rule
# and all nine "you might not need an Effect" rules ship at warn precisely because they
# propose restructures, and they land on legacy code in batches. A warn that blocks a
# commit is an error wearing a disguise.
#
# Written for bash 3.2 (the macOS system bash).
set -uo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
layer=${TVRMSMITH_ESLINT_LAYER:-$script_dir/eslint-layer.js}

mode=--staged
ref=
explicit_files=()

while [ $# -gt 0 ]; do
  case "$1" in
    --staged) mode=--staged; shift ;;
    --since) mode=--since; ref=${2:?--since needs a ref}; shift 2 ;;
    --files) mode=--files; shift; while [ $# -gt 0 ]; do explicit_files+=("$1"); shift; done ;;
    -h|--help) sed -n '2,20p' "${BASH_SOURCE[0]}"; exit 0 ;;
    *) echo "lint-changed: unknown argument '$1'" >&2; exit 2 ;;
  esac
done

if [ ! -f "$layer" ]; then
  echo "lint-changed: no layering wrapper at $layer" >&2
  exit 2
fi

repo_root=$(git rev-parse --show-toplevel 2>/dev/null) || {
  echo "lint-changed: not inside a git repository" >&2
  exit 2
}
cd "$repo_root" || exit 2

lintable() {
  case "$1" in
    *.js|*.jsx|*.ts|*.tsx|*.mjs|*.cjs|*.mts|*.cts) return 0 ;;
    *) return 1 ;;
  esac
}

case "$mode" in
  --staged) changed=$(git diff --cached --name-only --diff-filter=ACM) ;;
  --since) changed=$(git diff --name-only --diff-filter=ACM "$ref") ;;
  --files) changed=$(printf '%s\n' "${explicit_files[@]}") ;;
esac

files=()
while IFS= read -r file; do
  [ -n "$file" ] || continue
  lintable "$file" && files+=("$file")
done <<<"$changed"

[ ${#files[@]} -gt 0 ] || exit 0

# The package a file belongs to: nearest ancestor holding an ESLint config, since that is
# the directory ESLint has to run from — flat config does not cascade, and the wrapper
# loads the package's config from the process cwd.
package_of() {
  local dir
  dir=$(dirname "$1")
  while :; do
    for name in eslint.config.js eslint.config.mjs eslint.config.cjs eslint.config.ts \
                .eslintrc.js .eslintrc.cjs .eslintrc.mjs .eslintrc.json .eslintrc; do
      [ -f "$dir/$name" ] && { echo "$dir"; return 0; }
    done
    [ "$dir" = "." ] || [ "$dir" = "/" ] && return 1
    dir=$(dirname "$dir")
  done
}

# ESLint itself always comes from the repo, never from the harness: the package pins the
# version its config was written for, ESLint 8 or 9, and its
# plugins resolve relative to it. Nothing is installed on demand — a package whose
# dependencies are not installed is skipped, loudly.
eslint_bin_for() {
  local dir=$1
  while :; do
    [ -x "$dir/node_modules/.bin/eslint" ] && { echo "$dir/node_modules/.bin/eslint"; return 0; }
    [ "$dir" = "." ] || [ "$dir" = "/" ] && return 1
    dir=$(dirname "$dir")
  done
}

pairs=()
for file in "${files[@]}"; do
  [ -e "$file" ] || continue
  if pkg=$(package_of "$file"); then
    pairs+=("$pkg	$file")
  else
    echo "lint-changed: no ESLint config above $file — skipped" >&2
  fi
done

[ ${#pairs[@]} -gt 0 ] || exit 0

# A file staged in one state and left in another on disk must be linted as staged,
# otherwise the hook passes on content the commit will not contain.
staged_differs_from_worktree() {
  [ "$mode" = "--staged" ] || return 1
  git diff --quiet -- "$1" && return 1
  return 0
}

status=0
for pkg in $(printf '%s\n' "${pairs[@]}" | cut -f1 | sort -u); do
  eslint_bin=$(eslint_bin_for "$pkg") || {
    echo "lint-changed: $pkg has no installed eslint — skipped (run its package manager install)" >&2
    continue
  }
  eslint_bin=$repo_root/$eslint_bin

  batch=()
  for file in $(printf '%s\n' "${pairs[@]}" | awk -F'\t' -v p="$pkg" '$1 == p { print $2 }'); do
    rel=${file#"$pkg"/}
    if staged_differs_from_worktree "$file"; then
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

exit $status
