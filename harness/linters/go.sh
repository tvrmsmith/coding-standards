#!/usr/bin/env bash
#
# The Go branch of lint-changed.sh. Sourced, never executed.
#
# The linter is the personal golangci-lint binary built by go/build.sh: the custom rules are
# compiled into it, so there is nothing to install in the target repository and nothing for it to
# resolve.
#
# golangci-lint runs per package, not per file, so this lints the packages owning the changed
# files and hands their JSON reports up for the dispatcher's one `lint-changed` run, which keeps
# the findings touching a line the change actually wrote and drops the rest.
#
# This blocks, on ADR 0010's convention: the filter answers 2 when a finding survived it, and this
# branch returns 1 when it could not answer at all — a missing linter binary, a golangci run that
# blew up, a filter that would not build. Past a genuine false positive there is one route, and
# `lint-changed` prints the exact command for it.

gcl=${TVRMSMITH_GCL:-$hub/go/bin/tvrmsmith-gcl}
golangci_config=${TVRMSMITH_GOLANGCI_CONFIG:-$hub/go/golangci.yml}
go_registry=${TVRMSMITH_GO_REPOS:-${XDG_CONFIG_HOME:-$HOME/.config}/coding-standards-go-repos}

go_owns() {
  case "$1" in
    *.go) return 0 ;;
    *) return 1 ;;
  esac
}

_go_has_mod() { [ -f "$1/go.mod" ]; }

go_lint() {
  local file module dir rel status=0 present=() pairs=() packages=()
  local report reports=()
  local report_dir=$scratch/go-reports module_count=0

  # Which repos are adopted is state the Go side has nowhere else to keep: nothing is installed in
  # the target and the binary is machine-wide, so without this file bootstrapping one repo would
  # silently start linting every other repo the hook guards.
  if [ ! -f "$go_registry" ] || ! grep -qxF "$registry_key" "$go_registry"; then
    not_wired Go go
    return 0
  fi

  if [ ! -x "$gcl" ]; then
    echo "lint-changed: no personal golangci-lint at $gcl — run $hub/go/build.sh" >&2
    return 1
  fi

  # Deleted in the change, or named by --files and never there. golangci-lint reads the disk, so
  # there is nothing here for it to lint. The staged-versus-disk question a missing file raises is
  # lint-changed's, and it asks it across every staged path under --staged rather than only the
  # ones that reached a report, which is why nothing here returns early: add_reports at the bottom
  # has to be reached even when no module produced a report at all, or the dispatcher runs no
  # filter to ask it.
  for file in "$@"; do
    [ -e "$file" ] && present+=("$file")
  done

  # The module a file belongs to: nearest ancestor holding a go.mod. golangci-lint has to run from
  # there — outside a module it reports "directory prefix does not contain main module" and finds
  # nothing — and a monorepo can hold several.
  for file in ${present[@]+"${present[@]}"}; do
    if module=$(ancestor_with "$file" _go_has_mod); then
      pairs+=("$module	$(dirname "$file")	$repo_root/$file")
    else
      echo "lint-changed: no go.mod above $file — skipped" >&2
    fi
  done

  # Fail fast, before a single golangci run: a filter that will not build makes every report it
  # would have produced unreadable anyway.
  lint_changed_bin >/dev/null || return 1
  mkdir -p "$report_dir" || return 1

  # Read line by line rather than word-split: an unquoted `$(...)` splits a module or file path on
  # every space it holds and then globs each piece, which is the one thing the NUL-delimited
  # changed set upstream exists to prevent. With no module at all the loop reads one empty line
  # and does nothing, which is the path that leaves the dispatcher's filter to ask the divergence
  # question on its own.
  while IFS= read -r module; do
    [ -n "$module" ] || continue
    packages=()
    while IFS= read -r dir; do
      [ -n "$dir" ] || continue
      # Three cases, spelled out, because a prefix strip alone cannot tell them apart and either
      # collapse sends golangci-lint at the wrong package, so the file reports clean unlinted.
      #
      #   a module at the repo root, where ancestor_with answers "." and there is no prefix to
      #   strip, so the package path is $dir as it stands;
      #   the module's own root directory, which is package ".";
      #   anything below the module, which is $dir with the module prefix removed — including a
      #   package repeating the module's name, gate/gate under module gate.
      if [ "$module" = "." ]; then
        rel=$dir
      elif [ "$dir" = "$module" ]; then
        rel=.
      else
        rel=${dir#"$module"/}
      fi
      packages+=("./$rel")
    done <<<"$(printf '%s\n' ${pairs[@]+"${pairs[@]}"} | awk -F'\t' -v m="$module" '$1 == m { print $2 }' | sort -u)"

    # One report per module, named by count rather than by the module path, which can hold any
    # byte a directory name can.
    module_count=$((module_count + 1))
    report=$report_dir/$module_count.json

    # Every one of these flags is load-bearing, so each gets its reason.
    #
    # --path-mode abs so the reported paths resolve inside the repo whatever directory golangci
    # decided to make them relative to.
    #
    # --issues-exit-code 0 stays now that this branch blocks. lint-changed owns the blocking
    # verdict, so golangci's own non-zero has to keep meaning only that the run itself broke, the
    # same split dotnet.sh draws where the build's verdict stays the build's.
    #
    # --output.json.path takes a file rather than `stdout`. Pointed at stdout, golangci writes the
    # JSON document on the first line and then a human summary after it on the same stream, which
    # is unparseable as a report; a file sidesteps it and is the --report shape lint-changed wants
    # anyway. --show-stats=false silences that same summary, leaving golangci's stdout byte-empty.
    #
    # --max-issues-per-linter 0 --max-same-issues 0 because the defaults, 50 and 3, truncate
    # silently. A blocking gate that drops the fifty-first finding is the silent drop this whole
    # design exists to prevent.
    #
    # golangci's stderr is let through rather than captured: it carries the deprecation warnings
    # and the real diagnostics of a run that went wrong, and a reader needs those where they
    # happen rather than folded into a failure message after the fact.
    if ! (cd "$module" && "$gcl" run --config "$golangci_config" --path-mode abs --issues-exit-code 0 \
      --show-stats=false --max-issues-per-linter 0 --max-same-issues 0 \
      --output.json.path "$report" "${packages[@]}") </dev/null; then
      status=1
      # stderr, not stdout: stdout carries one porcelain finding line and nothing else, and a
      # banner on it is a line no-mistakes' one regex can read.
      echo "=== $module — the lint run failed ===" >&2
      continue
    fi

    reports+=("$report")
  done <<<"$(printf '%s\n' ${pairs[@]+"${pairs[@]}"} | cut -f1 | sort -u)"

  # Every module that did produce a report still hands it on, beside a 1 for the one that blew up:
  # the dispatcher reports 1 whatever the filter says, because a filter that read only some of the
  # modules proves nothing about the rest, but the findings it did read still reach stdout.
  add_reports golangci ${reports[@]+"${reports[@]}"}

  return "$status"
}
