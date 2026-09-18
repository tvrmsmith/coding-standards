// Command lint-changed is the blocking half of a pre-commit lint gate. See
// argv.go for the three forms it takes and the exit codes each returns.
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	cmd, err := Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(run(cmd, os.Stdin, os.Stdout, os.Stderr))
}

func run(cmd Command, stdin io.Reader, stdout, stderr io.Writer) int {
	switch cmd.Kind {
	case KindFilter:
		return runFilter(cmd.Filter, stdin, stdout, stderr)
	case KindWaive:
		return runWaive(cmd.Waive, stdout, stderr)
	case KindWaivers:
		return runWaivers(stdout, stderr)
	default:
		fmt.Fprintln(stderr, "lint-changed: internal error: unknown command kind")
		return 1
	}
}
