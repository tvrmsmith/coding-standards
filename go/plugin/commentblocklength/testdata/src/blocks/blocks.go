// Copyright header, standing in for the licence block a real repository carries above its
// package clause. It runs well past the budget on purpose: this region holds licences and
// //go:build constraints, neither of which justifies any code, and the rule exempts all of it.
// Line four.
// Line five.
// Line six.
// Line seven.
// Line eight.
// Line nine.
// Line ten.
// Line eleven.
// Line twelve.

// Package blocks is the fixture the comment-block-length analyzer runs against.
package blocks

// LongDoc is documented at length on purpose. A doc comment is exempt however long it runs,
// because the same guideline that caps a justification demands a documented contract.
// Line three.
// Line four.
// Line five.
// Line six.
// Line seven.
// Line eight.
// Line nine.
// Line ten.
// Line eleven.
// Line twelve.
func LongDoc() {}

func justifying() {
	// want `11-line comment block, over the 10-line budget`
	// The retry loop below cannot use the shared backoff helper. That helper reads its ceiling
	// from the config object, and the config object is not populated yet at this point in
	// startup, because the config loader itself goes through this client. So the ceiling is
	// inlined here instead. If you move the config load earlier, which was attempted in an
	// earlier revision, the credential provider breaks instead, because it also reads config,
	// and it is constructed before the loader. The ordering constraint is real but undocumented
	// elsewhere, so it is written out here. Someone should probably restructure startup so the
	// config load does not depend on the client that depends on the config, but that is a
	// larger change than this fix needed, and the inline ceiling is correct in the meantime.
	// Note that the value must stay under the gateway's own timeout.
	for attempt := 0; attempt < 5; attempt++ {
		_ = attempt
	}
}

func paragraphs() {
	// Six lines of prose, then a blank line, then six more. The author put the gap there, so
	// these are two comments rather than one twelve-line block, and neither is over budget.
	// Line three.
	// Line four.
	// Line five.
	// Line six.

	// Line one of the second paragraph.
	// Line two.
	// Line three.
	// Line four.
	// Line five.
	// Line six.
	_ = 0
}

func annotated() {
	a := 1  // a trailing comment annotates the code on its line
	a++     // so a run of them measures the code, not the comment
	a += 2  // and never forms a block
	a += 3  // line four
	a += 4  // line five
	a += 5  // line six
	a += 6  // line seven
	a += 7  // line eight
	a += 8  // line nine
	a += 9  // line ten
	a += 10 // line eleven
	_ = a
}

func interrupted() {
	// Five lines of prose, then a line of code, then five more. Code ends a block, so this is
	// two blocks of five rather than one of eleven.
	// Line three.
	// Line four.
	// Line five.
	_ = 0
	// Line one after the code.
	// Line two.
	// Line three.
	// Line four.
	// Line five.
	_ = 1
}
