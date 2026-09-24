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
# A branch runs its linter and hands the reports to `add_reports` rather than judging them itself.
# It returns 1 when it could not answer and 0 otherwise, never 2: whether a finding survived is the
# one filter's call, made by the dispatcher over every branch's reports at once. ADR 0010's
# convention, 2 for a finding and 1 for the gate breaking, is `rank_status` below, which folds
# each branch's status and then the filter's into one.
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
#
# --staged during an uncommitted merge is merge_written_paths. Not `git diff --cached HEAD` outside
# a merge, since that fails on an unborn HEAD where the bare form diffs against the empty tree.
changed_paths() {
  case "$mode" in
    --staged)
      if git rev-parse -q --verify MERGE_HEAD >/dev/null; then
        merge_written_paths
      else
        git diff --cached --name-only -z --diff-filter=ACM
      fi
      ;;
    --since) git diff --name-only -z --diff-filter=ACM "$ref" ;;
    # The guard is bash 3.2's: expanding an empty array under `set -u` is an error. It must not
    # become the function's status, which the caller checks.
    --files) [ ${#explicit_files[@]} -eq 0 ] || printf '%s\0' "${explicit_files[@]}" ;;
  esac
}

# The staged paths of an uncommitted merge that differ from both parents: a conflict resolution, or
# an edit made while merging. Against HEAD alone the index holds everything the incoming side
# brought, thousands of files and dozens of .NET builds on a long-lived branch. Against MERGE_HEAD
# alone it holds everything the branch already committed, whose lines all match HEAD, so the filter
# would drop every finding in them after paying for the builds. Only a path in both can carry a
# finding the filter keeps.
#
# One `git diff --quiet` per path in the MERGE_HEAD list rather than intersecting two lists, since
# bash 3.2 has no associative array and macOS comm cannot read NUL-delimited input. :(literal) keeps
# a path holding a glob character from matching more than itself.
merge_written_paths() {
  local list=$scratch/merge-candidates path
  git diff --cached --name-only -z --diff-filter=ACM MERGE_HEAD >"$list" || return 1
  while IFS= read -r -d '' path; do
    git diff --cached --quiet HEAD -- ":(literal)$path"
    case $? in
      0) ;;
      1) printf '%s\0' "$path" ;;
      *) return 1 ;;
    esac
  done <"$list"
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

# The blocking half every branch's reports go to. `lint-changed` is language-neutral: it reads a
# linter's own report, keeps the findings touching a changed line, applies any waiver and sets the
# exit status. This echoes the binary's path, or returns 1 with the reason on stderr.
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

# Every report the branches produced, as `lint-changed` argv, and every changed file they own that
# is on disk. The dispatcher runs one filter over the lot after the last branch, because a waiver
# is one commit's worth of permission, and a filter per branch spent it on whatever matched its
# own language's share without knowing another language still blocked the commit.
filter_reports=()
filter_files=()

# Where the one filter run writes the id of every waiver it matched, one per line. The filter never
# spends: the dispatcher does, and only once the whole commit has come back clean.
matched_waivers=$scratch/matched-waivers

# The tail every branch ends on, in place of running a filter of its own: `--format <format>` and a
# `--report` per path onto filter_reports. The --format goes on even with no report at all, so a
# non-empty filter_reports means at least one branch reached its tail, which is what tells
# run_filter a --staged commit still needs its staged-versus-disk check.
#
# Called as: add_reports <format> [<report>...]
add_reports() {
  local report
  filter_reports+=(--format "$1")
  shift
  for report in "$@"; do
    filter_reports+=(--report "$report")
  done
}

# The one `lint-changed` process over every report every branch handed in. The status is returned
# rather than echoed, since lint-changed's own stdout is the porcelain the caller must not swallow.
#
# No branch reaching its tail runs nothing, the way an unadopted repo's commits stay unblocked.
# With branches that reached it but produced no report, a --staged run still goes through, since
# ADR 0010's staged-versus-disk hard stop covers every staged path and lives in lint-changed: a
# commit whose every staged file was deleted from the working tree reaches no linter, and skipping
# the filter is how it would pass unexamined. Every other mode has no such stop to ask for.
run_filter() {
  local bin arg file scope=() accept_spent=() has_report=0

  [ ${#filter_reports[@]} -gt 0 ] || return 0
  for arg in "${filter_reports[@]}"; do
    [ "$arg" = "--report" ] && has_report=1
  done
  [ $has_report -eq 1 ] || [ "$mode" = "--staged" ] || return 0

  # Built once here rather than per branch, so a new mode or a change to how --files passes its
  # paths cannot land in one branch and leave another scoping against the wrong base. One --files
  # per path, since a comma is legal in a filename.
  case "$mode" in
    --staged) scope=(--staged) ;;
    --since) scope=(--since "$ref") ;;
    --files)
      for file in ${filter_files[@]+"${filter_files[@]}"}; do
        scope+=(--files "$file")
      done
      ;;
  esac

  # A run that spends nothing accepts a waiver spent against any tree. lint-changed.sh decides
  # $spends, next to the spend gate that reads it too.
  [ "$spends" -eq 1 ] || accept_spent=(--accept-spent)

  bin=$(lint_changed_bin) || return 1
  "$bin" ${scope[@]+"${scope[@]}"} --matched-waivers "$matched_waivers" ${accept_spent[@]+"${accept_spent[@]}"} "${filter_reports[@]}" </dev/null
}

# Marks every waiver the filter matched as spent. The dispatcher calls it only on a clean total of a
# full --staged run, so a branch that broke, which git will not commit past, spends nothing.
spend_waivers() {
  local bin id waivers=()
  while IFS= read -r id; do
    [ -n "$id" ] && waivers+=(--waiver "$id")
  done <"$matched_waivers"
  [ ${#waivers[@]} -gt 0 ] || return 0

  bin=$(lint_changed_bin) || return 1
  "$bin" spend "${waivers[@]}" </dev/null
}

# Skip, don't fail. A repo bootstrapped for one language must not have its commits blocked by a
# branch that was never wired up. Silent under --staged, because the hook runs on every commit.
not_wired() {
  [ "$mode" = "--staged" ] \
    || echo "lint-changed: $registry_key is not wired for $1 — run 'bootstrap $2 $registry_key'" >&2
}
