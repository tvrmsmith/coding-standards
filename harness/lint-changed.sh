#!/usr/bin/env bash
#
# Lint the changed files, in every language this repository is wired up for.
#
#   lint-changed.sh --staged                # what a commit would contain (the pre-commit hook)
#   lint-changed.sh --since main            # everything changed against a ref
#   lint-changed.sh --files a.ts b.go       # an explicit list, languages may be mixed
#   lint-changed.sh --only go --since main
#
# One entrypoint, because which languages apply is the tool's question and not the caller's. Each
# branch already self-gates — TypeScript skips a package with no ESLint config or no installed
# binary, Go skips a repo absent from its registry, C# skips a repo no props file scopes — so a
# caller naming the branches by hand is re-deriving what this script already knows. Running all of
# them and letting each decide costs nothing when it does not apply.
#
# Changed-files-only is not an optimisation, it is the thing that makes adoption possible: any
# mature repo surfaces a flood of pre-existing violations if whole packages are linted, and a
# flood is indistinguishable from noise.
#
# One exit convention, everywhere, the one ADR 0010 sets: **2 is "the gate says stop", 1 is "the
# gate broke"**. A surviving finding returns 2 — an ESLint error, a C# analyzer warning on a line
# the change wrote. Anything that stopped the gate from answering returns 1 — a failed build, a
# golangci-lint run that blew up, a missing layering wrapper, a bad argument. Personal Go findings
# are advisory and return 0.
#
# So the aggregate is not plain highest-wins: a 1 from any branch dominates a 2 from another. A
# filter that did not run proves nothing, and a surviving finding reported next to a broken branch
# would say the gate answered when half of it never did.
#
# --only runs a single branch. That is for no-mistakes `lint.extra_linters`, which wants one entry
# per language so each gets its own identity, finding ids, budget and exit-code isolation.
#
# Written for bash 3.2 (the macOS system bash).
set -uo pipefail

# Every branch this script knows, in the order they report. The order is fixed rather than
# meaningful: all three run whatever the earlier ones found, so a mixed commit reports every
# language, and pinning the sequence is what keeps two runs over the same change byte-identical.
LANGUAGES="ts dotnet go"

# The header down to the first line that is not a comment. A hardcoded last line drifts on the
# next edit, and printed source as help is worse than no help.
usage() {
  sed -n '3,${/^#/!q;p;}' "${BASH_SOURCE[0]}" | sed 's/^#\{1,2\} \{0,1\}//'
}

# -P: invoked through the ~/.config/coding-standards symlink, an unresolved path would look for
# the branches in ~/.config.
linters=$(cd -P -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/linters

# A branch that failed to source would leave `<lang>_owns` undefined, which returns 127 for every
# file, drops the language, and exits 0 — a partial install reading as a clean gate.
. "$linters/common.sh" || exit 1
for lang in $LANGUAGES; do . "$linters/$lang.sh" || exit 1; done

parse_args "$@"

if [ -n "$only" ]; then
  # Compared token by token. A substring match over " $LANGUAGES " accepts `--only "ts dotnet"`,
  # which then equals no single language in the loop below and lints nothing at all.
  match=0
  for lang in $LANGUAGES; do [ "$only" = "$lang" ] && match=1; done
  [ $match -eq 1 ] || { echo "lint-changed: --only takes one of: $LANGUAGES" >&2; exit 1; }
fi

resolve_repo

# Written to a file rather than read straight off a process substitution, whose exit status bash
# discards and `pipefail` does not reach. A bad --since ref would otherwise yield an empty changed
# set and exit 0, so a caller reading 0 as clean sees a broken invocation as a pass.
changed_list=$scratch/changed
changed_paths >"$changed_list" || exit 1

# NUL-delimited, since `$(...)` strips the NUL bytes that keep a filename holding a space in one
# piece.
changed=()
while IFS= read -r -d '' file; do
  [ -n "$file" ] && changed+=("$file")
done <"$changed_list"

status=0
for lang in $LANGUAGES; do
  [ -z "$only" ] || [ "$only" = "$lang" ] || continue

  owned=()
  for file in ${changed[@]+"${changed[@]}"}; do
    # Absent paths are passed on rather than filtered here. A path in the change but not on disk
    # is a question for the branch, not for the dispatcher: the C# branch has to stop the commit
    # over one, because a staged file MSBuild never compiled would otherwise pass unexamined.
    "${lang}_owns" "$file" && owned+=("$file")
  done

  [ ${#owned[@]} -gt 0 ] || continue

  # Ranked, not merely zero and non-zero, and not whichever branch happened to run last. The rule
  # lives in common.sh, next to the branch contract that states it. Anything that is neither 0 nor
  # 2 is a branch breaking, including a 127 from a function that was never defined, so it reports
  # as 1 rather than as some richer code the hook cannot read.
  "${lang}_lint" "${owned[@]}"
  branch_status=$?
  status=$(rank_status "$status" "$branch_status")
done

exit $status
