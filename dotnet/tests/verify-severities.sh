#!/usr/bin/env bash
#
# Proves the curated severities actually reach a build through the phase-1 bare-DLL path.
#
# Two builds of tests/Consumer, which references neither Tvrmsmith.Analyzers nor
# FluentAssertions.Analyzers and has Central Package Management on:
#
#   baseline  no CustomAfterMicrosoftCommonProps  -> zero FAA and zero TVRM diagnostics
#   injected  with it                             -> FAA0001, FAA0002, TVRM0001-0003 as *warnings*
#
# For the FAA ids the severity is the load-bearing part: both ship as Info, which never surfaces
# in a build, so "warning FAA0001" can only mean the .globalconfig was applied and not merely
# that the DLL loaded. The TVRM ids default to warning in their own descriptors, so what they
# prove is delivery — that the custom analyzers reach the compiler by both routes.

set -euo pipefail

dotnet_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
consumer="$dotnet_root/tests/Consumer/Consumer.csproj"
artifacts="$dotnet_root/artifacts/local"
# -P matters: on macOS mktemp hands back /var/..., a symlink to /private/var/.... MSBuild
# normalises a project path but does not resolve symlinks, so an unresolved path here and a
# resolved one in a StartsWith condition never match (research caveat 2). Resolve once, at the
# source, so every path derived from it agrees.
workdir="$(cd "$(mktemp -d)" && pwd -P)"
trap 'rm -rf "$workdir"' EXIT

fail() { printf '\nFAIL: %s\n' "$1" >&2; exit 1; }

# Single source of truth for the assertion-library version lives in Directory.Build.props.
fa_version="$(sed -n 's/.*<FluentAssertionsVersion>\(.*\)<\/FluentAssertionsVersion>.*/\1/p' \
  "$dotnet_root/Directory.Build.props")"
[ -n "$fa_version" ] || fail "could not read FluentAssertionsVersion from Directory.Build.props"

echo "==> Building Tvrmsmith.Analyzers"
dotnet build "$dotnet_root/src/Tvrmsmith.Analyzers/Tvrmsmith.Analyzers.csproj" \
  -c Release --nologo -v quiet

for f in Tvrmsmith.Analyzers.dll FluentAssertions.Analyzers.dll \
         Tvrmsmith.Analyzers.globalconfig Tvrmsmith.Analyzers.Local.props \
         tvrmsmith-scope-changed.sh; do
  [ -f "$artifacts/$f" ] || fail "the build did not stage $f into $artifacts"
done
[ -x "$artifacts/tvrmsmith-scope-changed.sh" ] \
  || fail "tvrmsmith-scope-changed.sh is not executable; scoping would silently do nothing"
echo "    staged: $artifacts"

# The injected props file, exactly as it would live at ~/.config/coding-standards/personal.props.
# The StartsWith condition is what scopes the blast radius; here it scopes to the consumer tree,
# which also means the baseline build below is a genuine control.
cat > "$workdir/personal.props" <<EOF
<Project>
  <Import Project="$artifacts/Tvrmsmith.Analyzers.Local.props"
          Condition="\$(MSBuildProjectDirectory.StartsWith('$dotnet_root/tests/Consumer'))" />
</Project>
EOF

# Changed-file scoping off throughout this script's severity assertions. It is on by default and
# tests/Consumer is a committed, unmodified file in this repository, so with it on the correct
# answer is zero diagnostics — which is indistinguishable from an injection that never arrived.
# Scoping gets its own section at the end, in a repository built for it.
scope_off=-p:TvrmsmithAnalyzersScopeToChanged=false

build_consumer() {
  dotnet build "$consumer" -c Debug --nologo --no-incremental "$scope_off" -v normal 2>&1 || true
}

echo "==> Baseline build (no injection)"
rm -rf "$dotnet_root/tests/Consumer/obj" "$dotnet_root/tests/Consumer/bin"
baseline="$(unset CustomAfterMicrosoftCommonProps; build_consumer)"
if grep -qE '\b(FAA|TVRM)[0-9]{4}\b' <<<"$baseline"; then
  grep -E '\b(FAA|TVRM)[0-9]{4}\b' <<<"$baseline" | head -5 >&2
  fail "the consumer emitted analyzer diagnostics without injection — the control is not clean"
fi
grep -q 'Build succeeded' <<<"$baseline" || fail "baseline build did not succeed"
echo "    clean: no FAA or TVRM diagnostics, build succeeded"

echo "==> Injected build (CustomAfterMicrosoftCommonProps)"
rm -rf "$dotnet_root/tests/Consumer/obj" "$dotnet_root/tests/Consumer/bin"
injected="$(CustomAfterMicrosoftCommonProps="$workdir/personal.props" build_consumer)"

for id in FAA0001 FAA0002 TVRM0001 TVRM0002 TVRM0003; do
  grep -q "warning $id" <<<"$injected" \
    || fail "expected '$id' at severity warning; got: $(grep -oE "(warning|info|error) $id" <<<"$injected" | sort -u | tr '\n' ' ')"
  echo "    $id: warning"
done

# CPM is on in the consumer, and a bare <Analyzer Include> must not trip it.
if grep -qE 'error (NU1008|NU1010)' <<<"$injected"; then
  fail "Central Package Management rejected the injection"
fi
grep -q 'Build succeeded' <<<"$injected" || fail "injected build did not succeed"
echo "    build succeeded, no NU1008/NU1010"

#
# TreatWarningsAsErrors, which most repos set and which is what makes "warning"
# the load-bearing word above.
#
# 58 of the target's 60 build trees set it, so without WarningsNotAsErrors every id this
# package emits arrives as a build error and injection breaks builds that succeeded before it.
# Both halves have to hold, and the second is why this is not just "assert the build passed":
#
#   our ids       demoted back to warning, build succeeds
#   everything else  still fatal — WarningsNotAsErrors is an allowlist, not a switch
#
# One consumer, built twice: once clean, once with a file carrying a plain CS0219.
#
echo "==> TreatWarningsAsErrors=true consumer (bare-DLL injection)"

twae="$workdir/twae"
mkdir -p "$twae"
cp "$dotnet_root/tests/Consumer/SloppyAssertions.cs" \
   "$dotnet_root/tests/Consumer/CustomRuleViolations.cs" "$twae/"

cat > "$workdir/personal-twae.props" <<EOF
<Project>
  <Import Project="$artifacts/Tvrmsmith.Analyzers.Local.props"
          Condition="\$(MSBuildProjectDirectory.StartsWith('$twae'))" />
</Project>
EOF

# Mirrors a typical consumer's Directory.Build.props: CPM on, warnings fatal, empty
# WarningsAsErrors, and no reference to either analyzer package.
cat > "$twae/Twae.csproj" <<EOF
<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>net8.0</TargetFramework>
    <Nullable>enable</Nullable>
    <IsPackable>false</IsPackable>
    <TreatWarningsAsErrors>true</TreatWarningsAsErrors>
    <WarningsAsErrors />
  </PropertyGroup>
  <ItemGroup>
    <PackageReference Include="FluentAssertions" Version="$fa_version" />
    <PackageReference Include="xunit" Version="2.9.2" />
  </ItemGroup>
</Project>
EOF

build_twae() {
  CustomAfterMicrosoftCommonProps="$workdir/personal-twae.props" \
    dotnet build "$twae/Twae.csproj" -c Debug --nologo --no-incremental "$scope_off" -v normal 2>&1 || true
}

twae_clean="$(build_twae)"
for id in FAA0001 FAA0002 TVRM0001 TVRM0002 TVRM0003; do
  grep -q "warning $id" <<<"$twae_clean" \
    || fail "TreatWarningsAsErrors: expected '$id' at severity warning; got: $(grep -oE "(warning|error) $id" <<<"$twae_clean" | sort -u | tr '\n' ' ')"
done
grep -qE 'error (FAA|TVRM)[0-9]{4}' <<<"$twae_clean" \
  && fail "TreatWarningsAsErrors: an injected diagnostic became an error — WarningsNotAsErrors did not apply"
grep -q 'Build succeeded' <<<"$twae_clean" \
  || fail "TreatWarningsAsErrors: injection broke a build that has no errors of its own"
echo "    5 injected diagnostics stayed warnings, build succeeded"

# The allowlist half. CS0219 (assigned but never used) is an ordinary warning the target repo
# would fail on today; it must keep failing with the injection in place, or WarningsNotAsErrors
# has been read as a blanket opt-out.
cat > "$twae/PlainCsWarning.cs" <<'EOF'
namespace Twae;

public static class PlainCsWarning
{
    public static void Method()
    {
        int unused = 1; // CS0219: the variable is assigned but its value is never used
    }
}
EOF

twae_cs="$(build_twae)"
grep -q 'error CS0219' <<<"$twae_cs" \
  || fail "WarningsNotAsErrors leaked: CS0219 should still be fatal, got: $(grep -oE '(warning|error) CS0219' <<<"$twae_cs" | sort -u | tr '\n' ' ')"
grep -q 'warning TVRM0001' <<<"$twae_cs" \
  || fail "TVRM0001 should still report as a warning in the same compilation"
echo "    CS0219 still fatal alongside them — the allowlist is an allowlist"
rm -f "$twae/PlainCsWarning.cs"

#
# Phase 2: the same severities, through the nupkg.
#
# Not the shipping path yet, but the package layout is easy to get subtly wrong in ways that
# only show up at consumption — a dependency with exclude="Analyzers" restores clean and emits
# nothing. So consume it for real from a local feed.
#
echo "==> Packing and consuming the nupkg"

# A unique version per run. NuGet caches by id/version in the global packages folder, so a
# rebuilt 0.1.0 restores as whatever 0.1.0 was the first time it was seen — which would quietly
# verify a stale package.
pkg_version="0.1.0-verify.$(date +%s)"

dotnet pack "$dotnet_root/src/Tvrmsmith.Analyzers/Tvrmsmith.Analyzers.csproj" \
  -c Release --nologo -v quiet -o "$workdir/feed" -p:Version="$pkg_version"

pkgconsumer="$workdir/pkgconsumer"
mkdir -p "$pkgconsumer"
cp "$dotnet_root/tests/Consumer/SloppyAssertions.cs" \
   "$dotnet_root/tests/Consumer/CustomRuleViolations.cs" "$pkgconsumer/"

cat > "$pkgconsumer/nuget.config" <<EOF
<configuration>
  <packageSources>
    <clear />
    <add key="local" value="$workdir/feed" />
    <add key="nuget.org" value="https://api.nuget.org/v3/index.json" />
  </packageSources>
</configuration>
EOF

cat > "$pkgconsumer/PkgConsumer.csproj" <<EOF
<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>net8.0</TargetFramework>
    <Nullable>enable</Nullable>
    <IsPackable>false</IsPackable>
    <TreatWarningsAsErrors>false</TreatWarningsAsErrors>
  </PropertyGroup>
  <ItemGroup>
    <PackageReference Include="FluentAssertions" Version="$fa_version" />
    <PackageReference Include="xunit" Version="2.9.2" />
    <!-- No PrivateAssets here on purpose: developmentDependency=true in the nuspec should be
         what keeps this from leaking downstream. -->
    <PackageReference Include="Tvrmsmith.Analyzers" Version="$pkg_version" />
  </ItemGroup>
</Project>
EOF

packaged="$(dotnet build "$pkgconsumer/PkgConsumer.csproj" -c Debug --nologo --no-incremental "$scope_off" -v normal 2>&1 || true)"
for id in FAA0001 FAA0002 TVRM0001 TVRM0002 TVRM0003; do
  grep -q "warning $id" <<<"$packaged" \
    || fail "packaged consumption: expected '$id' at severity warning; got: $(grep -oE "(warning|info|error) $id" <<<"$packaged" | sort -u | tr '\n' ' ')"
  echo "    $id: warning"
done
grep -q 'Build succeeded' <<<"$packaged" || fail "packaged consumer build did not succeed"
echo "    build succeeded"

#
# Changed-file scoping.
#
# A throwaway git repository holding both violation files, both committed, and then one of them
# modified. The two files carry disjoint id sets — SloppyAssertions.cs the FAA ones,
# CustomRuleViolations.cs the TVRM ones — so "scoped correctly" and "suppressed everything" are
# distinguishable, which is the failure this section exists to catch.
#
echo "==> Changed-file scoping"

scoped="$workdir/scoped"
mkdir -p "$scoped"
cp "$dotnet_root/tests/Consumer/SloppyAssertions.cs" \
   "$dotnet_root/tests/Consumer/CustomRuleViolations.cs" "$scoped/"

cat > "$workdir/personal-scoped.props" <<EOF
<Project>
  <Import Project="$artifacts/Tvrmsmith.Analyzers.Local.props"
          Condition="\$(MSBuildProjectDirectory.StartsWith('$scoped'))" />
</Project>
EOF

cat > "$scoped/Scoped.csproj" <<EOF
<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>net8.0</TargetFramework>
    <Nullable>enable</Nullable>
    <IsPackable>false</IsPackable>
  </PropertyGroup>
  <ItemGroup>
    <PackageReference Include="FluentAssertions" Version="$fa_version" />
    <PackageReference Include="xunit" Version="2.9.2" />
  </ItemGroup>
</Project>
EOF

printf 'bin/\nobj/\n' > "$scoped/.gitignore"
git -C "$scoped" init -q
git -C "$scoped" add -A
git -C "$scoped" -c user.name=verify -c user.email=verify@localhost commit -qm 'baseline'

# TVRMSMITH_SCOPE_TTL=0 because these builds run back to back and the generated config is
# normally reused for a few seconds — without it the second build would read the first one's.
# An environment variable and not -p:, because Exec does not export global properties.
build_scoped() {
  TVRMSMITH_SCOPE_TTL=0 CustomAfterMicrosoftCommonProps="$workdir/personal-scoped.props" \
    dotnet build "$scoped/Scoped.csproj" -c Debug --nologo --no-incremental "$@" -v normal 2>&1 || true
}

clean_tree="$(build_scoped)"
if grep -qE ': warning (FAA|TVRM)[0-9]{4}' <<<"$clean_tree"; then
  grep -E ': warning (FAA|TVRM)[0-9]{4}' <<<"$clean_tree" | head -5 >&2
  fail "nothing is modified, so every injected id should be suppressed"
fi
grep -q 'Build succeeded' <<<"$clean_tree" || fail "scoped build of a clean tree did not succeed"
echo "    clean tree: no injected diagnostics"

printf '\n// touched\n' >> "$scoped/CustomRuleViolations.cs"
one_changed="$(build_scoped)"
grep -q 'warning TVRM0001' <<<"$one_changed" \
  || fail "the modified file's diagnostics were suppressed too — scoping suppressed everything"
if grep -q 'warning FAA' <<<"$one_changed"; then
  fail "an unmodified file still reported: $(grep -oE 'warning FAA[0-9]{4}' <<<"$one_changed" | sort -u | tr '\n' ' ')"
fi
echo "    one file modified: its ids report, the other file's do not"

# The escape hatch, on the same tree: the whole standing backlog is still one flag away.
both="$(build_scoped -p:TvrmsmithAnalyzersScopeToChanged=false)"
grep -q 'warning FAA0001' <<<"$both" \
  || fail "TvrmsmithAnalyzersScopeToChanged=false did not restore the unmodified file's diagnostics"
echo "    scoping off: the unmodified file reports again"

printf '\nPASS: curated severities and the three custom analyzers apply through both the bare-DLL path and the nupkg, scoped to changed files.\n'
