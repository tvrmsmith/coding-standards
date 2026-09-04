// Package scope parses the whole of metric-gate's command line, the diff
// scope a run measures and the coverage reports it reads. It depends on
// stdlib alone: nothing here touches git or the filesystem, so a caller can
// validate a command line before it opens a repo.
//
// One parser owns argv. Two of them, each rejecting the other's flags as
// unknown, cannot print a usage block that tells the truth, and the first
// flag a developer adds to either one is a bug in the other.
//
// Parsing is a hand-rolled loop rather than the flag package. flag exits 2 on
// a parse error, and issue 14 fixes exit 1 for a usage mistake; flag also
// lets a positional argument through in silence, where this package refuses
// one outright.
package scope

import "strings"

// Mode is which diff a run measures.
type Mode string

const (
	// ModeMergeBase is the default: the merge base of HEAD and the default
	// branch. There is no flag that spells it; bare argv means this.
	ModeMergeBase Mode = "merge-base"
	// ModeStaged measures what a commit would contain.
	ModeStaged Mode = "staged"
	// ModeSince measures the merge base of HEAD and an explicit ref.
	ModeSince Mode = "since"
	// ModeFiles measures every method in an explicit file list. Parsing
	// recognises it; nothing downstream acts on it yet.
	ModeFiles Mode = "files"
)

// Scope is one parsed command line.
type Scope struct {
	Mode Mode
	// Ref is the argument --since named. Set only when Mode is ModeSince.
	Ref string
	// Files is the argument list --files consumed, in the order the
	// developer typed them. Set only when Mode is ModeFiles.
	Files []string
	// Coverage is every path --coverage named, in the order the developer
	// typed them. Empty means discovery. It is independent of Mode: any
	// scope can be scored against named reports.
	Coverage []string
}

// UsageError is a command line the gate refuses to guess at. Its Error()
// renders the usage block below the specific problem, which is the whole of
// what stderr prints for a usage mistake (ADR 0005: no document exists yet).
type UsageError struct{ Problem string }

func (e *UsageError) Error() string {
	return "metric-gate: " + e.Problem + "\n\n" + usage
}

const usage = `usage: metric-gate [--staged | --since <ref> | --files <path>...] [--coverage <path>]...
  (no flag)          the merge base of HEAD and the default branch
  --staged           what a commit would contain
  --since <ref>      the merge base of HEAD and <ref>
  --files <path>...  every method in each named file
  --coverage <path>  read this report instead of discovering one; repeatable`

// flagName is the human name for a scope flag, used to report a conflict
// between two of them.
func flagName(m Mode) string {
	switch m {
	case ModeStaged:
		return "--staged"
	case ModeSince:
		return "--since"
	case ModeFiles:
		return "--files"
	}
	return string(m)
}

// Parse reads argv without the program name. Bare argv is ModeMergeBase; any
// scope flag beyond the first is a usage error naming the flag seen second
// first, then the flag already set, so the developer reads which one they
// typed twice.
//
// --coverage is repeatable and takes either spelling, "--coverage <path>" or
// "--coverage=<path>". Both reach one value site, so neither can drift into
// accepting the empty value an unset shell variable expands to. A value
// spelled like a flag is refused there too: no producer writes a report whose
// name begins with two dashes, and a mistyped flag read as a path would reach
// the developer as a coverage diagnostic inside a document rather than as the
// usage error it is.
func Parse(args []string) (Scope, error) {
	var sc Scope
	sc.Mode = ModeMergeBase
	set := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--staged":
			if set {
				return Scope{}, conflict(ModeStaged, sc.Mode)
			}
			sc.Mode, set = ModeStaged, true

		case arg == "--since":
			if set {
				return Scope{}, conflict(ModeSince, sc.Mode)
			}
			i++
			if i >= len(args) {
				return Scope{}, &UsageError{Problem: "--since needs a ref"}
			}
			sc.Mode, sc.Ref, set = ModeSince, args[i], true

		case arg == "--files":
			if set {
				return Scope{}, conflict(ModeFiles, sc.Mode)
			}
			var files []string
			for i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				files = append(files, args[i])
			}
			if len(files) == 0 {
				return Scope{}, &UsageError{Problem: "--files needs at least one path"}
			}
			sc.Mode, sc.Files, set = ModeFiles, files, true

		case arg == "--coverage", strings.HasPrefix(arg, "--coverage="):
			value, joined := strings.CutPrefix(arg, "--coverage=")
			if !joined {
				value = ""
				if i+1 < len(args) {
					i++
					value = args[i]
				}
			}
			if value == "" {
				return Scope{}, &UsageError{Problem: "--coverage needs a path"}
			}
			if strings.HasPrefix(value, "--") {
				return Scope{}, &UsageError{Problem: "--coverage needs a path, not the flag '" + value + "'"}
			}
			sc.Coverage = append(sc.Coverage, value)

		case strings.HasPrefix(arg, "-"):
			return Scope{}, &UsageError{Problem: "unknown argument '" + arg + "'"}

		default:
			return Scope{}, &UsageError{Problem: "unexpected argument '" + arg + "'; there are no positional arguments"}
		}
	}
	return sc, nil
}

// conflict reports two scope flags named on the same command line, the one
// just seen first and the one already set second.
func conflict(second, first Mode) error {
	return &UsageError{Problem: flagName(second) + " and " + flagName(first) + " name two scopes; pass one"}
}
