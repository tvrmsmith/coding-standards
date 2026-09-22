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
# What it returns is the one convention all three share, ADR 0010's: 2 when a finding survived and
# the gate says stop, 1 when the branch itself could not answer, 0 otherwise. `rank_status` below
# is that convention as code, for every place two of these codes have to be folded into one.
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
      # 1, not 2: nothing was linted, so this is the gate breaking rather than a finding.
      *) echo "lint-changed: unknown argument '$1'" >&2; exit 1 ;;
    esac
  done
}

resolve_repo() {
  repo_root=$(git rev-parse --show-toplevel 2>/dev/null) || {
    echo "lint-changed: not inside a git repository" >&2
    exit 1
  }
  # The .NET analyzers are scoped by a StartsWith condition on the *resolved* project directory,
  # so every path derived here has to be resolved too or nothing matches.
  repo_root=$(cd "$repo_root" && pwd -P)
  cd "$repo_root" || exit 1

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
    # No fallback to the unresolved value. It would match neither the props condition nor a Go
    # registry line, so both branches would take not_wired and a typo in the variable would turn
    # the gate into a silent no-op. A caller that names a checkout is asserting one exists.
    registry_key=$(cd "$TVRMSMITH_REGISTRY_KEY" 2>/dev/null && pwd -P) || {
      echo "lint-changed: TVRMSMITH_REGISTRY_KEY names '$TVRMSMITH_REGISTRY_KEY', which is not a directory" >&2
      exit 1
    }
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
    # The guard is bash 3.2's: expanding an empty array under `set -u` is an error. It must not
    # become the function's status, which the caller checks.
    --files) [ ${#explicit_files[@]} -eq 0 ] || printf '%s\0' "${explicit_files[@]}" ;;
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

# Folds a child status into a running one and echoes the result. 0 changes nothing, 2 takes hold
# only while nothing has broken, and anything else is the gate breaking and sticks: a branch that
# could not run says nothing about the code it never read, so its 1 outranks another's finding.
# Echoed rather than assigned, so no caller's variable is written through dynamic scope.
rank_status() {
  case $2 in
    0) echo "$1" ;;
    2) if [ "$1" -eq 0 ]; then echo 2; else echo "$1"; fi ;;
    *) echo 1 ;;
  esac
}

# The blocking half every branch hands its reports to. `lint-changed` is language-neutral: it
# reads a linter's own report, keeps the findings touching a changed line, applies any waiver and
# sets the exit status. This echoes the binary's path, or returns 1 with the reason on stderr.
#
# Built here rather than bootstrapped, because Go's build cache makes a rebuild of an unchanged
# tree cost milliseconds against a dotnet build's seconds, and building every time is one less
# thing that can go stale. It has to be a built binary rather than `go run`: lint-changed reads
# the git repo it is *run in*, and `go run` would have to run in the hub's module directory.
#
# The memo is a file and not a variable because every caller reads the path through `$(...)`,
# which runs the function in a subshell, so an assignment it made would die with that subshell and
# all three branches would rebuild. $scratch is per-process and removed at exit, which is exactly
# the lifetime the memo wants.
lint_changed_bin() {
  local cache_home bin build_out memo=$scratch/lint-changed-path

  if [ -s "$memo" ]; then
    cat "$memo"
    return 0
  fi

  command -v go >/dev/null 2>&1 || {
    echo "lint-changed: no go on PATH, so the changed-line filter cannot run" >&2
    echo "  Findings would go unchecked and the commit would pass unexamined, so this is a failure" >&2
    echo "  rather than a skip. Install Go, or unstage the changes." >&2
    return 1
  }

  cache_home=${XDG_CACHE_HOME:-$HOME/.cache}
  bin=$cache_home/coding-standards/lint-changed
  mkdir -p "$(dirname "$bin")" || return 1
  if ! build_out=$(cd "$hub" && go build -o "$bin" ./lint/cmd/lint-changed 2>&1); then
    echo "lint-changed: could not build the changed-line filter" >&2
    printf '%s\n' "$build_out" >&2
    return 1
  fi

  # Only a success is remembered. A memo written on a failed build would hand the next branch a
  # path to a binary that is missing or stale, and it would run it rather than see the message.
  printf '%s\n' "$bin" >"$memo"
  printf '%s\n' "$bin"
}

# The tail every branch ends on: one `lint-changed` process over every report that branch's run
# produced, whose exit status is the branch's verdict. One process and not one per report, because
# a waiver is one commit's worth of permission, and a process per report spends it on whatever
# matched its own report without knowing another report still blocks the commit.
#
# With no report at all a --staged run still goes through, on an empty document in the branch's
# own format. ADR 0010's staged-versus-disk hard stop covers every staged path and lives in
# lint-changed, so a branch that returned early because nothing reached a report is how a commit
# whose every staged file was deleted from the working tree went through unexamined. Returning
# early is safe only when there is no such stop to ask for, which is every mode but --staged.
#
# Called as: lint_changed_run <format> <language> <empty-document> <scope-arg>... -- <report>...
# The status is returned rather than echoed, since lint-changed's own stdout is the porcelain the
# caller must not swallow.
lint_changed_run() {
  local format=$1 language=$2 empty=$3 bin report
  shift 3

  local scope=() report_args=()
  while [ $# -gt 0 ]; do
    [ "$1" = "--" ] && { shift; break; }
    scope+=("$1")
    shift
  done
  for report in "$@"; do
    report_args+=(--report "$report")
  done

  if [ ${#report_args[@]} -eq 0 ] && [ "$mode" != "--staged" ]; then
    return 0
  fi

  bin=$(lint_changed_bin) || return 1

  # The ${#...[@]} guards are for bash 3.2, where expanding an empty array under `set -u` is an
  # unbound-variable error rather than an empty expansion.
  if [ ${#report_args[@]} -gt 0 ]; then
    "$bin" --format "$format" --language "$language" \
      ${scope[@]+"${scope[@]}"} "${report_args[@]}" </dev/null
    return $?
  fi
  printf '%s' "$empty" | "$bin" --format "$format" --language "$language" --staged
}

# Skip, don't fail. A repo bootstrapped for one language must not have its commits blocked by a
# branch that was never wired up. Silent under --staged, because the hook runs on every commit.
not_wired() {
  [ "$mode" = "--staged" ] \
    || echo "lint-changed: $registry_key is not wired for $1 — run 'bootstrap $2 $registry_key'" >&2
}
