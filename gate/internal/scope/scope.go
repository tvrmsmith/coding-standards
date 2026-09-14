// Package scope parses the whole of metric-gate's command line, the diff
// scope a run measures, the coverage reports it reads, and the metrics it
// selects. It depends on stdlib and the metric catalogue alone: neither
// touches git or the filesystem, so a caller can validate a command line
// before it opens a repo.
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

import (
	"strconv"
	"strings"

	"github.com/tvrmsmith/coding-standards/gate/internal/metric"
)

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
	// ModeFiles measures every method in an explicit file list, so it
	// resolves no base and reads no diff.
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
	// Metrics is every metric the run selected with the threshold in force
	// for it, in the order metric.Hosted lists them. Never empty: no
	// --metric means every metric the binary hosts.
	Metrics []metric.Selection
}

// UsageError is a command line the gate refuses to guess at. Its Error()
// renders the usage block below the specific problem, which is the whole of
// what stderr prints for a usage mistake (ADR 0008: no document exists yet).
type UsageError struct{ Problem string }

func (e *UsageError) Error() string {
	return "metric-gate: " + e.Problem + "\n\n" + usage
}

const usage = `usage: metric-gate [--staged | --since <ref> | --files <path>...] [--coverage <path>]... [--metric <name>]... [--threshold <name>=<n>]...
  (no flag)               the merge base of HEAD and the default branch
  --staged                what a commit would contain
  --since <ref>           the merge base of HEAD and <ref>
  --files <path>...       every method in each named file
  --coverage <path>       read this report instead of discovering one; repeatable
  --metric <name>         measure this metric; repeatable, defaults to every one this binary hosts
  --threshold <name>=<n>  the bar for one metric, as in crap=30; repeatable, never global`

// flagNames is the human name for each scope flag, used to report a conflict
// between two of them. ModeMergeBase is absent on purpose: no flag spells it,
// and a conflict is always between two flags a developer typed. A fifth mode
// added without an entry here renders as the empty string rather than as a
// mode name pretending to be a flag.
var flagNames = map[Mode]string{
	ModeStaged: "--staged",
	ModeSince:  "--since",
	ModeFiles:  "--files",
}

// Parse reads argv without the program name. Bare argv is ModeMergeBase; any
// scope flag beyond the first is a usage error naming the flag seen second
// first, then the flag already set, so the developer reads which one they
// typed twice.
//
// Mode is the whole of that record, because no flag spells the merge-base
// default, so a mode other than it is a flag already taken. A second parallel
// bool would be one more thing a fourth flag could forget to set.
//
// --coverage is repeatable and takes either spelling, "--coverage <path>" or
// "--coverage=<path>". Both reach one value site, so neither can drift into
// accepting the empty value an unset shell variable expands to. A value
// spelled like a flag is refused there too: no producer writes a report whose
// name begins with two dashes, and a mistyped flag read as a path would reach
// the developer as a coverage diagnostic inside a document rather than as the
// usage error it is. The refusal is the two-dash spelling alone, narrower than
// the one --since and --files make: a value beginning with a single dash is
// taken as a path, since the gate's own flags are all long ones and a report
// really can be named that way.
func Parse(args []string) (Scope, error) {
	return parse(args, metric.Hosted())
}

// parse is Parse's work, taking the catalogue as a parameter rather than
// reading metric.Hosted() itself.
//
// The binary hosts CRAP alone, so several of the rules below cannot fire
// through Parse: with one metric hosted, a threshold naming a metric
// --metric did not select is unreachable, since the only name left over is
// the one --metric already took, and "defaults to every metric the binary
// hosts" cannot be told apart from "defaults to crap". Driving parse from
// gate/internal/scope/scope_test.go with a two-entry fake catalogue is what
// makes those rules real tests rather than dead branches.
func parse(args []string, hosted []metric.Definition) (Scope, error) {
	var sc Scope
	sc.Mode = ModeMergeBase

	// selected is every name --metric named, in typed order, used to reject a
	// repeat and, once argv is fully read, to pick which hosted metrics the
	// run measures. Empty means the flag was never typed, which is "every
	// hosted metric" rather than "none": a flag not given is a default, not a
	// negative selection.
	var selected []string
	// thresholds holds one explicit override per metric name, keyed for the
	// twice-check; thresholdOrder keeps the typed order so the
	// did-not-select validation below reports the first offending flag
	// deterministically rather than in map iteration order.
	thresholds := map[string]int{}
	var thresholdOrder []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--staged":
			if sc.Mode != ModeMergeBase {
				return Scope{}, conflict(ModeStaged, sc.Mode)
			}
			sc.Mode = ModeStaged

		case arg == "--since":
			if sc.Mode != ModeMergeBase {
				return Scope{}, conflict(ModeSince, sc.Mode)
			}
			i++
			// The same stop the --files arm makes: an argument starting with
			// '-' is another flag the developer typed, so taking it as a ref
			// would report a commit that does not exist rather than the
			// usage block. An empty argument names no ref either, and taken
			// as one it reaches the caller as the default-candidates message
			// telling them to pass the flag they just passed.
			if i >= len(args) || args[i] == "" || strings.HasPrefix(args[i], "-") {
				return Scope{}, &UsageError{Problem: "--since needs a ref"}
			}
			sc.Mode, sc.Ref = ModeSince, args[i]

		case arg == "--files":
			if sc.Mode != ModeMergeBase {
				return Scope{}, conflict(ModeFiles, sc.Mode)
			}
			files, err := slurpFiles(args, i+1)
			if err != nil {
				return Scope{}, err
			}
			sc.Mode, sc.Files, i = ModeFiles, files, i+len(files)

		case arg == "--coverage", strings.HasPrefix(arg, "--coverage="):
			var value string
			if after, joined := strings.CutPrefix(arg, "--coverage="); joined {
				value = after
			} else if i+1 < len(args) {
				i++
				value = args[i]
			}
			if value == "" {
				return Scope{}, &UsageError{Problem: "--coverage needs a path"}
			}
			if strings.HasPrefix(value, "--") {
				return Scope{}, &UsageError{Problem: "--coverage needs a path, not the flag '" + value + "'"}
			}
			sc.Coverage = append(sc.Coverage, value)

		case arg == "--metric", strings.HasPrefix(arg, "--metric="):
			var value string
			if after, joined := strings.CutPrefix(arg, "--metric="); joined {
				value = after
			} else if i+1 < len(args) {
				i++
				value = args[i]
			}
			if value == "" {
				return Scope{}, &UsageError{Problem: "--metric needs a metric name"}
			}
			// The same two-dash-only refusal --coverage makes, for the same
			// reason: a hosted metric name never begins with a dash, and a
			// mistyped flag taken as a name would reach the developer as an
			// unknown-metric message rather than the usage error it is.
			if strings.HasPrefix(value, "--") {
				return Scope{}, &UsageError{Problem: "--metric needs a metric name, not the flag '" + value + "'"}
			}
			if _, ok := metric.Lookup(hosted, value); !ok {
				return Scope{}, &UsageError{Problem: "unknown metric '" + value + "'; this binary hosts: " + strings.Join(metric.Names(hosted), ", ")}
			}
			for _, s := range selected {
				if s == value {
					return Scope{}, &UsageError{Problem: "--metric was given '" + value + "' twice; pass it once"}
				}
			}
			selected = append(selected, value)

		case arg == "--threshold", strings.HasPrefix(arg, "--threshold="):
			var value string
			if after, joined := strings.CutPrefix(arg, "--threshold="); joined {
				value = after
			} else if i+1 < len(args) {
				i++
				value = args[i]
			}
			if value == "" {
				return Scope{}, &UsageError{Problem: "--threshold needs a <metric>=<value> assignment, as in 'crap=30'"}
			}
			if strings.HasPrefix(value, "--") {
				return Scope{}, &UsageError{Problem: "--threshold needs a <metric>=<value> assignment, not the flag '" + value + "'"}
			}
			name, numStr, hasEquals := strings.Cut(value, "=")
			if !hasEquals {
				return Scope{}, &UsageError{Problem: "--threshold '" + value + "' is missing '=', as in 'crap=30'"}
			}
			if name == "" {
				return Scope{}, &UsageError{Problem: "--threshold '" + value + "' names no metric, as in 'crap=30'"}
			}
			if _, ok := metric.Lookup(hosted, name); !ok {
				return Scope{}, &UsageError{Problem: "--threshold names unknown metric '" + name + "'; this binary hosts: " + strings.Join(metric.Names(hosted), ", ")}
			}
			n, err := strconv.Atoi(numStr)
			if err != nil {
				return Scope{}, &UsageError{Problem: "--threshold '" + value + "' is not a whole number, as in 'crap=30'"}
			}
			// Below 1 no method can pass: complexity 1 fully covered scores 1,
			// the floor of crap.Measurement.Score, so a threshold under it
			// would fail every run outright rather than set a bar.
			if n < 1 {
				return Scope{}, &UsageError{Problem: "--threshold " + value + " is below 1, so no method could pass"}
			}
			if _, ok := thresholds[name]; ok {
				return Scope{}, &UsageError{Problem: "--threshold was given " + name + " twice; pass it once"}
			}
			thresholds[name] = n
			thresholdOrder = append(thresholdOrder, name)

		case strings.HasPrefix(arg, "-"):
			return Scope{}, &UsageError{Problem: "unknown argument '" + arg + "'"}

		default:
			return Scope{}, &UsageError{Problem: "unexpected argument '" + arg + "'; there are no positional arguments"}
		}
	}

	// Resolved after the whole of argv is read, so --metric and --threshold
	// may be typed in either order: an empty selection means every hosted
	// metric, and only then can "a threshold named a metric that was not
	// selected" be told apart from "no --metric was typed at all".
	var names []string
	if len(selected) == 0 {
		names = metric.Names(hosted)
	} else {
		chosen := make(map[string]bool, len(selected))
		for _, name := range selected {
			chosen[name] = true
		}
		for _, def := range hosted {
			if chosen[def.Name] {
				names = append(names, def.Name)
			}
		}
	}

	final := make(map[string]bool, len(names))
	for _, name := range names {
		final[name] = true
	}
	for _, name := range thresholdOrder {
		if !final[name] {
			return Scope{}, &UsageError{Problem: "--threshold names " + name + ", which --metric did not select"}
		}
	}

	// Order follows hosted, never the order --metric was typed in, so the
	// document does not vary with argv ordering (issue 19).
	sc.Metrics = make([]metric.Selection, 0, len(names))
	for _, name := range names {
		def, _ := metric.Lookup(hosted, name) // name came from hosted itself, so this always finds it
		threshold := def.DefaultThreshold
		if t, ok := thresholds[name]; ok {
			threshold = t
		}
		sc.Metrics = append(sc.Metrics, metric.Selection{Definition: def, Threshold: threshold})
	}

	return sc, nil
}

// slurpFiles takes --files' variadic argument list out of args starting at
// from. It appends exactly one entry per argument it consumes, so the caller
// finds the first argument it did not take at from+len(files).
//
// The slurp stops at any argument beginning with '-', so `--files a.cs
// --staged` is caught as two scopes rather than measuring a file named
// --staged.
func slurpFiles(args []string, from int) ([]string, error) {
	var files []string
	for i := from; i < len(args) && !strings.HasPrefix(args[i], "-"); i++ {
		// An empty argument does not start with '-', so the slurp takes it,
		// and it names no file. Resolved it is the working directory itself,
		// so the run would exit 1 saying an empty name is a directory and the
		// reader would see a message naming nothing.
		if args[i] == "" {
			return nil, &UsageError{Problem: "--files was handed an empty path"}
		}
		files = append(files, args[i])
	}
	if len(files) == 0 {
		return nil, &UsageError{Problem: "--files needs at least one path"}
	}
	return files, nil
}

// conflict reports two scope flags named on the same command line, the one
// just seen first and the one already set second.
//
// One flag typed twice is a different mistake from two flags naming two
// scopes, and the two-scopes sentence contradicts itself when both halves are
// the same flag. A repetition gets a sentence about the repetition instead.
//
// A second --files keeps its own wording. The flag is variadic rather than
// repeatable, so what the developer needs is not "pass it once" but where the
// paths go.
func conflict(second, first Mode) error {
	if second == first {
		if second == ModeFiles {
			return &UsageError{Problem: "--files takes every path in one list, as in 'metric-gate --files a.cs b.cs'"}
		}
		return &UsageError{Problem: flagNames[second] + " was passed twice; pass it once"}
	}
	return &UsageError{Problem: flagNames[second] + " and " + flagNames[first] + " name two scopes; pass one"}
}
