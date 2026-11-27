package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

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

// TestWriteLeavesFileWithNULUntouched checks that `pgfmt -w` exits
// nonzero and does not modify a file that contains a NUL byte.
func TestWriteLeavesFileWithNULUntouched(t *testing.T) {
	is := is.New(t)
	bin := filepath.Join(t.TempDir(), "pgfmt")
	build := exec.Command("go", "build", "-o", bin, ".")
	out, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	const input = "select 1; -- note\x00\nselect 2;\n"
	path := filepath.Join(t.TempDir(), "nul.sql")
	is.NoErr(os.WriteFile(path, []byte(input), 0o644))

	err = exec.Command(bin, "-w", path).Run()
	is.HasErr(err)

	got, err := os.ReadFile(path)
	is.NoErr(err)
	is.Eq(string(got), input)
}

// TestWriteLeavesCopyFromStdoutUntouched checks that `pgfmt -w` exits
// nonzero and does not modify a file that holds COPY ... FROM STDOUT.
func TestWriteLeavesCopyFromStdoutUntouched(t *testing.T) {
	is := is.New(t)
	bin := filepath.Join(t.TempDir(), "pgfmt")
	build := exec.Command("go", "build", "-o", bin, ".")
	out, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	const input = "COPY t FROM STDOUT; SELECT 1;\n"
	path := filepath.Join(t.TempDir(), "copy.sql")
	is.NoErr(os.WriteFile(path, []byte(input), 0o644))

	err = exec.Command(bin, "-w", path).Run()
	is.HasErr(err)

	got, err := os.ReadFile(path)
	is.NoErr(err)
	is.Eq(string(got), input)
}

// TestWriteLeavesCopyDottedToFromStdinUntouched checks that `pgfmt -w`
// exits nonzero and does not modify a file that holds COPY s.to FROM STDIN.
func TestWriteLeavesCopyDottedToFromStdinUntouched(t *testing.T) {
	is := is.New(t)
	bin := filepath.Join(t.TempDir(), "pgfmt")
	build := exec.Command("go", "build", "-o", bin, ".")
	out, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	const input = "COPY s.to FROM STDIN; SELECT 1;\n"
	path := filepath.Join(t.TempDir(), "copy.sql")
	is.NoErr(os.WriteFile(path, []byte(input), 0o644))

	err = exec.Command(bin, "-w", path).Run()
	is.HasErr(err)

	got, err := os.ReadFile(path)
	is.NoErr(err)
	is.Eq(string(got), input)
}

// TestWriteRefusesSymlink checks that pgfmt refuses a symbolic link in
// stdout, -c, and -w modes and leaves the target file outside the
// directory untouched.
func TestWriteRefusesSymlink(t *testing.T) {
	is := is.New(t)
	bin := filepath.Join(t.TempDir(), "pgfmt")
	build := exec.Command("go", "build", "-o", bin, ".")
	out, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	const input = "select 1;\n"
	target := filepath.Join(t.TempDir(), "outside.sql")
	is.NoErr(os.WriteFile(target, []byte(input), 0o644))

	link := filepath.Join(t.TempDir(), "link.sql")
	is.NoErr(os.Symlink(target, link))

	for _, args := range [][]string{{link}, {"-c", link}, {"-w", link}} {
		cmd := exec.Command(bin, args...)
		stdout, err := cmd.Output()
		is.HasErr(err)
		is.Eq(string(stdout), "")
	}

	got, err := os.ReadFile(target)
	is.NoErr(err)
	is.Eq(string(got), input)
}

// TestRefusesCharacterDevice checks that pgfmt refuses a character
// device and a symlink to one promptly in every mode.
func TestRefusesCharacterDevice(t *testing.T) {
	is := is.New(t)
	if _, err := os.Stat("/dev/zero"); err != nil {
		t.Skip("/dev/zero not available")
	}

	bin := filepath.Join(t.TempDir(), "pgfmt")
	build := exec.Command("go", "build", "-o", bin, ".")
	out, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	link := filepath.Join(t.TempDir(), "zero.sql")
	is.NoErr(os.Symlink("/dev/zero", link))

	for _, path := range []string{"/dev/zero", link} {
		for _, args := range [][]string{{path}, {"-c", path}, {"-w", path}} {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			cmd := exec.CommandContext(ctx, bin, args...)
			err := cmd.Run()
			is.NoErr(ctx.Err())
			cancel()
			is.HasErr(err)
		}
	}
}

// TestWriteFormatsRegularFile checks that pgfmt -w rewrites a regular
// file in place.
func TestWriteFormatsRegularFile(t *testing.T) {
	is := is.New(t)
	bin := filepath.Join(t.TempDir(), "pgfmt")
	build := exec.Command("go", "build", "-o", bin, ".")
	out, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	path := filepath.Join(t.TempDir(), "in.sql")
	is.NoErr(os.WriteFile(path, []byte("select 1;\n"), 0o644))

	is.NoErr(exec.Command(bin, "-w", path).Run())

	got, err := os.ReadFile(path)
	is.NoErr(err)
	is.Eq(string(got), "SELECT\n  1;\n")
}
