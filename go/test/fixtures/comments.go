package fixtures

// Comments carries the two comment fixtures. They are in their own file because a `//nolint`
// directive suppresses findings for the rest of its line or block, and a stray one in the file
// every other fixture lives in would be silencing the fixtures beside it.
func Comments() int {
	// The budget is ten lines and this run is eleven, which is the whole fixture.
	// The rule exists because a paragraph of prose in the middle of a function is almost
	// always justifying something the code should be doing differently, so the report asks
	// you to re-read the code rather than to trim the comment.
	// A doc comment is exempt at any length, which is why this block sits inside a function
	// body rather than above the declaration: above it, the parser would hand it to the
	// declaration as documentation and the rule would leave it alone.
	// That difference is the Go-specific half of the rule, and go/README.md records it
	// alongside the other one, the region above the package clause.
	// Line ten.
	// Line eleven.
	return 0
}

// Nolint suppresses a finding without saying which linter or why.
func Nolint() int {
	count := 1
	count = 2 //nolint
	return count
}
