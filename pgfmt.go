// Package pgfmt formats Postgres SQL files in one fixed style.
//
// Pipeline: lex (lex.go) -> buildTree (tree.go) -> emit (emit.go);
// clause.go splits statements into clauses. Format is the entry point.
package pgfmt

import (
	"bytes"
	"fmt"
	"strings"
)

// Format formats SQL source per the fixed style.
func Format(src string) (string, error) {
	toks, err := lex(src)
	if err != nil {
		return "", err
	}
	upcaseDDLKeywords(toks)
	items := buildTree(toks)
	var b bytes.Buffer
	pTop(&b, items)
	out := b.String()
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}

	outToks, err := lex(out)
	if err != nil {
		return "", fmt.Errorf("pgfmt: lex output: %w", err)
	}
	upcaseDDLKeywords(outToks)

	inNonTrivia := nonTriviaToks(toks)
	outNonTrivia := nonTriviaToks(outToks)
	if !compareTokens(inNonTrivia, outNonTrivia) {
		return "", fmt.Errorf("pgfmt: round-trip token mismatch")
	}

	return out, nil
}

func nonTriviaToks(toks []token) []token {
	var out []token
	for _, t := range toks {
		if t.kind != tkComment && t.kind != tkEOF {
			out = append(out, t)
		}
	}
	return out
}

func compareTokens(a, b []token) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].kind != b[i].kind || a[i].val != b[i].val {
			return false
		}
	}
	return true
}
