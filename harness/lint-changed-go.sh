#!/usr/bin/env bash
#
# Report personal Go findings on changed .go files only.
#
#   lint-changed-go.sh --staged            # what a commit would contain (the pre-commit hook)
#   lint-changed-go.sh --since main        # everything changed against a ref
#   lint-changed-go.sh --files a.go b.go   # an explicit list
#
# The Go counterpart to lint-changed.sh. The linter is the personal golangci-lint binary built
# by go/build.sh: the custom rules are compiled into it, so there is nothing to install in the
# target repository and nothing for it to resolve.
#
# golangci-lint runs per package, not per file, so this lints the packages owning the changed
# files and filters the output down to those files. Changed-files-only is the same
# non-negotiable it is on the other two sides — a mature repo reports a flood otherwise, and a
# flood is indistinguishable from noise.
#
# Exit status: **findings report, a broken run fails.** Every personal Go rule is advisory, the
# same position the injected Roslyn ids are in, so a finding never blocks a commit.
# golangci-lint exiting non-zero after --issues-exit-code=0 means something else went wrong —
# code that does not typecheck, an unreadable config, a missing package — and that is reported
# as a failure rather than swallowed.
#
# Written for bash 3.2 (the macOS system bash).
set -uo pipefail

# -P: invoked through the ~/.config/coding-standards symlink, an unresolved path would send
# every $hub-relative lookup below into ~/.config.
script_dir=$(cd -P -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
hub=$(cd "$script_dir/.." && pwd -P)
gcl=${TVRMSMITH_GCL:-$hub/go/bin/tvrmsmith-gcl}
config=${TVRMSMITH_GOLANGCI_CONFIG:-$hub/go/golangci.yml}
config_home=${XDG_CONFIG_HOME:-$HOME/.config}
registry=${TVRMSMITH_GO_REPOS:-$config_home/coding-standards-go-repos}

mode=--staged
ref=
explicit_files=()

while [ $# -gt 0 ]; do
  case "$1" in
    --staged) mode=--staged; shift ;;
    --since) mode=--since; ref=${2:?--since needs a ref}; shift 2 ;;
    --files) mode=--files; shift; while [ $# -gt 0 ]; do explicit_files+=("$1"); shift; done ;;
    -h|--help) sed -n '2,25p' "${BASH_SOURCE[0]}"; exit 0 ;;
    *) echo "lint-changed-go: unknown argument '$1'" >&2; exit 2 ;;
  esac
done

repo_root=$(git rev-parse --show-toplevel 2>/dev/null) || {
  echo "lint-changed-go: not inside a git repository" >&2
  exit 2
}
repo_root=$(cd "$repo_root" && pwd -P)
cd "$repo_root" || exit 2

# A linked worktree is the same adoption as the checkout it was made from, so the registry is
# keyed on the main checkout rather than on where the commit happens to be taken. The hook is
# shared anyway — it lives in the common git dir — so keying on $repo_root would install a hook
# in every worktree that then skipped, which reads exactly like the layer being broken. The
# common git dir is <main>/.git in both cases, so its parent is the main checkout.
#
# TVRMSMITH_REGISTRY_KEY overrides the lookup for a caller that knows the answer and would
# derive the wrong one. The no-mistakes pipeline is that caller: it lints in a detached
# worktree of its own bare gate repository under ~/.no-mistakes, so the common git dir resolves
# to that gate rather than to the adopted checkout, and every run would skip silently. It
# passes the registered checkout in NO_MISTAKES_REPO_PATH. Findings are still taken from the
# tree this runs in; only the adoption question is answered elsewhere.
registry_key=${TVRMSMITH_REGISTRY_KEY:-}
if [ -z "$registry_key" ]; then
  registry_key=$repo_root
  if common_git_dir=$(cd "$(git rev-parse --git-common-dir)" 2>/dev/null && pwd -P); then
    registry_key=$(cd "$common_git_dir/.." && pwd -P)
  fi
fi

# Which repos are adopted is state the Go side has nowhere else to keep: nothing is installed in
# the target and the binary is machine-wide, so without this file bootstrapping one repo would
# silently start linting every other repo the hook guards.
#
# Skip, don't fail. A repo bootstrapped for TypeScript only must not have its commits blocked by
# a Go branch that was never wired up.
if [ ! -f "$registry" ] || ! grep -qxF "$registry_key" "$registry"; then
  [ "$mode" = "--staged" ] \
    || echo "lint-changed-go: $registry_key is not wired for Go — run 'bootstrap go $registry_key'" >&2
  exit 0
fi

if [ ! -x "$gcl" ]; then
  echo "lint-changed-go: no personal golangci-lint at $gcl — run $hub/go/build.sh" >&2
  exit 1
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
    *.go) ;;
    *) continue ;;
  esac
  [ -e "$file" ] && files+=("$file")
done <<<"$changed"

[ ${#files[@]} -gt 0 ] || exit 0

# golangci-lint reads what is on disk, so a file staged in one state and left in another is
# linted as the disk copy. Same limitation as the .NET branch, and the same answer: say so
# rather than let it pass silently.
if [ "$mode" = "--staged" ]; then
  divergent=()
  for file in "${files[@]}"; do
    git diff --quiet -- "$file" || divergent+=("$file")
  done
  if [ ${#divergent[@]} -gt 0 ]; then
    echo "lint-changed-go: these are staged in one state and on disk in another; the linter sees the disk copy:" >&2
    printf '    %s\n' "${divergent[@]}" >&2
  fi
fi

# The module a file belongs to: nearest ancestor holding a go.mod. golangci-lint has to run from
# there — outside a module it reports "directory prefix does not contain main module" and finds
# nothing — and a monorepo can hold several.
module_of() {
  local dir
  dir=$(dirname "$1")
  while :; do
    [ -f "$dir/go.mod" ] && { echo "$dir"; return 0; }
    [ "$dir" = "." ] || [ "$dir" = "/" ] && return 1
    dir=$(dirname "$dir")
  done
}

pairs=()
for file in "${files[@]}"; do
  if module=$(module_of "$file"); then
    pairs+=("$module	$(dirname "$file")	$repo_root/$file")
  else
    echo "lint-changed-go: no go.mod above $file — skipped" >&2
  fi
done

[ ${#pairs[@]} -gt 0 ] || exit 0

status=0
findings=$(mktemp)
trap 'rm -f "$findings"' EXIT

for module in $(printf '%s\n' "${pairs[@]}" | cut -f1 | sort -u); do
  packages=()
  for dir in $(printf '%s\n' "${pairs[@]}" | awk -F'\t' -v m="$module" '$1 == m { print $2 }' | sort -u); do
    rel=${dir#"$module"/}
    [ "$rel" = "$module" ] && rel=.
    packages+=("./$rel")
  done

  # --path-mode abs so the reported paths can be matched against the changed set without
  # depending on which directory golangci-lint decided to make them relative to.
  out=$(cd "$module" && "$gcl" run --config "$config" --path-mode abs --issues-exit-code 0 \
    --output.text.print-issued-lines=false --output.text.colors=false "${packages[@]}" 2>&1)
  if [ $? -ne 0 ]; then
    status=1
    echo "=== $module — the lint run failed ==="
    printf '%s\n' "$out"
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
  count=$(sort -u -t: -k1,1 -k2,2n -k3,3n -k4 "$findings" | tee "$findings.u" | wc -l | tr -d ' ')
  mv "$findings.u" "$findings"
  echo
  echo "personal coding standards — $count finding(s) in the changed Go files:"
  sed 's|^'"$repo_root"'/||; s/^/  /' "$findings"
  echo
  echo "  reported, not blocking. Every personal Go rule is advisory, so the commit proceeds."
fi

exit $status
