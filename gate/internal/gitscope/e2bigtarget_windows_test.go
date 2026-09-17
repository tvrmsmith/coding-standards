package gitscope

import "testing"

// e2bigTarget on Windows is the CreateProcessW ceiling plus a budget's slack.
// There is no rlimit to read: lpCommandLine is capped at 32767 characters for
// the whole command line, flags and executable path included, so a pathspec
// list past that cannot launch whatever the stack is set to. The unix build
// derives its target from RLIMIT_STACK, which is why the two live in separate
// files rather than behind a runtime.GOOS switch: syscall.Getrlimit and
// syscall.RLIMIT_STACK do not exist here, and a single file naming them fails
// to build under GOOS=windows, a target build.sh ships.
func e2bigTarget(t *testing.T) int {
	t.Helper()
	const createProcessCommandLineMax = 32767
	return createProcessCommandLineMax + divergenceBudget
}
