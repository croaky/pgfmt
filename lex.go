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
	// MERGE opens a merge statement. The words in its WHEN heads
	// (MATCHED, SOURCE, TARGET) are not reserved, so clause.go
	// uppercases those where it recognizes the head.
	"merge": true,
	"from":  true, "where": true, "having": true,
	"using": true,
	"group": true, "order": true, "by": true,
	"limit": true, "offset": true,
	// FOR opens a row-locking clause (FOR UPDATE). Postgres reserves the
	// word, so it is never a bare identifier. The words after it (NO,
	// KEY, SHARE, OF, NOWAIT, SKIP, LOCKED) are not reserved, so
	// clause.go uppercases those where it recognizes the clause.
	"for":       true,
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

// upperKeywords maps each keyword, lowercase and uppercase, to its
// uppercase form. lex reads the form from here, so it allocates no new
// string for a keyword. Format lexes its own output, where each keyword
// is already uppercase, so that pass also lowercases no keyword.
var upperKeywords = func() map[string]string {
	m := make(map[string]string, 2*len(keywords))
	for k := range keywords {
		up := strings.ToUpper(k)
		m[k] = up
		m[up] = up
	}
	return m
}()

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

// lex reads src into tokens. want is the number of tokens the caller
// expects, which sets the first capacity of the slice.
func lex(src string, want int) ([]token, error) {
	toks := make([]token, 0, want)
	i, n := 0, len(src)
	blanks := 0
	for i < n {
		ch := src[i]
		if ch == ' ' || ch == '\t' {
			i++
			continue
		}
		// Postgres ends a line at CR or LF. CR LF is one line break.
		if ch == '\n' || ch == '\r' {
			if ch == '\n' || i+1 >= n || src[i+1] != '\n' {
				blanks++
			}
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
			j := lineEnd(src, i+2)
			if blanks > 0 || len(toks) == 0 {
				toks = append(toks, token{kind: tkComment, val: src[i:j], blanksBefore: blanks})
			} else if commentHasDelimiter(src[i+2 : j]) {
				return nil, fmt.Errorf("pgfmt: trailing comment at offset %d holds a quote or delimiter; move it to its own line above the statement", i)
			}
			blanks = 0
			i = j
			continue
		}
		// Block comment: /* ... */. May span multiple lines; internal
		// newlines do not affect blanksBefore on the next token.
		//
		// Block comments nest, as in Postgres: the comment ends at the
		// `*/` that brings the depth back to zero. A scan that stopped at
		// the first `*/` read the rest of the comment as code.
		if ch == '/' && i+1 < n && src[i+1] == '*' {
			j, depth, closed := i+2, 1, false
			for j+1 < n {
				if src[j] == '/' && src[j+1] == '*' {
					depth++
					j += 2
					continue
				}
				if src[j] == '*' && src[j+1] == '/' {
					j += 2
					if depth--; depth == 0 {
						closed = true
						break
					}
					continue
				}
				j++
			}
			if !closed {
				return nil, fmt.Errorf("pgfmt: unterminated block comment at offset %d", i)
			}
			val := src[i:j]
			if hasCopyEndLine(val) {
				return nil, fmt.Errorf("pgfmt: block comment at offset %d holds a bare \\. line", i)
			}
			toks = append(toks, token{kind: tkComment, val: val, blanksBefore: blanks})
			blanks = 0
			i = j
			continue
		}
		// String literal: 'foo', or one with a prefix against the quote:
		// E'foo', B'101', X'1F', N'foo', or U&'foo'. The prefix is part
		// of the literal. A scan that read it as a name printed X'1F' as
		// `x '1F'`, which Postgres reads as a typed literal.
		if p := stringPrefix(src, i); p >= 0 {
			q := i + p
			var prefix byte
			if p > 0 {
				prefix = src[i] | 0x20 // ASCII lowercase
			}
			j, err := quoteEnd(src, i, q+1, prefix)
			if err != nil {
				return nil, err
			}
			if continuesString(src, j) {
				return nil, fmt.Errorf("pgfmt: string literal continued on a new line at offset %d", i)
			}
			// The prefix prints in uppercase. A literal with no prefix
			// or an uppercase one keeps a slice of src, with no copy.
			val := src[i:j]
			if strings.ContainsAny(src[i:q], "ebxnu") {
				val = strings.ToUpper(src[i:q]) + src[q:j]
			}
			if hasCopyEndLine(val) {
				return nil, fmt.Errorf("pgfmt: string literal at offset %d holds a bare \\. line", i)
			}
			toks = append(toks, token{kind: tkString, val: val, blanksBefore: blanks})
			blanks = 0
			i = j
			continue
		}
		// Number (including leading-dot floats such as .5).
		if isDigit(ch) || (ch == '.' && i+1 < n && isDigit(src[i+1])) {
			j := numberEnd(src, i)
			// An identifier byte (isIdentCont or `$`) against the end
			// of a number means numberEnd stopped short. Emitting two
			// tokens prints `0b02` as `0b0 2` or `0B0$$$$` as
			// `0B0 $$$$`, and the round-trip check in Format misses
			// the split. Refuse the input instead: Postgres rejects
			// these forms as trailing junk after a numeric literal.
			if j < n && (isIdentCont(src[j]) || src[j] == '$') {
				return nil, fmt.Errorf("pgfmt: trailing junk after numeric literal at offset %d", i)
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
				val := src[i:j]
				if hasCopyEndLine(val) {
					return nil, fmt.Errorf("pgfmt: dollar-quoted string at offset %d holds a bare \\. line", i)
				}
				toks = append(toks, token{kind: tkString, val: val, blanksBefore: blanks})
				blanks = 0
				i = j
				continue
			}
		}
		// Parameter: $N. Postgres 15+ reads a parameter directly followed
		// by an identifier start byte (ASCII letter, underscore, or byte
		// >= 0x80) as one erroring param_junk token. Splitting it prints
		// two valid tokens or shifts a string boundary when E' follows.
		if ch == '$' && i+1 < n && isDigit(src[i+1]) {
			j := i + 1
			for j < n && isDigit(src[j]) {
				j++
			}
			if j < n && (isIdentStart(src[j]) || src[j] >= 0x80) {
				return nil, fmt.Errorf("pgfmt: trailing junk after parameter at offset %d", i)
			}
			toks = append(toks, token{kind: tkParam, val: src[i:j], blanksBefore: blanks})
			blanks = 0
			i = j
			continue
		}
		// Identifier or keyword. Postgres continues an identifier
		// through `$`, so `like$t$` is one name. A scan that stopped at
		// `$` read a keyword and then a dollar quote or a parameter.
		if isIdentStart(ch) {
			j := i
			for j < n && (isIdentCont(src[j]) || src[j] == '$') {
				j++
			}
			word := src[i:j]
			up, ok := upperKeywords[word]
			if !ok {
				up, ok = upperKeywords[strings.ToLower(word)]
			}
			kind, out := tkKeyword, up
			if !ok {
				kind, out = tkIdent, strings.ToLower(word)
			}
			toks = append(toks, token{kind: kind, val: out, blanksBefore: blanks})
			blanks = 0
			i = j
			continue
		}
		// Punctuation.
		if ch == '(' || ch == ')' || ch == '[' || ch == ']' || ch == ',' || ch == ';' {
			// A slice of src, since string(ch) allocates for each token.
			toks = append(toks, token{kind: tkPunct, val: src[i : i+1], blanksBefore: blanks})
			blanks = 0
			i++
			continue
		}
		// Operators.
		if op, end := matchOp(src, i); op != "" {
			toks = append(toks, token{kind: tkOp, val: op, blanksBefore: blanks})
			blanks = 0
			// Each byte of the + and - tail that matchOp cut off is
			// one operator. Reading the tail here, and not with one
			// call of matchOp for each byte, keeps lex linear.
			for i += len(op); i < end; i++ {
				toks = append(toks, token{kind: tkOp, val: src[i : i+1]})
			}
			continue
		}
		return nil, fmt.Errorf("pgfmt: unexpected character %q at offset %d", ch, i)
	}
	toks = append(toks, token{kind: tkEOF, blanksBefore: blanks})
	return toks, nil
}

// numberEnd returns the offset one past the numeric literal that starts
// at i, which the caller has checked begins with a digit.
//
// It reads the whole grammar rather than digits and one dot. The short
// version stopped at the first letter, and the lexer then read that
// letter as an identifier, so a literal came apart into two tokens with
// a space between them: 1e6 printed as "1 e6" and 0x1f as "0 x1f".
// The caller refuses a literal this stops short of, so a gap in the
// grammar here is an error and not a rewritten file.
//
// The literal keeps the bytes the author wrote. Numbers are not cased
// or normalized anywhere else here either: 1.50 keeps its zero and
// 0xDEAD keeps its capitals, the same way this does not rewrite 1e6 as
// 1000000.
func numberEnd(src string, i int) int {
	n := len(src)
	// A radix prefix takes its own digits and no dot or exponent: 0x1f
	// is hex and 0x1e5 is one hex number, not hex with an exponent.
	if src[i] == '0' && i+2 < n {
		var radix string
		switch src[i+1] {
		case 'x', 'X':
			radix = hexDigits
		case 'o', 'O':
			radix = octalDigits
		case 'b', 'B':
			radix = binaryDigits
		}
		if strings.IndexByte(radix, src[i+2]) >= 0 {
			return runOfDigits(src, i+2, radix)
		}
	}

	j := runOfDigits(src, i, decimalDigits)
	if j < n && src[j] == '.' && !strings.HasPrefix(src[j:], "..") {
		j = runOfDigits(src, j+1, decimalDigits)
	}

	// An exponent needs at least one digit, after an optional sign.
	// Without one the `e` is the start of something else, so the
	// number ends before it: Postgres rejects `1e` as a literal, and
	// guessing otherwise would read the next word into the number.
	if j < n && (src[j] == 'e' || src[j] == 'E') {
		k := j + 1
		if k < n && (src[k] == '+' || src[k] == '-') {
			k++
		}
		if k < n && isDigit(src[k]) {
			j = runOfDigits(src, k, decimalDigits)
		}
	}
	return j
}

// The digits each radix accepts, as a set runOfDigits searches.
const (
	decimalDigits = "0123456789"
	hexDigits     = "0123456789abcdefABCDEF"
	octalDigits   = "01234567"
	binaryDigits  = "01"
)

// runOfDigits returns the offset one past a run of digits that may hold
// an underscore between two of them.
//
// Postgres 16 added the separator for readable literals such as
// 1_000_000. It must sit between two digits. A leading or trailing
// underscore ends the scan so lex refuses it as trailing junk.
func runOfDigits(src string, i int, digits string) int {
	n := len(src)
	j := i
	for j < n {
		if strings.IndexByte(digits, src[j]) >= 0 {
			j++
			continue
		}
		if j > i && src[j] == '_' && j+1 < n && strings.IndexByte(digits, src[j+1]) >= 0 {
			j += 2
			continue
		}
		break
	}
	return j
}

// lineEnd returns the offset of the CR or LF that ends the line at i,
// or the end of src. Postgres ends a line comment at either byte.
func lineEnd(src string, i int) int {
	for i < len(src) && src[i] != '\n' && src[i] != '\r' {
		i++
	}
	return i
}

// commentHasDelimiter reports whether a comment body holds a byte or
// pair that can start or end a token under another reading. Dropping
// such a comment silently would hide SQL if lex disagreed with
// Postgres.
func commentHasDelimiter(body string) bool {
	return strings.ContainsAny(body, "'\"$\\") ||
		strings.Contains(body, "/*") ||
		strings.Contains(body, "*/")
}

// hasCopyEndLine reports whether s holds a physical line that is only
// `\.`, the psql COPY FROM STDIN end-of-data marker, with LF or CRLF
// ending.
func hasCopyEndLine(s string) bool {
	for len(s) > 0 {
		line := s
		if nl := strings.IndexByte(s, '\n'); nl >= 0 {
			line = s[:nl]
			s = s[nl+1:]
		} else {
			s = ""
		}
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		if line == `\.` {
			return true
		}
	}
	return false
}

// commentBody returns the interior of a `--` or `/* ... */` comment.
func commentBody(val string) string {
	if strings.HasPrefix(val, "/*") && len(val) >= 4 {
		return val[2 : len(val)-2]
	}
	if strings.HasPrefix(val, "--") {
		return val[2:]
	}
	return val
}

// continuesString reports whether the string literal that ends at j
// goes on in a second quoted segment. Postgres joins two segments that
// only whitespace with a newline separates. It reads the second by the
// rules of the first, so an E-string rule reaches into a segment that
// looks plain. pgfmt refuses the form, because a printer that put the
// segments on one line would change the SQL. It also refuses a line
// comment between the segments, which Postgres rejects.
func continuesString(src string, j int) bool {
	newline := false
	for j < len(src) {
		switch {
		case src[j] == ' ' || src[j] == '\t':
			j++
		case src[j] == '\n' || src[j] == '\r':
			newline = true
			j++
		case strings.HasPrefix(src[j:], "--"):
			j = lineEnd(src, j)
		default:
			return newline && src[j] == '\''
		}
	}
	return false
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

// stringPrefix returns the length of the prefix of a string literal
// that starts at i: 0 for 'foo', 1 for E'foo', B'101', X'1F', and
// N'foo', and 2 for U&'foo'. It returns -1 when no string literal
// starts at i.
func stringPrefix(src string, i int) int {
	switch {
	case src[i] == '\'':
		return 0
	case i+1 < len(src) && src[i+1] == '\'' && strings.IndexByte("eEbBxXnN", src[i]) >= 0:
		return 1
	case i+2 < len(src) && (src[i] == 'u' || src[i] == 'U') && src[i+1] == '&' && src[i+2] == '\'':
		return 2
	}
	return -1
}

// quoteEnd returns the offset one past the quote that ends a string
// literal that starts at start and whose body starts at j0. The prefix,
// the first byte of the literal in lowercase or 0 for none, sets the
// rules, as in Postgres.
//
// Postgres finds the end of a string from client_encoding and
// standard_conforming_strings, which the SQL file itself can change
// with SET. pgfmt refuses each string whose end depends on either
// setting rather than guess: a backslash directly after a byte of 0x80
// or above in an escape-aware reading, and a plain or N string whose
// end differs when standard_conforming_strings is off.
func quoteEnd(src string, start, j0 int, prefix byte) (int, error) {
	doubled := prefix != 'b' && prefix != 'x'
	j, closed, err := scanQuote(src, j0, prefix == 'e', doubled)
	if err != nil {
		return 0, err
	}
	if !closed {
		return 0, fmt.Errorf("pgfmt: unterminated string literal at offset %d", start)
	}
	if (prefix == 0 || prefix == 'n') && strings.IndexByte(src[j0:j], '\\') >= 0 {
		offEnd, offClosed, err := scanQuote(src, j0, true, true)
		if err != nil {
			return 0, err
		}
		if !offClosed || offEnd != j {
			return 0, fmt.Errorf("pgfmt: string literal at offset %d ends differently with standard_conforming_strings off", start)
		}
	}
	return j, nil
}

// scanQuote scans a string literal body that starts at j0 and returns
// the offset one past its closing quote and whether that quote exists.
// When escape is true, a backslash escapes the next byte, and a
// backslash directly after a byte of 0x80 or above is an error because
// multibyte client encodings such as SJIS, BIG5, and GBK can read 0x5C
// as the second byte of a character.
func scanQuote(src string, j0 int, escape, doubled bool) (int, bool, error) {
	j := j0
	for j < len(src) {
		switch {
		case escape && src[j] == '\\':
			if j > j0 && src[j-1] >= 0x80 {
				return 0, false, fmt.Errorf("pgfmt: non-ASCII byte before backslash at offset %d; use a \\u escape or an ASCII byte between them", j)
			}
			j += 2
		case src[j] != '\'':
			j++
		case doubled && j+1 < len(src) && src[j+1] == '\'':
			j += 2
		default:
			return j + 1, true, nil
		}
	}
	return j, false, nil
}

// opChars are the bytes that Postgres reads as one operator when they
// run together.
const opChars = "~!@#^&|`?+-*/%<>="

// matchOp returns the operator that starts at i, by the rules of the
// Postgres lexer (scan.l), and the offset one past the run it read. It
// returns "" when no operator starts at i. An operator is the longest
// run of opChars, cut before a `--` or `/*` inside it. A run of two or
// more that ends in `+` or `-` loses that tail, unless it holds one of
// ~ ! @ # ^ & | ` ? %. So `=-1` is `=` then `-1`, and `!=-1` is `!=-`
// then `1`.
//
// Postgres reads each byte of a cut tail as one operator: the rest of
// the run holds only + and - and none of the bytes above. The caller
// reads the tail from the offset. A caller that calls matchOp again
// for each byte of the tail scans the run again each time, and takes
// time quadratic in the run length.
//
// The scan stops at the first `--` or `/*`. `/` and `*` are opChars,
// so a run can continue after an empty comment. A scan that read the
// full run and then cut it read the same bytes again on each call, so
// `+/**/` repeated took time quadratic in its length.
//
// A table of known operators took a prefix of the run, so `<<` came
// apart as `<` and `<`. The printer put a space between them, and
// Postgres read two operators.
func matchOp(src string, i int) (string, int) {
	if strings.HasPrefix(src[i:], "::") {
		return "::", i + 2
	}
	if strings.HasPrefix(src[i:], "..") {
		return "..", i + 2
	}
	if src[i] == '.' {
		return ".", i + 1
	}
	j := i
	for j < len(src) && strings.IndexByte(opChars, src[j]) >= 0 {
		if j+1 < len(src) && (src[j] == '-' && src[j+1] == '-' || src[j] == '/' && src[j+1] == '*') {
			break
		}
		j++
	}
	op, end := src[i:j], j
	if !strings.ContainsAny(op, "~!@#^&|`?%") {
		for len(op) > 1 && (op[len(op)-1] == '+' || op[len(op)-1] == '-') {
			op = op[:len(op)-1]
		}
	}
	return op, end
}
