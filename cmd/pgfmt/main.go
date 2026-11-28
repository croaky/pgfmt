// pgfmt formats Postgres SQL files in one fixed style.
//
// Usage:
//
//	pgfmt -c file.sql ...       Check formatting; exit 1 if any file would
//	                            change. Writes nothing.
//	pgfmt -w file.sql ...       Format named files, writing in place.
//	pgfmt file.sql ...          Format named files to stdout.
//	pgfmt < input.sql           Format stdin to stdout.
//
// -c reports only true formatting violations, never your edits, so a clean
// working tree is not required. -c and -w are mutually exclusive.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/croaky/pgfmt"
)

func main() {
	write := flag.Bool("w", false, "write result back to the file")
	check := flag.Bool("c", false, "check formatting; exit 1 if any file would change (writes nothing)")
	flag.Parse()

	if *write && *check {
		fmt.Fprintln(os.Stderr, "pgfmt: -c and -w are mutually exclusive")
		os.Exit(2)
	}

	args := flag.Args()

	if len(args) == 0 {
		// stdin → stdout.
		src, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "pgfmt:", err)
			os.Exit(2)
		}
		out, err := pgfmt.Format(string(src))
		if err != nil {
			fmt.Fprintln(os.Stderr, "pgfmt:", err)
			os.Exit(2)
		}
		io.WriteString(os.Stdout, out)
		return
	}

	status := 0
	for _, path := range args {
		code := formatFile(path, *check, *write)
		if code > status {
			status = code
		}
	}
	os.Exit(status)
}

func formatFile(path string, check, write bool) int {
	f, info, err := openRegular(path, os.O_RDONLY)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pgfmt:", err)
		return 2
	}
	defer f.Close()

	src, err := readBounded(f, info, path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pgfmt:", err)
		return 2
	}
	out, err := pgfmt.Format(string(src))
	if err != nil {
		fmt.Fprintf(os.Stderr, "pgfmt: %s: %v\n", path, err)
		return 2
	}
	// Without a flag, stdout is the output, so a file already in the
	// style prints as it is: `pgfmt in.sql > out.sql` must never
	// leave out.sql empty. Only -c and -w have nothing to do for it.
	if !check && !write {
		io.WriteString(os.Stdout, out)
		return 0
	}
	if out == string(src) {
		return 0
	}
	if check {
		fmt.Fprintf(os.Stderr, "FAIL: %s needs formatting. Run: pgfmt -w %s\n", path, path)
		return 1
	}
	if err := writeSameRegular(path, info, out); err != nil {
		fmt.Fprintln(os.Stderr, "pgfmt:", err)
		return 2
	}
	fmt.Printf("Formatted %s\n", path)
	return 0
}

func readBounded(f *os.File, info os.FileInfo, path string) ([]byte, error) {
	size := info.Size()
	if size < 0 {
		return nil, fmt.Errorf("%s: invalid file size", path)
	}
	src, err := io.ReadAll(io.LimitReader(f, size+1))
	if err != nil {
		return nil, err
	}
	if int64(len(src)) > size {
		return nil, fmt.Errorf("%s: file grew while reading", path)
	}
	return src, nil
}

// writeSameRegular rewrites path only if it still refers to the same
// regular file as readInfo, without following a symbolic link.
func writeSameRegular(path string, readInfo os.FileInfo, out string) error {
	wf, winfo, err := openRegular(path, os.O_WRONLY)
	if err != nil {
		return err
	}
	if !os.SameFile(readInfo, winfo) {
		wf.Close()
		return fmt.Errorf("%s: file changed before write", path)
	}
	if err := wf.Truncate(0); err != nil {
		wf.Close()
		return err
	}
	if _, err := io.WriteString(wf, out); err != nil {
		wf.Close()
		return err
	}
	return wf.Close()
}

// openRegular opens path as a regular file without following a trailing
// symbolic link. When path lies inside the working directory, it opens
// through os.Root so no symlinked parent directory can escape the tree.
func openRegular(path string, flag int) (*os.File, os.FileInfo, error) {
	if rootDir, rel, ok := treeRelative(path); ok {
		return openRegularInRoot(rootDir, rel, path, flag)
	}
	return openRegularDirect(path, flag)
}

// treeRelative reports whether path lies inside the current working
// directory and returns the root directory and relative path.
func treeRelative(path string) (rootDir, rel string, ok bool) {
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		if filepath.IsLocal(clean) {
			return ".", clean, true
		}
		return "", "", false
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", false
	}
	if r, err := filepath.Rel(cwd, clean); err == nil && filepath.IsLocal(r) {
		return cwd, r, true
	}
	cwdInfo, err := os.Stat(cwd)
	if err != nil {
		return "", "", false
	}
	// Match the shortest ancestor of clean that refers to cwd so system
	// symlinks above cwd (such as /var -> /private/var on macOS) still
	// confine the remainder of the path to cwd.
	matched := ""
	for dir := filepath.Dir(clean); ; {
		if info, err := os.Stat(dir); err == nil && os.SameFile(cwdInfo, info) {
			matched = dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if matched == "" {
		return "", "", false
	}
	r, err := filepath.Rel(matched, clean)
	if err != nil || !filepath.IsLocal(r) {
		return "", "", false
	}
	return cwd, r, true
}

// openRegularInRoot opens rel inside rootDir through os.Root so no
// parent directory symbolic link can escape rootDir, refuses a trailing
// symbolic link, and verifies that the open file is regular.
func openRegularInRoot(rootDir, rel, path string, flag int) (*os.File, os.FileInfo, error) {
	root, err := os.OpenRoot(rootDir)
	if err != nil {
		return nil, nil, err
	}
	defer root.Close()

	linfo, err := root.Lstat(rel)
	if err != nil {
		return nil, nil, err
	}
	if !linfo.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("%s: not a regular file", path)
	}
	f, err := root.OpenFile(rel, openRegularFlags(flag), 0)
	if err != nil {
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	if !info.Mode().IsRegular() || !os.SameFile(linfo, info) {
		f.Close()
		return nil, nil, fmt.Errorf("%s: not a regular file", path)
	}
	return f, info, nil
}
