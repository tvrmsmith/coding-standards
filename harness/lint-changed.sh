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
# gate broke"**. A surviving finding returns 2, an ESLint warning or a C# analyzer warning or a
# golangci-lint issue on a line the change wrote. Anything that stopped the gate from answering
# returns 1, a failed build, a golangci-lint run that blew up, a missing layering wrapper, a bad
# argument. All three branches block; none of them reports advisory findings any more.
#
# The branches do not judge their own findings. Each runs its linter and hands the reports up, and
# one `lint-changed` run over every branch's reports decides what survives, after the last branch.
# One run and not one per branch, because a waiver is one commit's worth of permission: a filter
# per branch spent a waiver on its own language's clean share while another language still blocked
# the commit. The filter never spends. This script does, once, and only on a full --staged run
# that came back 0: that is the pre-commit hook, the one run whose clean result is a commit going
# through. A --since or --files run is not a commit, and an --only run is one language's share of
# one, so neither spends; a waiver they match still passes the finding and stays unspent.
#
# So a branch returns 0 or 1, and only the filter returns 2. The aggregate is not plain
# highest-wins: a 1 from any branch dominates the filter's 2. A branch that could not run proves
# nothing, and a surviving finding reported next to it would say the gate answered when half of it
# never did. The findings the filter did read still reach stdout, and no waiver is spent.
#
# --only runs a single branch. That is for no-mistakes `lint.extra_linters`, which wants one entry
# per language so each gets its own identity, finding ids, budget and exit-code isolation.
#
# Written for bash 3.2 (the macOS system bash).
set -uo pipefail

# Every branch this script knows, in the order they report. The order is fixed rather than
# meaningful: every branch runs whatever the earlier ones found, so a mixed commit reports every
# language, and the filter prints survivors in the order the reports were handed to it, so pinning
# the sequence is what keeps two runs over the same change byte-identical.
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

  # The --files scope the one filter run is given. Only what is on disk: lint-changed refuses a
  # --files path that is not a regular file, and a file absent from disk reached no linter anyway.
  for file in "${owned[@]}"; do
    [ -f "$file" ] && filter_files+=("$file")
  done

  # Ranked, not merely zero and non-zero, and not whichever branch happened to run last. The rule
  # lives in common.sh, next to the branch contract that states it. A branch returns 0 or 1, and
  # anything else is it breaking, including a 127 from a function that was never defined, so it
  # reports as 1 rather than as some richer code the hook cannot read.
  "${lang}_lint" "${owned[@]}"
  branch_status=$?
  status=$(rank_status "$status" "$branch_status")
done

# One filter over every branch's reports, folded the same way, so a broken branch's 1 still
# outranks a finding the filter found.
run_filter
filter_status=$?
status=$(rank_status "$status" "$filter_status")

# Spent here and nowhere else, and only on a clean total of a full --staged run. A waiver is one
# commit's worth of permission, and a commit goes through only when no branch broke and no finding
# survived, so spending on any lesser verdict burns it on a commit git never makes. --since and
# --files make no commit, and --only sees one language of it.
if [ "$status" -eq 0 ] && [ "$mode" = --staged ] && [ -z "$only" ] && [ -s "$matched_waivers" ]; then
  spend_waivers || status=1
fi

exit "$status"
