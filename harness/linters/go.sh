#!/usr/bin/env bash
#
# The Go branch of lint-changed.sh. Sourced, never executed.
#
# The linter is the personal golangci-lint binary built by go/build.sh: the custom rules are
# compiled into it, so there is nothing to install in the target repository and nothing for it to
# resolve.
#
# golangci-lint runs per package, not per file, so this lints the packages owning the changed
# files and filters the output down to those files.
#
# Exit status: **findings report, a broken run fails.** Every personal Go rule is advisory, the
# same position the injected Roslyn ids are in, so a finding never blocks a commit. golangci-lint
# exiting non-zero after --issues-exit-code=0 means something else went wrong — code that does not
# typecheck, an unreadable config, a missing package — and that is reported as a failure rather
# than swallowed.

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
  local file module dir rel out status=0 pairs=() packages=() findings=$scratch/go

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

  warn_divergent linter "$@"

  # The module a file belongs to: nearest ancestor holding a go.mod. golangci-lint has to run from
  # there — outside a module it reports "directory prefix does not contain main module" and finds
  # nothing — and a monorepo can hold several.
  for file in "$@"; do
    # Deleted in the change, or named by --files and never there. golangci-lint reads the disk and
    # this branch is advisory, so there is nothing here to report and nothing to refuse.
    [ -e "$file" ] || continue
    if module=$(ancestor_with "$file" _go_has_mod); then
      pairs+=("$module	$(dirname "$file")	$repo_root/$file")
    else
      echo "lint-changed: no go.mod above $file — skipped" >&2
    fi
  done
  [ ${#pairs[@]} -gt 0 ] || return 0

  : >"$findings"
  for module in $(printf '%s\n' "${pairs[@]}" | cut -f1 | sort -u); do
    packages=()
    for dir in $(printf '%s\n' "${pairs[@]}" | awk -F'\t' -v m="$module" '$1 == m { print $2 }' | sort -u); do
      rel=${dir#"$module"/}
      [ "$rel" = "$module" ] && rel=.
      packages+=("./$rel")
    done

    # --path-mode abs so the reported paths can be matched against the changed set without
    # depending on which directory golangci-lint decided to make them relative to.
    out=$(cd "$module" && "$gcl" run --config "$golangci_config" --path-mode abs --issues-exit-code 0 \
      --output.text.print-issued-lines=false --output.text.colors=false "${packages[@]}" 2>&1)
    if [ $? -ne 0 ]; then
      status=1
      { echo "=== $module — the lint run failed ==="; printf '%s\n' "$out"; } | report_failure
      continue
    fi

    for file in $(printf '%s\n' "${pairs[@]}" | awk -F'\t' -v m="$module" '$1 == m { print $3 }' | sort -u); do
      grep -F "$file:" <<<"$out" >>"$findings"
    done
  done

  if [ -s "$findings" ]; then
    # Dedupe (a file can be reported by two passes) and order by file, then line, then column.
    # Numerically: a plain sort reads line 102 as coming before line 14. The trailing `-k4` is
    # load-bearing, because `-u` compares the keys rather than the line: without it, two linters
    # reporting the same position would collapse into one finding.
    sort -u -t: -k1,1 -k2,2n -k3,3n -k4 "$findings" -o "$findings"
    report_findings "$findings" "Go files" \
      "reported, not blocking. Every personal Go rule is advisory, so the commit proceeds."
  fi

  return $status
}
