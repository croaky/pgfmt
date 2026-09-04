package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/croaky/is"
)

// TestStdoutPrintsAFormattedFile pins the no-flag path: stdout is the
// output, so a file already in the style still prints. Before this
// test the command skipped such a file, and `pgfmt in.sql > out.sql`
// left out.sql empty.
func TestStdoutPrintsAFormattedFile(t *testing.T) {
	is := is.New(t)
	bin := filepath.Join(t.TempDir(), "pgfmt")
	build := exec.Command("go", "build", "-o", bin, ".")
	out, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	const formatted = "SELECT\n  1;\n"
	path := filepath.Join(t.TempDir(), "in.sql")
	is.NoErr(os.WriteFile(path, []byte(formatted), 0o644))

	got, err := exec.Command(bin, path).Output()
	is.NoErr(err)
	is.Eq(string(got), formatted)

	// -c and -w still have nothing to say about it.
	got, err = exec.Command(bin, "-c", path).CombinedOutput()
	is.NoErr(err)
	is.Eq(string(got), "")

	got, err = exec.Command(bin, "-w", path).CombinedOutput()
	is.NoErr(err)
	is.Eq(string(got), "")
}
