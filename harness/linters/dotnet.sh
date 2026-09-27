#!/usr/bin/env bash
#
# The C# branch of lint-changed.sh. Sourced, never executed.
#
# It works differently from the other two in one important way: Roslyn analyzers only run as part
# of a compilation, so there is no way to lint a file on its own. This builds the project that
# owns each changed file and filters the diagnostics down to the changed lines.
#
# This blocks. A compile error is the build's own verdict and fails as it always did. A warning
# now fails too, but only when it touches a line the change actually wrote, which is the
# per-changed-line scoping an earlier version of this header named as the missing precondition.
# `lint-changed` does that scoping: the build writes SARIF, this branch hands it up, and the
# dispatcher's one filter run over every branch's reports sets the exit status. Exit 2 is "a
# finding survived", so a hook can tell that from the gate breaking. ADR 0010 carries the rule and
# the reasoning.
#
# Severity in `Descriptors` stays at Warning, since adoption is machine-local against code other
# people wrote and no previously-succeeding build may start failing. The build's verdict is still
# the build's; the blocking verdict is the filter's.
#
# Past a genuine false positive there is one route, and `lint-changed` prints the exact command
# for it. Waivers are one-shot, carry a reason, and live in a log outside the repo.
#
# Unlike the other two branches, this one refuses rather than skips when it cannot do its job. In
# an adopted repo a missing dotnet, a missing analyzer props file, a missing Go toolchain or a
# build that wrote no SARIF would each mean the commit passing on a compilation nothing inspected,
# which is the failure this exists to prevent. Those return non-zero. Only "no C# here" and "not
# wired for .NET" return 0.

config_home=${XDG_CONFIG_HOME:-$HOME/.config}
props=${TVRMSMITH_ANALYZER_PROPS:-$config_home/coding-standards.props}
analyzer_props=${TVRMSMITH_ANALYZER_LOCAL_PROPS:-$hub/dotnet/artifacts/local/Tvrmsmith.Analyzers.Local.props}

dotnet_owns() {
  case "$1" in
    # Generated output is never worth reporting on, and obj/ is full of .cs the SDK wrote.
    */obj/*|obj/*|*/bin/*|bin/*) return 1 ;;
  esac
  case "$1" in
    *.cs) return 0 ;;
    *) return 1 ;;
  esac
}

dotnet_lint() {
  local file proj member out build_status status=0 projects=()
  local files=() prefix sarif sarifs=() found errorlog_props groups=$scratch/dotnet-owners
  local sarif_dir=$scratch/dotnet-sarif adopted

  # Whether this repo is adopted is a question the props file already answers: it carries one
  # path-scoped Import per adopted repo, so the scoping condition doubles as the registry. That
  # keeps the pre-commit hook free of per-language state.
  adopted=$(adoption csharp "$props") || return 1
  if [ "$adopted" != adopted ]; then
    not_wired .NET dotnet
    return 0
  fi

  # Below the registry check, so a repo wired for nothing .NET-shaped has already returned. Past
  # it the repo is adopted, and no dotnet means no compilation, so nothing inspected the changed
  # C# at all — a GUI git client or a stripped login shell is enough to get here. That is the gate
  # failing to run, the same as the missing Go toolchain below, so it refuses rather than skips.
  command -v dotnet >/dev/null 2>&1 || {
    echo "lint-changed: no dotnet on PATH, so the changed C# cannot be compiled or inspected" >&2
    echo "  This repo is wired for .NET, so this is a failure rather than a skip. Put dotnet on" >&2
    echo "  PATH, or unstage the C# changes." >&2
    return 1
  }

  # Adoption is settled above, so the builds below import the analyzer props directly instead of
  # going through the path-scoped wrapper. The wrapper's condition names the main checkout, and a
  # project in a linked worktree sits outside it — routing through it would skip the analyzers in
  # every worktree while the registry check said the repo was wired up. Importing directly also
  # means a new worktree needs no bootstrap of its own.
  #
  # The registry above already said this repo is wired for .NET, so missing props means the
  # bootstrap output was deleted or never materialised. Building without the analyzers would
  # report clean on a compilation nothing inspected, so this refuses rather than warns.
  if [ ! -f "$analyzer_props" ]; then
    echo "lint-changed: $analyzer_props is missing, so the analyzers cannot load" >&2
    echo "  Run 'bootstrap dotnet $registry_key', or unstage the C# changes." >&2
    return 1
  fi

  # A file the change carries but the working tree does not is named rather than dropped in
  # silence. MSBuild has nothing to compile for it, so the run below is the divergence check
  # alone, and a reader who is not told that reads the pass as the code being clean.
  for file in "$@"; do
    if [ ! -e "$file" ]; then
      echo "lint-changed: $file is in the change but absent from the working tree, so nothing compiled it" >&2
      continue
    fi
    files+=("$file")
  done

  # The dispatcher only calls a branch that owns something, so C# is in the change by definition.
  # An empty build set still goes on under --staged, because the staged-versus-disk hard stop in
  # the dispatcher's filter has to be asked, and it runs only once a branch reaches add_reports:
  # every staged .cs being absent from disk is exactly the case where returning here would pass
  # the commit unexamined.
  [ ${#files[@]} -gt 0 ] || [ "$mode" = "--staged" ] || return 0

  # The project that owns a file: every .csproj in its nearest ancestor holding one. That is also
  # the directory MSBuild treats as the project root, so every .cs below it is in each of those
  # compilations by default, and picking one would lint the file under settings the others may not
  # share.
  #
  # The blocking half, built by common.sh since every branch's reports go to the same binary, also
  # answers this. The dispatcher runs it over the SARIF below, keeping only the findings touching a
  # changed line and applying any waiver. owner_groups builds it before a single build runs,
  # because a filter that will not build makes every SARIF it would have produced unreadable anyway.
  owner_groups csharp "$groups" ${files[@]+"${files[@]}"} || return 1
  while IFS= read -r -d '' proj; do
    projects+=("$proj")
    # The members are the changed files, and a build compiles the whole project whatever they are.
    while IFS= read -r -d '' member && [ -n "$member" ]; do :; done
  done <"$groups"
  [ ${#projects[@]} -gt 4 ] && echo "lint-changed: ${#projects[@]} projects to build; this will take a moment" >&2

  mkdir -p "$sarif_dir" || return 1

  # The SARIF report name is MSBuild's to build, not this branch's: it has to expand
  # $(TargetFramework) once per inner build, and it has to escape the comma before the version
  # suffix. errorlog.props carries both and says why; all pass 2 passes in is the prefix.
  errorlog_props=$harness/errorlog.props

  for proj in ${projects[@]+"${projects[@]}"}; do
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
    # Scoping off in both passes. The build-time scoping is keyed on the working tree against
    # HEAD; this branch is keyed on what is *staged*, and the two sets are not the same one.
    # Letting both filter would make the reported set the intersection, silently. The filter
    # below stays the single answer to "which lines count".
    out=$(CustomAfterMicrosoftCommonProps="$analyzer_props" \
      dotnet build "$proj" -p:TvrmsmithAnalyzersEnabled=true \
        -p:TvrmsmithAnalyzersScopeToChanged=false -v:m --nologo 2>&1 </dev/null)
    build_status=$?

    # A compile error is the build's verdict and it stands — that is the whole gate here.
    #
    # Reported as 1, never as the build's own status. The exit codes here mean one thing each: 2
    # is "a finding survived the changed-line filter", and MSBuild exiting 2 for its own reasons
    # must not be read as that.
    if [ $build_status -ne 0 ]; then
      status=1
      # stderr, not stdout: stdout carries one porcelain finding line and nothing else, and an
      # MSBuild diagnostic is close enough to that shape for the one regex to read it as a finding.
      echo "=== $proj — build failed ===" >&2
      grep -E ': (error|warning) [A-Z]+[0-9]+' <<<"$out" | sort -u >&2
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
    prefix=$sarif_dir/$(echo "$proj" | tr '/' '_')
    out=$(CustomAfterMicrosoftCommonProps="$analyzer_props" \
      CustomAfterMicrosoftCommonTargets="$errorlog_props" \
      dotnet build "$proj" --no-incremental -p:BuildProjectReferences=false \
        -p:TvrmsmithAnalyzersEnabled=true -p:TvrmsmithAnalyzersScopeToChanged=false \
        -p:TvrmsmithSarifPrefix="$prefix" -v:m --nologo 2>&1 </dev/null)
    if [ $? -ne 0 ]; then
      status=1
      echo "=== $proj — the diagnostics pass failed after a clean build ===" >&2
      grep -E ': error [A-Z]+[0-9]+' <<<"$out" | sort -u >&2
      continue
    fi

    # A clean build still writes a SARIF log with an empty results array, so no report at all
    # means ErrorLog never took effect — the SDK ignoring the property, the project overriding it,
    # a write that failed. Findings would go unchecked and the commit would pass unexamined, which
    # is the same failure the missing-go branch above refuses to pass off as a skip.
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
  done

  # Every report goes to the dispatcher's one filter run. Merging the SARIF here instead would mean
  # this branch understanding SARIF, which is the one thing handing the job to lint-changed buys.
  #
  # A broken build still hands on the reports the others wrote, beside its 1. The dispatcher
  # reports 1 whatever the filter says, because a filter that did not read every project proves
  # nothing; 2 only survives when nothing broke.
  add_reports sarif ${sarifs[@]+"${sarifs[@]}"}

  return "$status"
}
