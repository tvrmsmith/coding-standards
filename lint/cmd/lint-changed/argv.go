// Command lint-changed is the blocking half of a pre-commit lint gate. It
// reads every linter report named by --report, zero or more, keeps only the
// findings that touch lines the commit changed, lets a one-shot waiver
// suppress one of them, and exits non-zero if any survive. A separate spend
// form marks a matched waiver as used, once the dispatcher that called the
// filter knows the whole commit went through, a changed-paths form lists the
// files a run covers, which the dispatcher hands to each language branch, and
// an owners form groups a branch's files by the build unit that lints them.
// An adopted form tells a branch whether its language is wired up at all.
//
// argv is parsed by hand rather than through the flag package, for the
// reason gate/internal/scope gives: flag exits 2 on a usage mistake and lets
// a stray positional argument through in silence, and this binary's exit
// codes are a one-way door (0 clean, 1 broken, 2 a finding survived) that a
// usage mistake must never collapse into the last of the three.
package main

import (
	"github.com/tvrmsmith/coding-standards/lint/internal/lintfind"
)

// Kind is which of lint-changed's six forms argv named.
type Kind int

const (
	// KindFilter reads every --report named, scopes the findings to the
	// diff, and lets a waiver suppress a survivor. Anything not naming
	// another form means this.
	KindFilter Kind = iota
	// KindWaive records a one-shot waiver.
	KindWaive
	// KindWaivers lists every recorded waiver.
	KindWaivers
	// KindSpend marks one or more already-matched waivers spent against the
	// current index tree. Separate from the filter form because only the
	// dispatcher, which sees every language branch, knows the commit was
	// clean.
	KindSpend
	// KindChangedPaths prints the files a run under one scope covers,
	// NUL-terminated.
	KindChangedPaths
	// KindOwners groups one language's changed files by the build unit that
	// owns each, NUL-terminated.
	KindOwners
	// KindAdopted answers whether a repository is adopted for one language.
	KindAdopted
)

// ScopeMode is which base a filter run scopes its findings against.
type ScopeMode int

const (
	ScopeStaged ScopeMode = iota
	ScopeSince
	ScopeFiles
)

// Scope is the three mutually exclusive flags that say which change a run is
// about.
type Scope struct {
	Mode ScopeMode
	// Ref is the argument --since named. Set only when Mode is ScopeSince.
	Ref string
	// Files is every --files path, one per flag, since a comma is legal in a
	// filename. Set only when Mode is ScopeFiles.
	Files []string
}

// Command is one parsed invocation.
type Command struct {
	Kind         Kind
	Filter       FilterArgs
	Waive        WaiveArgs
	Spend        SpendArgs
	ChangedPaths Scope
	Owners       OwnersArgs
	Adopted      AdoptedArgs
}

// Report is one report path and the parser that reads it, paired at parse
// time by the --format in force when the path was given.
type Report struct {
	Path   string
	Parser lintfind.Parser
}

// FilterArgs is argv for the filter form.
type FilterArgs struct {
	Scope
	// Reports is every --report the caller gave, each paired with the parser
	// the --format in force named. Empty is legal: it means the branches ran
	// and produced nothing. One run reads them all, because a waiver is one
	// commit's worth of permission and a process per report would spend it
	// on whichever report happened to come first.
	Reports []Report
	// MatchedWaivers is the path --matched-waivers named, where the run
	// writes the id of every waiver that matched, one per line. Empty means
	// write nowhere. The filter never spends: the dispatcher is the only
	// caller that knows whether the whole commit went through.
	MatchedWaivers string
	// AcceptSpent marks a run that will spend nothing, so a waiver already
	// spent against any tree still covers its finding.
	AcceptSpent bool
}

// WaiveArgs is argv for the waive form.
type WaiveArgs struct {
	Language string
	Path     string
	Rule     string
	Reason   string
}

// AdoptedArgs is argv for the adopted form.
type AdoptedArgs struct {
	// Language is lintfind.LanguageGo or lintfind.LanguageCSharp. TypeScript
	// has no registry: a package is linted when it holds an ESLint config.
	Language string
	// Key is the path adoption is looked up under, as the shell resolved it.
	Key string
	// Registry is the file that records adoption: the Go registry, or the
	// .NET props file whose path-scoped Imports double as one.
	Registry string
}

// SpendArgs is argv for the spend form.
type SpendArgs struct{ IDs []string }

// UsageError is a command line lint-changed refuses to guess at. It always
// exits 1: a usage mistake is the tool breaking before it ever reads a
// report, never a finding surviving.
type UsageError struct{ Problem string }

func (e *UsageError) Error() string {
	return "lint-changed: " + e.Problem + "\n\n" + usage
}

const usage = `usage: lint-changed [--staged | --since <ref> | --files <path> ...] [--format <fmt> --report <file> ...] [--matched-waivers <file>] [--accept-spent]
       lint-changed waive --language <lang> [--path <p>] --rule <r> --reason <why>
       lint-changed waivers
       lint-changed spend --waiver <id> [--waiver <id> ...]
       lint-changed changed-paths (--staged | --since <ref> | --files <path> ...)
       lint-changed owners --language <lang> [--files <path> ...]
       lint-changed adopted --language (go | csharp) --registry-key <path> --registry <file>`

// Parse reads argv without the program name. "waive", "waivers", "spend",
// "changed-paths", "owners" and "adopted" as the first argument select those
// six forms; anything else, including no arguments at all, is read as the
// filter form.
func Parse(args []string) (Command, error) {
	if len(args) > 0 && args[0] == "waive" {
		wa, err := parseWaive(args[1:])
		if err != nil {
			return Command{}, err
		}
		return Command{Kind: KindWaive, Waive: wa}, nil
	}
	if len(args) > 0 && args[0] == "waivers" {
		if len(args) > 1 {
			return Command{}, &UsageError{Problem: "waivers takes no arguments"}
		}
		return Command{Kind: KindWaivers}, nil
	}
	if len(args) > 0 && args[0] == "spend" {
		sa, err := parseSpend(args[1:])
		if err != nil {
			return Command{}, err
		}
		return Command{Kind: KindSpend, Spend: sa}, nil
	}
	if len(args) > 0 && args[0] == "changed-paths" {
		scope, err := parseChangedPaths(args[1:])
		if err != nil {
			return Command{}, err
		}
		return Command{Kind: KindChangedPaths, ChangedPaths: scope}, nil
	}
	if len(args) > 0 && args[0] == "owners" {
		oa, err := parseOwners(args[1:])
		if err != nil {
			return Command{}, err
		}
		return Command{Kind: KindOwners, Owners: oa}, nil
	}
	if len(args) > 0 && args[0] == "adopted" {
		aa, err := parseAdopted(args[1:])
		if err != nil {
			return Command{}, err
		}
		return Command{Kind: KindAdopted, Adopted: aa}, nil
	}
	fa, err := parseFilter(args)
	if err != nil {
		return Command{}, err
	}
	return Command{Kind: KindFilter, Filter: fa}, nil
}

func parseFilter(args []string) (FilterArgs, error) {
	var fa FilterArgs
	var scope scopeParser
	// parser is the lintfind.Parser the --format currently in force
	// resolved to, nil until the first --format. --format is sticky: it
	// scopes every --report that follows it, until the next --format, since
	// dotnet emits one report per target framework and go one per module,
	// so N reports under one format is the common case.
	var parser lintfind.Parser

	for i := 0; i < len(args); i++ {
		next, ok, err := scope.flag(args, i)
		if err != nil {
			return FilterArgs{}, err
		}
		if ok {
			i = next
			continue
		}
		switch args[i] {
		case "--format":
			v, next, err := flagValue(args, i, "--format")
			if err != nil {
				return FilterArgs{}, err
			}
			p, err := formatParser(v)
			if err != nil {
				return FilterArgs{}, err
			}
			parser, i = p, next
		case "--report":
			v, next, err := flagValue(args, i, "--report")
			if err != nil {
				return FilterArgs{}, err
			}
			if parser == nil {
				return FilterArgs{}, &UsageError{Problem: "--report " + v + " has no --format before it"}
			}
			fa.Reports, i = append(fa.Reports, Report{Path: v, Parser: parser}), next
		case "--matched-waivers":
			v, next, err := flagValue(args, i, "--matched-waivers")
			if err != nil {
				return FilterArgs{}, err
			}
			fa.MatchedWaivers, i = v, next
		case "--accept-spent":
			fa.AcceptSpent = true
		default:
			return FilterArgs{}, &UsageError{Problem: "unknown argument '" + args[i] + "'"}
		}
	}

	s, err := scope.result()
	if err != nil {
		return FilterArgs{}, err
	}
	fa.Scope = s
	return fa, nil
}

func parseChangedPaths(args []string) (Scope, error) {
	var scope scopeParser
	for i := 0; i < len(args); i++ {
		next, ok, err := scope.flag(args, i)
		if err != nil {
			return Scope{}, err
		}
		if !ok {
			return Scope{}, &UsageError{Problem: "changed-paths: unknown argument '" + args[i] + "'"}
		}
		i = next
	}
	return scope.result()
}

// scopeParser reads the scope flags for both forms that take them, so the
// filter and changed-paths cannot disagree about what one run's scope is.
type scopeParser struct {
	scope Scope
	set   bool
}

// flag consumes args[i] when it is a scope flag, returning the index the
// caller's loop resumes at and whether args[i] was one.
func (p *scopeParser) flag(args []string, i int) (int, bool, error) {
	const oneScope = "only one of --staged, --since, --files may be given"
	switch args[i] {
	case "--staged":
		if p.set {
			return i, true, &UsageError{Problem: oneScope}
		}
		p.set, p.scope.Mode = true, ScopeStaged
		return i, true, nil
	case "--since":
		v, next, err := flagValue(args, i, "--since")
		if err != nil {
			return i, true, err
		}
		if p.set {
			return i, true, &UsageError{Problem: oneScope}
		}
		p.set, p.scope.Mode, p.scope.Ref = true, ScopeSince, v
		return next, true, nil
	case "--files":
		v, next, err := flagValue(args, i, "--files")
		if err != nil {
			return i, true, err
		}
		if p.set && p.scope.Mode != ScopeFiles {
			return i, true, &UsageError{Problem: oneScope}
		}
		p.set, p.scope.Mode, p.scope.Files = true, ScopeFiles, append(p.scope.Files, v)
		return next, true, nil
	default:
		return i, false, nil
	}
}

func (p *scopeParser) result() (Scope, error) {
	if !p.set {
		return Scope{}, &UsageError{Problem: "exactly one of --staged, --since, --files is required"}
	}
	return p.scope, nil
}

// formatParser resolves --format to the lintfind.Parser that reads it. An
// unknown value is a usage error rather than a nil Parser, so a caller that
// misspells --format learns that before the run ever opens a report.
func formatParser(format string) (lintfind.Parser, error) {
	switch format {
	case "sarif":
		return lintfind.ParseSARIF, nil
	case "golangci":
		return lintfind.ParseGolangCI, nil
	case "eslint":
		return lintfind.ParseESLint, nil
	default:
		return nil, &UsageError{Problem: "unknown --format '" + format + "', want one of sarif, golangci, eslint"}
	}
}

func parseWaive(args []string) (WaiveArgs, error) {
	var wa WaiveArgs
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--language":
			v, next, err := flagValue(args, i, "--language")
			if err != nil {
				return WaiveArgs{}, err
			}
			wa.Language, i = v, next
		case "--path":
			v, next, err := flagValue(args, i, "--path")
			if err != nil {
				return WaiveArgs{}, err
			}
			wa.Path, i = v, next
		case "--rule":
			v, next, err := flagValue(args, i, "--rule")
			if err != nil {
				return WaiveArgs{}, err
			}
			wa.Rule, i = v, next
		case "--reason":
			v, next, err := flagValue(args, i, "--reason")
			if err != nil {
				return WaiveArgs{}, err
			}
			wa.Reason, i = v, next
		default:
			return WaiveArgs{}, &UsageError{Problem: "unknown argument '" + args[i] + "'"}
		}
	}
	if wa.Language == "" {
		return WaiveArgs{}, &UsageError{Problem: "waive: --language is required"}
	}
	switch wa.Language {
	case lintfind.LanguageCSharp, lintfind.LanguageGo, lintfind.LanguageTS:
	default:
		return WaiveArgs{}, &UsageError{Problem: "waive: unknown --language '" + wa.Language + "', want one of " +
			lintfind.LanguageCSharp + ", " + lintfind.LanguageGo + ", " + lintfind.LanguageTS}
	}
	// --path is optional, and omitting it is the only route out of a finding
	// that has no location to name: Roslyn reports an analyzer that failed to
	// load at Location.None, so a waiver for it keys on language and rule alone.
	if wa.Rule == "" {
		return WaiveArgs{}, &UsageError{Problem: "waive: --rule is required"}
	}
	if wa.Reason == "" {
		return WaiveArgs{}, &UsageError{Problem: "waive: --reason is required"}
	}
	return wa, nil
}

// parseOwners takes no --files at all as legal, since every changed file of a
// language can be absent from disk and the branch still has to reach its tail.
func parseOwners(args []string) (OwnersArgs, error) {
	var oa OwnersArgs
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--language":
			v, next, err := flagValue(args, i, "--language")
			if err != nil {
				return OwnersArgs{}, err
			}
			oa.Language, i = v, next
		case "--files":
			v, next, err := flagValue(args, i, "--files")
			if err != nil {
				return OwnersArgs{}, err
			}
			oa.Files, i = append(oa.Files, v), next
		default:
			return OwnersArgs{}, &UsageError{Problem: "owners: unknown argument '" + args[i] + "'"}
		}
	}
	switch oa.Language {
	case lintfind.LanguageCSharp, lintfind.LanguageGo, lintfind.LanguageTS:
		return oa, nil
	default:
		return OwnersArgs{}, &UsageError{Problem: "owners: --language '" + oa.Language + "' is not one of " +
			lintfind.LanguageCSharp + ", " + lintfind.LanguageGo + ", " + lintfind.LanguageTS}
	}
}

func parseAdopted(args []string) (AdoptedArgs, error) {
	var aa AdoptedArgs
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--language":
			v, next, err := flagValue(args, i, "--language")
			if err != nil {
				return AdoptedArgs{}, err
			}
			aa.Language, i = v, next
		case "--registry-key":
			v, next, err := flagValue(args, i, "--registry-key")
			if err != nil {
				return AdoptedArgs{}, err
			}
			aa.Key, i = v, next
		case "--registry":
			v, next, err := flagValue(args, i, "--registry")
			if err != nil {
				return AdoptedArgs{}, err
			}
			aa.Registry, i = v, next
		default:
			return AdoptedArgs{}, &UsageError{Problem: "adopted: unknown argument '" + args[i] + "'"}
		}
	}
	switch aa.Language {
	case lintfind.LanguageGo, lintfind.LanguageCSharp:
	default:
		return AdoptedArgs{}, &UsageError{Problem: "adopted: --language must be " +
			lintfind.LanguageGo + " or " + lintfind.LanguageCSharp}
	}
	if aa.Key == "" {
		return AdoptedArgs{}, &UsageError{Problem: "adopted: --registry-key is required"}
	}
	if aa.Registry == "" {
		return AdoptedArgs{}, &UsageError{Problem: "adopted: --registry is required"}
	}
	return aa, nil
}

func parseSpend(args []string) (SpendArgs, error) {
	var sa SpendArgs
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--waiver":
			v, next, err := flagValue(args, i, "--waiver")
			if err != nil {
				return SpendArgs{}, err
			}
			sa.IDs, i = append(sa.IDs, v), next
		default:
			return SpendArgs{}, &UsageError{Problem: "unknown argument '" + args[i] + "'"}
		}
	}
	if len(sa.IDs) == 0 {
		return SpendArgs{}, &UsageError{Problem: "spend: at least one --waiver is required"}
	}
	return sa, nil
}

// flagValue reads the value that follows a flag spelled as two argv
// entries, "--flag value", and returns the index the loop should resume at.
func flagValue(args []string, i int, flag string) (string, int, error) {
	if i+1 >= len(args) {
		return "", i, &UsageError{Problem: flag + " needs a value"}
	}
	return args[i+1], i + 1, nil
}
