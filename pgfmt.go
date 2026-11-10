// Package pgfmt formats Postgres SQL files in one fixed style.
//
// Pipeline: lex (lex.go) -> buildTree (tree.go) -> emit (emit.go);
// clause.go splits statements into clauses. Format is the entry point.
package pgfmt

import (
	"bytes"
	"fmt"
)

// Format formats SQL source per the fixed style.
func Format(src string) (string, error) {
	// The input holds about one token for each 4 bytes. A slice of
	// that size grows once or not at all, where a slice that grows
	// from empty copies the tokens many times.
	toks, err := lex(src, len(src)/4+1)
	if err != nil {
		return "", err
	}
	upcaseDDLKeywords(toks)
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

	return out, nil
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
