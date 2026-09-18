#!/usr/bin/env bash
#
# Shared by every language branch of lint-changed.sh. Sourced, never executed.
#
# What a branch may rely on after the entrypoint has set up:
#
#   repo_root     the checkout, resolved through symlinks
#   registry_key  the path adoption is looked up under, which is not always $repo_root
#   mode          --staged, --since or --files
#   hub           the coding-standards checkout, for the binaries and configs it builds
#   scratch       a directory that outlives the branch and is removed at exit
#
# A branch is two functions, `<lang>_owns` and `<lang>_lint`. See lint-changed.sh for the
# contract. The one rule easy to get wrong: a branch **returns**, it never exits. Three branches
# share this process now, so an `exit 0` meaning "nothing here for me" would cancel the languages
# that had not run yet.
#
# Written for bash 3.2 (the macOS system bash).

# -P: invoked through the ~/.config/coding-standards symlink, an unresolved path would send every
# $hub-relative lookup into ~/.config.
harness=$(cd -P -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
hub=$(cd -P -- "$harness/.." && pwd)

# One scratch directory and one trap, rather than a mktemp and a trap per branch: a second
# `trap ... EXIT` replaces the first rather than adding to it, so the branch that registered
# earliest would leak its file.
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT

mode=--staged
ref=
only=
explicit_files=()

# --files takes every remaining argument, so it has to come last. Saying so is the whole reason
# this is not just a getopts loop.
parse_args() {
  while [ $# -gt 0 ]; do
    case "$1" in
      --staged) mode=--staged; shift ;;
      --since) mode=--since; ref=${2:?--since needs a ref}; shift 2 ;;
      --files) mode=--files; shift; while [ $# -gt 0 ]; do explicit_files+=("$1"); shift; done ;;
      --only) only=${2:?--only needs a language}; shift 2 ;;
      -h|--help) usage; exit 0 ;;
      *) echo "lint-changed: unknown argument '$1'" >&2; exit 2 ;;
    esac
  done
}

resolve_repo() {
  repo_root=$(git rev-parse --show-toplevel 2>/dev/null) || {
    echo "lint-changed: not inside a git repository" >&2
    exit 2
  }
  # The .NET analyzers are scoped by a StartsWith condition on the *resolved* project directory,
  # so every path derived here has to be resolved too or nothing matches.
  repo_root=$(cd "$repo_root" && pwd -P)
  cd "$repo_root" || exit 2

  # A linked worktree is the same adoption as the checkout it was made from, so adoption is keyed
  # on the main checkout rather than on where the commit happens to be taken. The hook is shared
  # anyway — it lives in the common git dir — so keying on $repo_root would install a hook in
  # every worktree that then skipped, which reads exactly like the layer being broken. The common
  # git dir is <main>/.git in both cases, so its parent is the main checkout.
  #
  # TVRMSMITH_REGISTRY_KEY names the checkout directly, for a caller that already knows it and
  # whose worktree cannot say. A no-mistakes run is that caller: it lints in a detached worktree
  # of the daemon's own bare gate repo, so the common git dir resolves to the gate, the gate is in
  # no registry, and the skip is indistinguishable from a clean result.
  registry_key=$repo_root
  if [ -n "${TVRMSMITH_REGISTRY_KEY:-}" ]; then
    registry_key=$(cd "$TVRMSMITH_REGISTRY_KEY" 2>/dev/null && pwd -P) \
      || registry_key=$TVRMSMITH_REGISTRY_KEY
  elif common_git_dir=$(cd "$(git rev-parse --git-common-dir)" 2>/dev/null && pwd -P); then
    registry_key=$(cd "$common_git_dir/.." && pwd -P)
  fi
}

# NUL-delimited throughout. git quotes a path holding a space or a non-ASCII byte in its
# newline-delimited output, and `read -r` splits the quoted form on whitespace, so a newline list
# drops exactly the filenames a developer is most likely to get wrong. Callers read this with
# `read -r -d ''` through a process substitution, never `$(...)`, which strips NUL bytes.
changed_paths() {
  case "$mode" in
    --staged) git diff --cached --name-only -z --diff-filter=ACM ;;
    --since) git diff --name-only -z --diff-filter=ACM "$ref" ;;
    --files) [ ${#explicit_files[@]} -gt 0 ] && printf '%s\0' "${explicit_files[@]}" ;;
  esac
}

# The nearest ancestor of $1 that satisfies the predicate named in $2, which is called with a
# directory and answers by exit status. Every language asks this question — which package, which
# module, which project owns this file — and only the predicate differs.
ancestor_with() {
  local dir predicate=$2
  dir=$(dirname "$1")
  while :; do
    "$predicate" "$dir" && { echo "$dir"; return 0; }
    [ "$dir" = "." ] || [ "$dir" = "/" ] && return 1
    dir=$(dirname "$dir")
  done
}

# Skip, don't fail. A repo bootstrapped for one language must not have its commits blocked by a
# branch that was never wired up. Silent under --staged, because the hook runs on every commit.
not_wired() {
  [ "$mode" = "--staged" ] \
    || echo "lint-changed: $registry_key is not wired for $1 — run 'bootstrap $2 $registry_key'" >&2
}

# Linters that read the disk cannot honour a file staged in one state and left in another the way
# ESLint can, because ESLint takes content on stdin and a compiler does not. Say so rather than
# let it pass silently.
warn_divergent() {
  local what=$1 file divergent=()
  shift
  [ "$mode" = "--staged" ] || return 0
  for file in "$@"; do
    git diff --quiet -- "$file" || divergent+=("$file")
  done
  [ ${#divergent[@]} -gt 0 ] || return 0
  echo "lint-changed: these are staged in one state and on disk in another; the $what sees the disk copy:" >&2
  printf '    %s\n' "${divergent[@]}" >&2
}

# The human half of a branch's output. The caller has already deduped and sorted, because the sort
# key belongs to whatever format its linter emits; everything downstream of that is the same
# report. Paths go relative so the reader sees the repo, not the machine.
report_findings() {
  local file=$1 label=$2 note=$3 count
  count=$(wc -l <"$file" | tr -d ' ')
  echo
  echo "personal coding standards — $count finding(s) in the changed $label:"
  sed 's|^'"$repo_root"'/||; s/^/  /' "$file"
  echo
  echo "  $note"
}
