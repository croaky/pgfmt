// Package pgfmt formats Postgres SQL files in one fixed style.
//
// Pipeline: lex (lex.go) -> buildTree (tree.go) -> emit (emit.go);
// clause.go splits statements into clauses. Format is the entry point.
package pgfmt

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Format formats SQL source per the fixed style.
func Format(src string) (string, error) {
	if i := strings.IndexByte(src, 0); i >= 0 {
		return "", fmt.Errorf("pgfmt: NUL byte at offset %d", i)
	}
	if !utf8.ValidString(src) {
		for i := 0; i < len(src); {
			r, size := utf8.DecodeRuneInString(src[i:])
			if r == utf8.RuneError && size == 1 {
				return "", fmt.Errorf("pgfmt: invalid UTF-8 at offset %d", i)
			}
			i += size
		}
	}
	// The input holds about one token for each 4 bytes. A slice of
	// that size grows once or not at all, where a slice that grows
	// from empty copies the tokens many times.
	toks, err := lex(src, len(src)/4+1)
	if err != nil {
		return "", err
	}
	upcaseDDLKeywords(toks)
	if err := checkCopyFromStdin(toks); err != nil {
		return "", err
	}
	if err := checkDepth(toks); err != nil {
		return "", err
	}
	items := buildTree(toks)
	var b bytes.Buffer
	b.Grow(len(src) + len(src)/4)
	pTop(&b, items)
	if bs := b.Bytes(); len(bs) == 0 || bs[len(bs)-1] != '\n' {
		b.WriteByte('\n')
	}
	out := b.String()

	// The output holds the tokens of the input, so their count sizes
	// the slice.
	outToks, err := lex(out, len(toks))
	if err != nil {
		return "", fmt.Errorf("pgfmt: lex output: %w", err)
	}
	upcaseDDLKeywords(outToks)

	if !sameTokens(toks, outToks) {
		return "", fmt.Errorf("pgfmt: round-trip token mismatch")
	}
	if !keptDelimiterComments(toks, outToks) {
		return "", fmt.Errorf("pgfmt: dropped comment holds a quote or delimiter")
	}

	return out, nil
}

// checkCopyFromStdin refuses any COPY ... FROM statement whose source
// is not a file name or PROGRAM. Postgres treats both STDIN and STDOUT
// after FROM as a client-side copy, and psql reads lines after that
// statement as data until a `\.` line, so moving line breaks changes
// which lines psql runs as SQL.
func checkCopyFromStdin(toks []token) error {
	i, n := 0, len(toks)
	for i < n {
		j := i
		for j < n && toks[j].kind == tkComment {
			j++
		}
		if j >= n || toks[j].kind == tkEOF {
			break
		}
		k := j
		for k < n && toks[k].kind != tkEOF && !(toks[k].kind == tkPunct && toks[k].val == ";") {
			k++
		}
		if toks[j].kind == tkIdent && toks[j].val == "copy" {
			depth := 0
			afterDot := false
		scan:
			for s := j + 1; s < k; s++ {
				if toks[s].kind == tkComment {
					continue
				}
				switch {
				case toks[s].kind == tkPunct && toks[s].val == "(":
					depth++
				case toks[s].kind == tkPunct && toks[s].val == ")" && depth > 0:
					depth--
				case depth == 0 && !afterDot && toks[s].kind == tkKeyword && toks[s].val == "TO":
					break scan
				case depth == 0 && !afterDot && toks[s].kind == tkKeyword && toks[s].val == "FROM":
					next := skipComments(toks, s+1, k)
					if next < k && toks[next].kind == tkIdent && toks[next].val == "program" {
						next = skipComments(toks, next+1, k)
					}
					if next >= k || toks[next].kind != tkString {
						return fmt.Errorf("pgfmt: COPY FROM requires a file name or PROGRAM")
					}
					break scan
				}
				afterDot = toks[s].kind == tkOp && toks[s].val == "."
			}
		}
		if k < n && toks[k].kind == tkPunct && toks[k].val == ";" {
			i = k + 1
		} else {
			i = k
		}
	}
	return nil
}

func skipComments(toks []token, i, limit int) int {
	for i < limit && toks[i].kind == tkComment {
		i++
	}
	return i
}

// keptDelimiterComments reports whether every comment in a that holds a
// quote or delimiter still appears in b in the same order. lex and
// checkDepth check trailing and in-paren comments, so this catches any
// emitter that drops a comment silently.
func keptDelimiterComments(a, b []token) bool {
	j := 0
	for _, t := range a {
		if t.kind != tkComment || !commentHasDelimiter(commentBody(t.val)) {
			continue
		}
		found := false
		for j < len(b) {
			if b[j].kind == tkComment && b[j].val == t.val {
				j++
				found = true
				break
			}
			j++
		}
		if !found {
			return false
		}
	}
	return true
}

// sameTokens reports whether a and b hold the same tokens, with each
// comment and the EOF token left out. It skips those tokens in place,
// so it copies neither slice.
func sameTokens(a, b []token) bool {
	i, j := 0, 0
	for {
		for i < len(a) && isTrivia(a[i]) {
			i++
		}
		for j < len(b) && isTrivia(b[j]) {
			j++
		}
		if i == len(a) || j == len(b) {
			return i == len(a) && j == len(b)
		}
		if a[i].kind != b[j].kind || a[i].val != b[j].val {
			return false
		}
		i++
		j++
	}
}

func isTrivia(t token) bool { return t.kind == tkComment || t.kind == tkEOF }
