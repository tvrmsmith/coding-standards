// Package comments is the fixture the no-comments analyzer runs against. A package doc comment
// is documentation, so this one is exempt.
package comments

//go:generate echo generating

import (
	"fmt"
	_ "unsafe" // want `Remove this comment`
)

// Documented is a function with a doc comment, which is exempt.
func Documented() {}

// Config shows the struct-level doc and both field forms godoc renders.
type Config struct {
	// Timeout is documented above the field.
	Timeout int
	Retries int // Retries is documented on its own line, which godoc also renders.
}

// Modes documents a const block.
const (
	// Once never retries.
	Once  = iota
	Twice // Twice retries one time.
)

// Celsius documents a type.
type Celsius float64

func suppressed() error {
	return fmt.Errorf("x") //nolint:err113 // the fixture wants a dynamic error
}

func nosec() {
	_ = 0 // #nosec G104 -- fixture
}

func lintIgnore() {
	//lint:ignore SA4006 the fixture assigns on purpose
	x := 1
	_ = x
}

func bareNolint() {
	_ = 0 //nolint
}

func localDecl() int {
	// ttl is documented as a declaration inside the body, which the parser attaches to the var.
	var ttl = 5
	return ttl
}

func narrated() int {
	// want `Remove this comment`
	a := 1
	a++ // want `Remove this comment`
	/* want `Remove this comment` */
	return a
}

func run() {
	// want `Remove this comment`
	// want `Remove this comment`
	_ = 0
}

func commentedOut() {
	// fmt.Println("old") // want `Remove this comment`
	_ = 0
}

func spacedDirective() {
	// go:generate is prose with the space // want `Remove this comment`
	_ = 0
}
