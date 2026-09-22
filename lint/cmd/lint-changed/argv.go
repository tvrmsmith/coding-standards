// Command lint-changed reads a linter report on stdin, keeps only the
// findings that touch lines the commit changed, lets a one-shot waiver
// suppress one of them, and exits non-zero if any survive. It is the
// blocking half of a pre-commit lint gate.
//
// argv is parsed by hand rather than through the flag package, for the
// reason gate/internal/scope gives: flag exits 2 on a usage mistake and lets
// a stray positional argument through in silence, and this binary's exit
// codes are a one-way door (0 clean, 1 broken, 2 a finding survived) that a
// usage mistake must never collapse into the last of the three.
package main

import (
	"strings"

	"github.com/tvrmsmith/coding-standards/lint/internal/lintfind"
)

// Kind is which of lint-changed's three forms argv named.
type Kind int

const (
	// KindFilter reads a report on stdin and scopes it to the diff. No word
	// on the command line spells it; anything not "waive" or "waivers"
	// means this.
	KindFilter Kind = iota
	// KindWaive records a one-shot waiver.
	KindWaive
	// KindWaivers lists every recorded waiver.
	KindWaivers
)

// ScopeMode is which base a filter run scopes its findings against.
type ScopeMode int

const (
	ScopeStaged ScopeMode = iota
	ScopeSince
	ScopeFiles
)

// Command is one parsed invocation.
type Command struct {
	Kind   Kind
	Filter FilterArgs
	Waive  WaiveArgs
}

// FilterArgs is argv for the filter form.
type FilterArgs struct {
	Format   string
	Language string
	Mode     ScopeMode
	// Ref is the argument --since named. Set only when Mode is ScopeSince.
	Ref string
	// Files is --files' comma-separated list, split. Set only when Mode is
	// ScopeFiles.
	Files []string
	// Reports is every --report the caller gave. Empty means the report comes
	// in on stdin instead. One run reads them all, because a waiver is one
	// commit's worth of permission and a process per report would spend it on
	// whichever report happened to come first.
	Reports []string
	// Parser is the lintfind.Parser --format resolved to. It is resolved at
	// parse time, alongside every other usage mistake, rather than at read
	// time, so an unknown --format is a usage error and not a report-reading
	// failure blamed on whichever report happened to come first.
	Parser lintfind.Parser
}

// WaiveArgs is argv for the waive form.
type WaiveArgs struct {
	Language string
	Path     string
	Rule     string
	Reason   string
}

// UsageError is a command line lint-changed refuses to guess at. It always
// exits 1: a usage mistake is the tool breaking before it ever reads a
// report, never a finding surviving.
type UsageError struct{ Problem string }

func (e *UsageError) Error() string {
	return "lint-changed: " + e.Problem + "\n\n" + usage
}

const usage = `usage: lint-changed --format <fmt> --language <lang> [--staged | --since <ref> | --files <a,b,...>] [--report <file> ...]
       lint-changed waive --language <lang> [--path <p>] --rule <r> --reason <why>
       lint-changed waivers`

// Parse reads argv without the program name. "waive" and "waivers" as the
// first argument select those two forms; anything else, including no
// arguments at all, is read as the filter form.
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
	fa, err := parseFilter(args)
	if err != nil {
		return Command{}, err
	}
	return Command{Kind: KindFilter, Filter: fa}, nil
}

func parseFilter(args []string) (FilterArgs, error) {
	var fa FilterArgs
	modeSet := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--format":
			v, next, err := flagValue(args, i, "--format")
			if err != nil {
				return FilterArgs{}, err
			}
			fa.Format, i = v, next
		case "--language":
			v, next, err := flagValue(args, i, "--language")
			if err != nil {
				return FilterArgs{}, err
			}
			fa.Language, i = v, next
		case "--staged":
			if modeSet {
				return FilterArgs{}, &UsageError{Problem: "only one of --staged, --since, --files may be given"}
			}
			modeSet, fa.Mode = true, ScopeStaged
		case "--since":
			v, next, err := flagValue(args, i, "--since")
			if err != nil {
				return FilterArgs{}, err
			}
			if modeSet {
				return FilterArgs{}, &UsageError{Problem: "only one of --staged, --since, --files may be given"}
			}
			modeSet, fa.Mode, fa.Ref, i = true, ScopeSince, v, next
		case "--files":
			v, next, err := flagValue(args, i, "--files")
			if err != nil {
				return FilterArgs{}, err
			}
			if modeSet {
				return FilterArgs{}, &UsageError{Problem: "only one of --staged, --since, --files may be given"}
			}
			modeSet, fa.Mode, fa.Files, i = true, ScopeFiles, strings.Split(v, ","), next
		case "--report":
			v, next, err := flagValue(args, i, "--report")
			if err != nil {
				return FilterArgs{}, err
			}
			fa.Reports, i = append(fa.Reports, v), next
		default:
			return FilterArgs{}, &UsageError{Problem: "unknown argument '" + args[i] + "'"}
		}
	}

	if fa.Format == "" {
		return FilterArgs{}, &UsageError{Problem: "--format is required"}
	}
	parser, err := formatParser(fa.Format)
	if err != nil {
		return FilterArgs{}, err
	}
	fa.Parser = parser
	if fa.Language == "" {
		return FilterArgs{}, &UsageError{Problem: "--language is required"}
	}
	if !modeSet {
		return FilterArgs{}, &UsageError{Problem: "exactly one of --staged, --since, --files is required"}
	}
	return fa, nil
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

// flagValue reads the value that follows a flag spelled as two argv
// entries, "--flag value", and returns the index the loop should resume at.
func flagValue(args []string, i int, flag string) (string, int, error) {
	if i+1 >= len(args) {
		return "", i, &UsageError{Problem: flag + " needs a value"}
	}
	return args[i+1], i + 1, nil
}
