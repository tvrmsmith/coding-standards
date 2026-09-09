//go:build unix

package gate_test

import (
	"path/filepath"
	"syscall"
	"testing"
)

func TestFilesNamingAFifoRefusesItWithoutCallingItADirectory(t *testing.T) {
	const pipe = "src/Ordering/Pipe.cs"

	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	if err := syscall.Mkfifo(filepath.Join(f.root, filepath.FromSlash(pipe)), 0o644); err != nil {
		t.Skipf("the filesystem does not allow a fifo: %v", err)
	}

	// Neither a regular file nor a directory. No extractor can read it, so the
	// run still refuses, but named as a directory the message would describe a
	// mistake the developer did not make.
	f.runArgs("--files", pipe).assertMatches(t, "files_not_regular", 1, "",
		"src/Ordering/Pipe.cs is not a regular file\n")
}
