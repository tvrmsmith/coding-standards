// Command lint-changed is the blocking half of a pre-commit lint gate. See
// argv.go for the seven forms it takes and the exit codes each returns.
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	cmd, err := Parse(os.Args[1:])
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(run(cmd, os.Stdout, os.Stderr))
}

func run(cmd Command, stdout, stderr io.Writer) int {
	switch cmd.Kind {
	case KindFilter:
		return runFilter(cmd.Filter, stdout, stderr)
	case KindWaive:
		return runWaive(cmd.Waive, stdout, stderr)
	case KindWaivers:
		return runWaivers(stdout, stderr)
	case KindSpend:
		return runSpend(cmd.Spend, stdout, stderr)
	case KindChangedPaths:
		return runChangedPaths(cmd.ChangedPaths, stdout, stderr)
	case KindOwners:
		return runOwners(cmd.Owners, stdout, stderr)
	case KindAdopted:
		return runAdopted(cmd.Adopted, stdout, stderr)
	default:
		_, _ = fmt.Fprintln(stderr, "lint-changed: internal error: unknown command kind")
		return 1
	}
}
