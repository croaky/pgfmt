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
		src, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "pgfmt:", err)
			status = 2
			continue
		}
		out, err := pgfmt.Format(string(src))
		if err != nil {
			fmt.Fprintf(os.Stderr, "pgfmt: %s: %v\n", path, err)
			status = 2
			continue
		}
		if out == string(src) {
			continue
		}
		if *check {
			fmt.Fprintf(os.Stderr, "FAIL: %s needs formatting. Run: pgfmt -w %s\n", path, path)
			if status < 1 {
				status = 1
			}
			continue
		}
		if *write {
			if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
				fmt.Fprintln(os.Stderr, "pgfmt:", err)
				status = 2
				continue
			}
			fmt.Printf("Formatted %s\n", path)
		} else {
			io.WriteString(os.Stdout, out)
		}
	}
	os.Exit(status)
}
