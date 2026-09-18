#!/usr/bin/env bash
#
# Block the commit on analyzer diagnostics touching changed C# lines.
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
# This blocks. A compile error is the build's own verdict and fails as it always did. A warning
# now fails too, but only when it touches a line the change actually wrote, which is the
# per-changed-line scoping an earlier version of this header named as the missing precondition.
# `lint-changed` does that scoping: the build writes SARIF, this script pipes it in, and its
# exit status becomes the script's. Exit 2 is "a finding survived", so a hook can tell that from
# the gate breaking. ADR 0010 carries the rule and the reasoning.
#
# Severity in `Descriptors` stays at Warning, since adoption is machine-local against code other
# people wrote and no previously-succeeding build may start failing. The build's verdict is
# still the build's; the blocking verdict is this script's.
#
# Past a genuine false positive there is one route, and `lint-changed` prints the exact command
# for it. Waivers are one-shot, carry a reason, and live in a log outside the repo.
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
    -h|--help) sed -n '2,30p' "${BASH_SOURCE[0]}"; exit 0 ;;
    *) echo "lint-changed-dotnet: unknown argument '$1'" >&2; exit 1 ;;
  esac
done

command -v dotnet >/dev/null 2>&1 || {
  echo "lint-changed-dotnet: no dotnet on PATH — skipped" >&2
  exit 0
}

repo_root=$(git rev-parse --show-toplevel 2>/dev/null) || {
  echo "lint-changed-dotnet: not inside a git repository" >&2
  exit 1
}
# The analyzers are scoped by a StartsWith condition on the *resolved* project directory, so
# every path this script derives has to be resolved too or nothing matches (research caveat 2).
repo_root=$(cd "$repo_root" && pwd -P)
cd "$repo_root" || exit 1

# A linked worktree is the same adoption as the checkout it was made from, so the registry is
# keyed on the main checkout rather than on where the commit happens to be taken. The hook is
# shared anyway — it lives in the common git dir — so keying on $repo_root would install a hook
# in every worktree that then skipped, which reads exactly like the layer being broken. The
# common git dir is <main>/.git in both cases, so its parent is the main checkout.
registry_key=$repo_root
if common_git_dir=$(cd "$(git rev-parse --git-common-dir)" 2>/dev/null && pwd -P); then
  registry_key=$(cd "$common_git_dir/.." && pwd -P)
fi

# Whether this repo is adopted is a question the props file already answers: it carries one
# path-scoped Import per adopted repo, so the scoping condition doubles as the registry. That
# keeps the pre-commit hook free of per-language state — one template serves both branches, and
# each decides for itself whether it applies here.
#
# Skip, don't fail. A repo bootstrapped for TypeScript only must not have its commits blocked
# by a .NET branch that was never wired up.
if [ ! -f "$props" ] || ! grep -qF "StartsWith('$registry_key/')" "$props"; then
  [ "$mode" = "--staged" ] \
    || echo "lint-changed-dotnet: $registry_key is not wired for .NET — run 'bootstrap dotnet $registry_key'" >&2
  exit 0
fi

# Adoption is settled above, so the builds below import the analyzer props directly instead of
# going through the path-scoped wrapper. The wrapper's condition names the main checkout, and
# a project in a linked worktree sits outside it — routing through it would skip the analyzers
# in every worktree while the registry check said the repo was wired up. Importing directly
# also means a new worktree needs no bootstrap of its own.
hub=$(cd -P -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
analyzer_props=$hub/dotnet/artifacts/local/Tvrmsmith.Analyzers.Local.props
if [ ! -f "$analyzer_props" ]; then
  echo "lint-changed-dotnet: $analyzer_props is missing — run 'bootstrap dotnet $registry_key'" >&2
  exit 0
fi

# NUL-delimited throughout. git quotes a path holding a space or a non-ASCII byte in its
# newline-delimited output, and read -r splits the quoted form on whitespace, so a newline list
# drops exactly the filenames a developer is most likely to get wrong. The list goes through a
# process substitution rather than a command substitution because `$(...)` strips NUL bytes.
list_changed() {
  case "$mode" in
    --staged) git diff --cached --name-only -z --diff-filter=ACM ;;
    --since)  git diff --name-only -z --diff-filter=ACM "$ref" ;;
    --files)  [ ${#explicit_files[@]} -gt 0 ] && printf '%s\0' "${explicit_files[@]}" ;;
  esac
}

files=()
while IFS= read -r -d '' file; do
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
done < <(list_changed)

[ ${#files[@]} -gt 0 ] || exit 0

# A staged file whose worktree copy differs still cannot be honoured here the way
# lint-changed.sh honours it: ESLint takes content on stdin, MSBuild compiles what is on disk.
# At warning severity that was a caveat worth printing. Now that a finding blocks, it would fail
# a commit over code the commit does not contain, so lint-changed makes it a hard stop instead.
# The check lives there rather than here because it has to cover every staged path, not only the
# ones this script chose to build.

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

# lint-changed is the blocking half, and it is language-neutral: it reads the SARIF below,
# keeps only the findings touching a changed line, applies any waiver, and sets the exit status.
#
# Built here rather than bootstrapped, because Go's build cache makes a rebuild of an unchanged
# tree cost milliseconds against a dotnet build's seconds, and building every time is one less
# thing that can go stale. It has to be a built binary rather than `go run`: lint-changed reads
# the git repo it is *run in*, and `go run` would have to run in the hub's module directory.
command -v go >/dev/null 2>&1 || {
  echo "lint-changed-dotnet: no go on PATH, so the changed-line filter cannot run" >&2
  echo "  Findings would go unchecked and the commit would pass unexamined, so this is a failure" >&2
  echo "  rather than a skip. Install Go, or unstage the C# changes." >&2
  exit 1
}

cache_home=${XDG_CACHE_HOME:-$HOME/.cache}
lint_changed=$cache_home/coding-standards/lint-changed
mkdir -p "$(dirname "$lint_changed")" || exit 1
if ! build_out=$(cd "$hub" && go build -o "$lint_changed" ./lint/cmd/lint-changed 2>&1); then
  echo "lint-changed-dotnet: could not build lint-changed, so the changed-line filter cannot run" >&2
  printf '%s\n' "$build_out" >&2
  exit 1
fi

case "$mode" in
  --staged) scope_args=(--staged) ;;
  --since)  scope_args=(--since "$ref") ;;
  --files)  scope_args=(--files "$(IFS=,; echo "${files[*]}")") ;;
esac

status=0
sarif_dir=$(mktemp -d)
trap 'rm -rf "$sarif_dir"' EXIT
sarifs=()

# The SARIF report name is MSBuild's to build, not this script's: it has to expand
# $(TargetFramework) once per inner build, and it has to escape the comma before the version
# suffix. errorlog.props carries both and says why; all pass 2 passes in is the prefix.
errorlog_props=$hub/harness/errorlog.props

# Read rather than word-split: an unquoted expansion splits a project path on every space it
# holds and then globs each piece.
while IFS= read -r proj; do
  [ -n "$proj" ] || continue
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
  out=$(CustomAfterMicrosoftCommonProps="$analyzer_props" \
    dotnet build "$proj" -p:TvrmsmithAnalyzersEnabled=true \
      -p:TvrmsmithAnalyzersScopeToChanged=false -v:m --nologo 2>&1 </dev/null)
  build_status=$?

  # A compile error is the build's verdict and it stands — that is the whole gate here.
  #
  # Reported as 1, never as the build's own status. The exit codes here mean one thing each: 2 is
  # "a finding survived the changed-line filter", and MSBuild exiting 2 for its own reasons must
  # not be read as that.
  if [ $build_status -ne 0 ]; then
    status=1
    echo "=== $proj — build failed ==="
    grep -E ': (error|warning) [A-Z]+[0-9]+' <<<"$out" | sort -u
    continue
  fi

  # Pass 2 forces the diagnostics out. Analyzers only run when csc runs, and pass 1 has just
  # left the project up to date, so on its own it reports nothing at all: zero diagnostics
  # incremental against eight forced on the same tree. --no-incremental makes csc run again;
  # BuildProjectReferences=false stops that force from cascading through the graph, which is
  # safe here and only here, because pass 1 has already put the referenced assemblies on disk.
  #
  # ErrorLog is what makes the diagnostics readable rather than greppable. MSBuild repeats each
  # one across its passes and again in the summary, and its console format carries no line span
  # and no related locations. SARIF carries all three, so the scoping below works off structure
  # instead of a regex over English.
  #
  # The report name is built by errorlog.props above, one per framework, all of them collected
  # below. All this pass hands over is the prefix.
  prefix=$sarif_dir/$(echo "$proj" | tr '/' '_')
  out=$(CustomAfterMicrosoftCommonProps="$analyzer_props" \
    CustomAfterMicrosoftCommonTargets="$errorlog_props" \
    dotnet build "$proj" --no-incremental -p:BuildProjectReferences=false \
      -p:TvrmsmithAnalyzersEnabled=true -p:TvrmsmithAnalyzersScopeToChanged=false \
      -p:TvrmsmithSarifPrefix="$prefix" -v:m --nologo 2>&1 </dev/null)
  if [ $? -ne 0 ]; then
    status=1
    echo "=== $proj — the diagnostics pass failed after a clean build ==="
    grep -E ': error [A-Z]+[0-9]+' <<<"$out" | sort -u
    continue
  fi

  # A clean build still writes a SARIF log with an empty results array, so no report at all means
  # ErrorLog never took effect — the SDK ignoring the property, the project overriding it, a write
  # that failed. Findings would go unchecked and the commit would pass unexamined, which is the
  # same failure the missing-go branch above refuses to pass off as a skip.
  found=0
  for sarif in "$prefix".*.sarif; do
    [ -s "$sarif" ] || continue
    sarifs+=("$sarif")
    found=1
  done
  if [ $found -eq 0 ]; then
    status=1
    echo "=== $proj — the build wrote no SARIF report, so nothing could be checked ===" >&2
    echo "  Expected $prefix.<framework>.sarif from ErrorLog. Check that the project does not" >&2
    echo "  set its own ErrorLog." >&2
  fi
done <<<"$projects"

# Every report goes into one lint-changed run, named with --report. A process per report was the
# first shape and it was wrong: a waiver is one commit's worth of permission, and each process
# spent whatever matched its own report without knowing another report still blocked the commit,
# so the waiver was burnt on a commit that never went through. One process sees the whole
# commit's findings and makes one spend decision. Merging the SARIF here instead would mean this
# script understanding SARIF, which is the one thing handing the job to lint-changed buys.
#
# A broken run is reported as 1 whatever else happened, because a filter that did not run proves
# nothing; 2 only survives when nothing broke.
#
# The ${#sarifs[@]} guard is for bash 3.2, where expanding an empty array under `set -u` is an
# unbound-variable error rather than an empty expansion.
if [ ${#sarifs[@]} -gt 0 ]; then
  report_args=()
  for sarif in "${sarifs[@]}"; do
    report_args+=(--report "$sarif")
  done
  "$lint_changed" --format sarif --language csharp "${scope_args[@]}" "${report_args[@]}" </dev/null
  case $? in
    0) ;;
    2) [ $status -eq 0 ] && status=2 ;;
    *) status=1 ;;
  esac
fi

exit $status
