//go:build unix

package gitscope

import (
	"syscall"
	"testing"
)

// e2bigTarget is how much argv one invocation has to build before exec refuses
// it on the host running the case, plus a budget's slack, since argv and the
// environment share the ceiling on Linux. Batched, the same list is
// target/divergenceBudget invocations, a size no platform refuses.
//
// The two unix ceilings do not agree and neither is safe alone. macOS caps argv
// at a fixed 1 MiB that RLIMIT_STACK has no bearing on, so the Linux formula
// under `ulimit -s 512` would answer 128 KiB, an argv macOS execs happily, and
// the case would pass against exactly the unbatched code it exists to refuse.
// Linux caps it at max(min(6 MiB, RLIMIT_STACK/4), 128 KiB), so a container
// with a large or unlimited stack accepts far more than the 1 MiB macOS stops
// at, and a target written down as a constant would exec fine there. The
// derived figure is therefore floored at macOS's fixed cap, which is above the
// Linux 128 KiB floor and so subsumes it: whichever host runs the case, the
// target is past what that host's exec will take.
//
// It is read live rather than written down for the same reason, and it is a
// test-only figure. divergenceBudget stays a fixed number on every platform, so
// two machines batch identically.
func e2bigTarget(t *testing.T) int {
	t.Helper()
	var stack syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_STACK, &stack); err != nil {
		t.Fatal(err)
	}
	const darwinArgMax = 1 << 20
	return int(max(min(uint64(6<<20), stack.Cur/4), darwinArgMax)) + divergenceBudget
}
