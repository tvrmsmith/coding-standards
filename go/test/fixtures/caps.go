package fixtures

// CapsFixture is deliberately repetitive, and the repetition is the point.
//
// golangci-lint caps its output twice by default: 50 findings per linter
// (`max-issues-per-linter`) and 3 sharing a message (`max-same-issues`). Both drop silently, and
// harness/lint-changed.sh's Go branch filters package output down to the changed files *after* the cap
// has been applied, so a capped run reports an arbitrary subset that changes from run to run.
// golangci.yml sets both to 0, and these calls are what proves it: every one is the same
// errcheck message, and there are more of them than either cap allows.
//
// Exported so it does not become a second `unused` fixture.
func CapsFixture() {
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
	MayFail()
}
