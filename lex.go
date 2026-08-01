// lex.go turns SQL source into a flat token stream and applies keyword
// casing.

package pgfmt

import (
	"fmt"
	"strings"
)

type tokenKind int

const (
	tkEOF tokenKind = iota
	tkKeyword
	tkIdent
	tkString
	tkNumber
	tkParam
	tkPunct
	tkOp
	tkComment
)

type token struct {
	kind tokenKind
	val  string
	// blanksBefore is the number of "\n" characters between this token and
	// the previous one. 0 means same line, 1 means next line, 2+ means blank
	// line(s) before. EOF carries the final newline count.
	blanksBefore int
}

// keywords is the set of words rendered UPPERCASE. Anything else stays as
// lowercase identifier. Types like bigint/text/date/interval are NOT here;
// they remain lowercase per the style.
var keywords = map[string]bool{
	"select": true, "insert": true, "into": true, "values": true,
	"update": true, "set": true, "delete": true, "with": true,
	"from": true, "where": true, "having": true,
	"using": true,
	"group": true, "order": true, "by": true,
	"limit": true, "offset": true,
	"returning": true,
	"on":        true, "conflict": true, "do": true, "nothing": true,
	"union": true, "intersect": true, "except": true,
	"join": true, "left": true, "right": true, "inner": true, "outer": true,
	"full": true, "cross": true, "lateral": true,
	"and": true, "or": true, "not": true, "is": true,
	"in": true, "exists": true, "any": true, "all": true,
	"between": true, "like": true, "ilike": true, "as": true,
	"null": true, "true": true, "false": true,
	"case": true, "when": true, "then": true, "else": true, "end": true,
	"distinct": true, "asc": true, "desc": true,
	"nulls": true, "first": true, "last": true,
	"over": true, "partition": true,
	"array":    true,
	"truncate": true, "refresh": true, "materialized": true,
	"view": true, "concurrently": true,
	"create": true, "drop": true, "table": true, "if": true,
	"including": true, "alter": true, "rename": true, "to": true,
	"begin": true, "commit": true, "rollback": true,
	"epoch":    true, // EXTRACT field constant
	"excluded": true, // ON CONFLICT DO UPDATE pseudo-row
}

// ddlKeywords are uppercased only inside DDL statements (see ddlStarters).
// They are not global keywords because several double as ordinary
// identifiers in DML: e.g. the `value, index` column list of WITH
// ORDINALITY, or a column literally named `key` or `add`.
var ddlKeywords = map[string]bool{
	"add": true, "analyze": true, "constraint": true, "excluding": true,
	"index": true, "indexes": true, "key": true, "primary": true,
	"sequence": true,
}

// ddlStarters are the leading keywords that mark a statement as DDL, so
// ddlKeywords inside it should be uppercased.
var ddlStarters = map[string]bool{
	"create": true, "alter": true, "drop": true,
	"analyze": true, "truncate": true, "reindex": true,
}

// upcaseDDLKeywords applies statement-scoped casing the context-free lexer
// cannot: within each DDL statement it uppercases ddlKeywords, leaving the
// same words untouched in DML where they may be identifiers.
func upcaseDDLKeywords(toks []token) {
	i, n := 0, len(toks)
	for i < n {
		j := i
		for j < n && toks[j].kind == tkComment {
			j++
		}
		if j >= n || toks[j].kind == tkEOF {
			break
		}
		isDDL := ddlStarters[strings.ToLower(toks[j].val)]
		k := j
		for k < n && toks[k].kind != tkEOF && !(toks[k].kind == tkPunct && toks[k].val == ";") {
			k++
		}
		if isDDL {
			for s := j; s <= k && s < n; s++ {
				if toks[s].kind == tkIdent && ddlKeywords[strings.ToLower(toks[s].val)] {
					toks[s].kind = tkKeyword
					toks[s].val = strings.ToUpper(toks[s].val)
				}
			}
		}
		if k < n && toks[k].kind == tkPunct && toks[k].val == ";" {
			i = k + 1
		} else {
			i = k
		}
	}
}

// operators ordered by length (longest first) for correct longest-match.
var operators = []string{
	"->>", "!~*",
	"=>", "<>", "<=", ">=", "!=", "->", "~*", "!~", "::", "||", "@>", "<@",
	"<", ">", "=", "+", "-", "*", "/", "%", ".", "~", "?",
}

func lex(src string) ([]token, error) {
	var toks []token
	i, n := 0, len(src)
	blanks := 0
	for i < n {
		ch := src[i]
		if ch == ' ' || ch == '\t' || ch == '\r' {
			i++
			continue
		}
		if ch == '\n' {
			blanks++
			i++
			continue
		}
		// Line comment: -- to end of line.
		//
		// Trailing comments (same line as a prior token) are dropped
		// rather than emitted: the printer joins items via inline()
		// with single spaces, and a `--` line-comment would swallow
		// every following token on the rendered line. Standalone
		// comments (those that start on their own line) pass through
		// and are emitted by pTop before the following statement.
		if ch == '-' && i+1 < n && src[i+1] == '-' {
			j := i + 2
			for j < n && src[j] != '\n' {
				j++
			}
			if blanks > 0 || len(toks) == 0 {
				toks = append(toks, token{kind: tkComment, val: src[i:j], blanksBefore: blanks})
			}
			blanks = 0
			i = j
			continue
		}
		// Block comment: /* ... */. May span multiple lines; internal
		// newlines do not affect blanksBefore on the next token.
		if ch == '/' && i+1 < n && src[i+1] == '*' {
			j, closed := i+2, false
			for j+1 < n {
				if src[j] == '*' && src[j+1] == '/' {
					j += 2
					closed = true
					break
				}
				j++
			}
			if !closed {
				return nil, fmt.Errorf("pgfmt: unterminated block comment at offset %d", i)
			}
			toks = append(toks, token{kind: tkComment, val: src[i:j], blanksBefore: blanks})
			blanks = 0
			i = j
			continue
		}
		// String literal: 'foo' with '' as escape for literal '.
		if (ch == 'e' || ch == 'E') && i+1 < n && src[i+1] == '\'' {
			j, closed := i+2, false
			for j < n {
				if src[j] != '\'' {
					j++
					continue
				}
				if j+1 < n && src[j+1] == '\'' {
					j += 2
					continue
				}
				j++
				closed = true
				break
			}
			if !closed {
				return nil, fmt.Errorf("pgfmt: unterminated string literal at offset %d", i)
			}
			toks = append(toks, token{kind: tkString, val: "E" + src[i+1:j], blanksBefore: blanks})
			blanks = 0
			i = j
			continue
		}
		// String literal: 'foo' with '' as escape for literal '.
		if ch == '\'' {
			j, closed := i+1, false
			for j < n {
				if src[j] != '\'' {
					j++
					continue
				}
				if j+1 < n && src[j+1] == '\'' {
					j += 2
					continue
				}
				j++
				closed = true
				break
			}
			if !closed {
				return nil, fmt.Errorf("pgfmt: unterminated string literal at offset %d", i)
			}
			toks = append(toks, token{kind: tkString, val: src[i:j], blanksBefore: blanks})
			blanks = 0
			i = j
			continue
		}
		// Number: digits[.digits].
		if isDigit(ch) {
			j, seenDot := i, false
			for j < n {
				if isDigit(src[j]) {
					j++
					continue
				}
				if src[j] == '.' && !seenDot {
					seenDot = true
					j++
					continue
				}
				break
			}
			toks = append(toks, token{kind: tkNumber, val: src[i:j], blanksBefore: blanks})
			blanks = 0
			i = j
			continue
		}
		// Dollar-quoted string: $$body$$ or $tag$body$tag$. The body is
		// another language -- a PL/pgSQL function, usually -- so it is one
		// opaque token, kept byte for byte. Formatting inside it would mean
		// knowing that language, and reindenting it would change a string.
		if ch == '$' {
			if tag, ok := dollarTag(src, i); ok {
				end := strings.Index(src[i+len(tag):], tag)
				if end < 0 {
					return nil, fmt.Errorf("pgfmt: unterminated %s string at offset %d", tag, i)
				}
				j := i + len(tag) + end + len(tag)
				toks = append(toks, token{kind: tkString, val: src[i:j], blanksBefore: blanks})
				blanks = 0
				i = j
				continue
			}
		}
		// Parameter: $N.
		if ch == '$' && i+1 < n && isDigit(src[i+1]) {
			j := i + 1
			for j < n && isDigit(src[j]) {
				j++
			}
			toks = append(toks, token{kind: tkParam, val: src[i:j], blanksBefore: blanks})
			blanks = 0
			i = j
			continue
		}
		// Identifier or keyword.
		if isIdentStart(ch) {
			j := i
			for j < n && isIdentCont(src[j]) {
				j++
			}
			lc := strings.ToLower(src[i:j])
			kind, out := tkIdent, lc
			if keywords[lc] {
				kind, out = tkKeyword, strings.ToUpper(lc)
			}
			toks = append(toks, token{kind: kind, val: out, blanksBefore: blanks})
			blanks = 0
			i = j
			continue
		}
		// Punctuation.
		if ch == '(' || ch == ')' || ch == '[' || ch == ']' || ch == ',' || ch == ';' {
			toks = append(toks, token{kind: tkPunct, val: string(ch), blanksBefore: blanks})
			blanks = 0
			i++
			continue
		}
		// Operators.
		if op, ok := matchOp(src, i); ok {
			toks = append(toks, token{kind: tkOp, val: op, blanksBefore: blanks})
			blanks = 0
			i += len(op)
			continue
		}
		return nil, fmt.Errorf("pgfmt: unexpected character %q at offset %d", ch, i)
	}
	toks = append(toks, token{kind: tkEOF, blanksBefore: blanks})
	return toks, nil
}

func isDigit(ch byte) bool { return ch >= '0' && ch <= '9' }

func isIdentStart(ch byte) bool {
	return ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isIdentCont(ch byte) bool { return isIdentStart(ch) || isDigit(ch) }

// dollarTag returns the opening delimiter of a dollar-quoted string at i,
// including both dollar signs. A tag is $$ or $ident$; $1 is a parameter and
// $ alone is neither.
func dollarTag(src string, i int) (string, bool) {
	if i+1 < len(src) && isDigit(src[i+1]) {
		return "", false
	}
	j := i + 1
	for j < len(src) && isIdentCont(src[j]) {
		j++
	}
	if j >= len(src) || src[j] != '$' {
		return "", false
	}
	return src[i : j+1], true
}

func matchOp(src string, i int) (string, bool) {
	for _, op := range operators {
		if strings.HasPrefix(src[i:], op) {
			return op, true
		}
	}
	return "", false
}
