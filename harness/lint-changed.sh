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
# Exit status is the worst of the branches that ran, and they do not share a convention. ESLint
# errors fail and warnings do not. The C# branch blocks on any analyzer warning touching a line
# the change wrote, 2 for a finding that survived and 1 for the gate itself breaking. Personal Go
# findings are advisory, so a non-zero there means the run broke rather than that it found
# something.
#
# --only runs a single branch. That is for no-mistakes `lint.extra_linters`, which wants one entry
# per language so each gets its own identity, finding ids, budget and exit-code isolation.
#
# Written for bash 3.2 (the macOS system bash).
set -uo pipefail

# Every branch this script knows, in the order they report. TypeScript first because it is the one
# that can block a commit, so its findings should be the last thing scrolled past.
LANGUAGES="ts dotnet go"

usage() {
  sed -n '3,36p' "${BASH_SOURCE[0]}" | sed 's/^#\{1,2\} \{0,1\}//'
}

# -P: invoked through the ~/.config/coding-standards symlink, an unresolved path would look for
# the branches in ~/.config.
linters=$(cd -P -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/linters

. "$linters/common.sh"
for lang in $LANGUAGES; do . "$linters/$lang.sh"; done

parse_args "$@"

if [ -n "$only" ]; then
  case " $LANGUAGES " in
    *" $only "*) ;;
    *) echo "lint-changed: --only takes one of: $LANGUAGES" >&2; exit 2 ;;
  esac
fi

resolve_repo

# Read once into an array, through a process substitution rather than `$(...)`, which strips the
# NUL bytes that keep a filename holding a space in one piece.
changed=()
while IFS= read -r -d '' file; do
  [ -n "$file" ] && changed+=("$file")
done < <(changed_paths)

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

  # Highest wins, rather than whichever branch happened to run last. The codes are ranked, not
  # merely zero and non-zero, so a `status=$?` that overwrites would report the wrong cause for a
  # commit touching two languages: a later branch returning 1 would mask an earlier 2.
  "${lang}_lint" "${owned[@]}"
  branch_status=$?
  [ $branch_status -gt $status ] && status=$branch_status
done

exit $status
