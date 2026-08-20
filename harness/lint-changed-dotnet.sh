#!/usr/bin/env bash
#
# Report personal analyzer diagnostics on changed C# files only.
#
#   lint-changed-dotnet.sh --staged            # what a commit would contain (the pre-commit hook)
#   lint-changed-dotnet.sh --since main        # everything changed against a ref
#   lint-changed-dotnet.sh --files a.cs b.cs   # an explicit list
#
# The C# counterpart to lint-changed.sh, and it works differently in one important way:
# Roslyn analyzers only run as part of a compilation, so there is no way to lint a file on its
# own. This builds the project that owns each changed file and filters the diagnostics down to
# the changed files. Changed-files-only is the same non-negotiable as on the TypeScript side —
# a single project build on a mature codebase reports dozens of warnings, nearly all of them in
# code nobody is being asked to touch.
#
# Exit status is dotnet build's: **compile errors fail, analyzer warnings do not**. The same
# convention lint-changed.sh uses, and here it is not merely consistent but forced — every id
# this harness injects is a warning by design — no previously-succeeding build may start
# failing — so blocking on them would mean overriding the build's own verdict. Doing that fairly
# would need per-changed-*line* scoping; without it, a legacy file you touch one line in carries
# findings on lines you never wrote. So: report loudly, do not block.
#
# Written for bash 3.2 (the macOS system bash).
set -uo pipefail

config_home=${XDG_CONFIG_HOME:-$HOME/.config}
props=${TVRMSMITH_ANALYZER_PROPS:-$config_home/coding-standards.props}

mode=--staged
ref=
explicit_files=()

while [ $# -gt 0 ]; do
  case "$1" in
    --staged) mode=--staged; shift ;;
    --since) mode=--since; ref=${2:?--since needs a ref}; shift 2 ;;
    --files) mode=--files; shift; while [ $# -gt 0 ]; do explicit_files+=("$1"); shift; done ;;
    -h|--help) sed -n '2,24p' "${BASH_SOURCE[0]}"; exit 0 ;;
    *) echo "lint-changed-dotnet: unknown argument '$1'" >&2; exit 2 ;;
  esac
done

command -v dotnet >/dev/null 2>&1 || {
  echo "lint-changed-dotnet: no dotnet on PATH — skipped" >&2
  exit 0
}

repo_root=$(git rev-parse --show-toplevel 2>/dev/null) || {
  echo "lint-changed-dotnet: not inside a git repository" >&2
  exit 2
}
# The analyzers are scoped by a StartsWith condition on the *resolved* project directory, so
# every path this script derives has to be resolved too or nothing matches (research caveat 2).
repo_root=$(cd "$repo_root" && pwd -P)
cd "$repo_root" || exit 2

# Whether this repo is adopted is a question the props file already answers: it carries one
# path-scoped Import per adopted repo, so the scoping condition doubles as the registry. That
# keeps the pre-commit hook free of per-language state — one template serves both branches, and
# each decides for itself whether it applies here.
#
# Skip, don't fail. A repo bootstrapped for TypeScript only must not have its commits blocked
# by a .NET branch that was never wired up.
if [ ! -f "$props" ] || ! grep -qF "StartsWith('$repo_root/')" "$props"; then
  [ "$mode" = "--staged" ] \
    || echo "lint-changed-dotnet: $repo_root is not wired for .NET — run 'bootstrap dotnet $repo_root'" >&2
  exit 0
fi

case "$mode" in
  --staged) changed=$(git diff --cached --name-only --diff-filter=ACM) ;;
  --since) changed=$(git diff --name-only --diff-filter=ACM "$ref") ;;
  --files) changed=$(printf '%s\n' "${explicit_files[@]}") ;;
esac

files=()
while IFS= read -r file; do
  [ -n "$file" ] || continue
  case "$file" in
    *.cs) ;;
    *) continue ;;
  esac
  # Generated output is never worth reporting on, and obj/ is full of .cs the SDK wrote.
  case "$file" in
    */obj/*|obj/*|*/bin/*|bin/*) continue ;;
  esac
  [ -e "$file" ] && files+=("$file")
done <<<"$changed"

[ ${#files[@]} -gt 0 ] || exit 0

# A staged file whose worktree copy differs cannot be honoured here the way lint-changed.sh
# honours it: ESLint takes content on stdin, MSBuild compiles what is on disk. Building a
# rewritten copy of the tree to fix that would cost more than it buys at warning severity.
# Say so rather than let it pass silently.
if [ "$mode" = "--staged" ]; then
  divergent=()
  for file in "${files[@]}"; do
    git diff --quiet -- "$file" || divergent+=("$file")
  done
  if [ ${#divergent[@]} -gt 0 ]; then
    echo "lint-changed-dotnet: these are staged in one state and on disk in another; the compiler sees the disk copy:" >&2
    printf '    %s\n' "${divergent[@]}" >&2
  fi
fi

# The project that owns a file: nearest ancestor holding a .csproj. That is also the directory
# MSBuild treats as the project root, so every .cs below it is in the compilation by default.
project_of() {
  local dir found
  dir=$(dirname "$1")
  while :; do
    found=$(ls "$dir"/*.csproj 2>/dev/null | head -1)
    [ -n "$found" ] && { echo "$found"; return 0; }
    [ "$dir" = "." ] || [ "$dir" = "/" ] && return 1
    dir=$(dirname "$dir")
  done
}

pairs=()
for file in "${files[@]}"; do
  if proj=$(project_of "$file"); then
    pairs+=("$proj	$repo_root/$file")
  else
    echo "lint-changed-dotnet: no .csproj above $file — skipped" >&2
  fi
done

[ ${#pairs[@]} -gt 0 ] || exit 0

projects=$(printf '%s\n' "${pairs[@]}" | cut -f1 | sort -u)
project_count=$(printf '%s\n' "$projects" | grep -c .)
[ "$project_count" -gt 4 ] && echo "lint-changed-dotnet: $project_count projects to build; this will take a moment" >&2

status=0
findings=$(mktemp)
trap 'rm -f "$findings"' EXIT

for proj in $projects; do
  # Two passes, and both are needed for different reasons.
  #
  # Pass 1 is an ordinary incremental build, project references included. Its only job is to
  # make the dependencies real. Skipping it and going straight to pass 2 was the first design
  # and it was wrong: on a tree whose referenced projects have not been built, reusing what is
  # on disk means reusing nothing, and the compiler reports the whole file as undefined types.
  # That failure is common rather than exotic: most unbuilt trees produce CS0246/CS0234 that
  # way, none of which exist. A hook that blocks a commit on an error the codebase does not
  # have is worse than no hook.
  #
  # Scoping off in both passes. The build-time scoping is keyed on the
  # working tree against HEAD; this hook is keyed on what is *staged*, and the two sets are not
  # the same one. Letting both filter would make the reported set the intersection, silently.
  # This script's own filter, below, stays the single answer to "which files count".
  out=$(CustomAfterMicrosoftCommonProps="$props" \
    dotnet build "$proj" -p:TvrmsmithAnalyzersEnabled=true \
      -p:TvrmsmithAnalyzersScopeToChanged=false -v:m --nologo 2>&1)
  build_status=$?

  # A compile error is the build's verdict and it stands — that is the whole gate here.
  if [ $build_status -ne 0 ]; then
    status=$build_status
    echo "=== $proj — build failed ==="
    grep -E ': (error|warning) [A-Z]+[0-9]+' <<<"$out" | sort -u
    continue
  fi

  # Pass 2 forces the diagnostics out. Analyzers only run when csc runs, and pass 1 has just
  # left the project up to date, so on its own it reports nothing at all: zero diagnostics
  # incremental against eight forced on the same tree. --no-incremental makes csc run again;
  # BuildProjectReferences=false stops that force from cascading through the graph, which is
  # safe here and only here, because pass 1 has already put the referenced assemblies on disk.
  out=$(CustomAfterMicrosoftCommonProps="$props" \
    dotnet build "$proj" --no-incremental -p:BuildProjectReferences=false \
      -p:TvrmsmithAnalyzersEnabled=true -p:TvrmsmithAnalyzersScopeToChanged=false -v:m --nologo 2>&1)
  if [ $? -ne 0 ]; then
    status=1
    echo "=== $proj — the diagnostics pass failed after a clean build ==="
    grep -E ': error [A-Z]+[0-9]+' <<<"$out" | sort -u
    continue
  fi

  # MSBuild repeats each diagnostic across its passes, and the summary at the end repeats it
  # again, so the same finding arrives several times. Cut to the message and dedupe.
  for file in $(printf '%s\n' "${pairs[@]}" | awk -F'\t' -v p="$proj" '$1 == p { print $2 }' | sort -u); do
    grep -F "$file(" <<<"$out" \
      | grep -E ': warning (TVRM|FAA)[0-9]+' \
      | sed -e 's/^ *[0-9]*>//' -e 's/ \[[^]]*\.csproj\]$//' \
      | sort -u >>"$findings"
  done
done

if [ -s "$findings" ]; then
  count=$(sort -u "$findings" | tee "$findings.u" | wc -l | tr -d ' ')
  mv "$findings.u" "$findings"
  echo
  echo "personal coding standards — $count finding(s) in the changed C# files:"
  sed 's|^'"$repo_root"'/||; s/^/  /' "$findings"
  echo
  echo "  reported, not blocking. Every id here is a warning by design, so the commit proceeds."
fi

exit $status
