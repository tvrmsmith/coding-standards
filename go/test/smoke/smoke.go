// Package smoke is the fixture build.sh lints to prove the plugin is linked into the binary
// rather than merely that the build exited zero.
package smoke

// Reported is documented at length, and a doc comment is exempt however long it runs, so a
// report on these lines would mean the exemption broke.
// Line three.
// Line four.
// Line five.
// Line six.
// Line seven.
// Line eight.
// Line nine.
// Line ten.
// Line eleven.
func Reported() int {
	// The eleven lines below are the block the run has to find. They are prose justifying the
	// constant underneath, which is the shape the guideline names: a paragraph arguing for code
	// is a paragraph asking for the code to be fixed instead. Everything about this block is
	// deliberate — it starts its own lines, it has no blank line in it, it is not attached to a
	// declaration, and it runs one line past the ten-line budget. Change any one of those and
	// the smoke check stops proving anything, which is the failure mode a fixture this small
	// exists to avoid. Line seven. Line eight follows, and then two more, and then the code it
	// is pretending to justify, which needs no justification at all and never did.
	// Line nine.
	// Line ten.
	// Line eleven.
	return 42
}
