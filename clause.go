// clause.go splits a statement into clauses and recognizes clause heads
// (SELECT, FROM, WHERE, ...).

package pgfmt

import (
	"strings"
)

func indexTopKeyword(items []item, keyword string) int {
	for i, it := range items {
		if it.isKW(keyword) {
			return i
		}
	}
	return -1
}

func indexTopKeywordSeq(items []item, first, second string) int {
	for i := 0; i+1 < len(items); i++ {
		if items[i].isKW(first) && items[i+1].isKW(second) {
			return i
		}
	}
	return -1
}

// clause holds a clause keyword (possibly compound) and its content tokens.
type clause struct {
	keyword string // canonical keyword: SELECT, FROM, WHERE, etc.
	head    []item // keyword tokens (e.g. SELECT, SELECT DISTINCT, ORDER BY)
	body    []item // rest of the clause content
	trailer string // ";" if this clause owns the terminator
}

func splitClauses(items []item) []clause {
	var out []clause
	i := 0
	for i < len(items) {
		head, key, after := matchClauseHead(items, i)
		if head == nil {
			if len(out) == 0 {
				out = append(out, clause{})
			}
			last := &out[len(out)-1]
			last.body = append(last.body, items[i])
			i++
			continue
		}
		c := clause{keyword: key, head: head}
		i = after
		for i < len(items) {
			if items[i].isTok(";") {
				c.trailer = ";"
				i++
				break
			}
			// Inside ON CONFLICT, keep conflict-target WHERE (before DO)
			// and DO UPDATE SET tokens in the same clause.
			if c.keyword == "ON CONFLICT" {
				if items[i].isKW("UPDATE", "SET") {
					c.body = append(c.body, items[i])
					i++
					continue
				}
				if items[i].isKW("WHERE") {
					sawDo := false
					for _, b := range c.body {
						if b.isKW("DO") {
							sawDo = true
							break
						}
					}
					if !sawDo {
						c.body = append(c.body, items[i])
						i++
						continue
					}
				}
			}
			if h2, _, _ := matchClauseHead(items, i); h2 != nil {
				break
			}
			c.body = append(c.body, items[i])
			i++
		}
		out = append(out, c)
	}
	return out
}

// lockStrengths are the four row-locking clauses, longest first, so FOR
// NO KEY UPDATE matches before FOR UPDATE reads the same tokens.
var lockStrengths = [][]string{
	{"FOR", "NO", "KEY", "UPDATE"},
	{"FOR", "KEY", "SHARE"},
	{"FOR", "UPDATE"},
	{"FOR", "SHARE"},
}

// lockOptions are the words that follow a lock strength: which table to
// lock, and what to do with a row another transaction holds.
var lockOptions = map[string]bool{
	"OF": true, "NOWAIT": true, "SKIP": true, "LOCKED": true,
}

// matchLockingHead matches a row-locking clause at items[i], which
// begins with FOR. It returns nil for any other FOR, such as the FOR
// EACH ROW of a trigger, which stays inline in the statement it is in.
//
// Only FOR is a keyword to the lexer. NO, KEY, SHARE, and UPDATE are
// matched whatever their case, and the canonical keyword the emitter
// prints is uppercase.
func matchLockingHead(items []item, i int) ([]item, string, int) {
	for _, words := range lockStrengths {
		if !matchWords(items, i, words) {
			continue
		}
		n := len(words)
		return items[i : i+n], strings.Join(words, " "), i + n
	}
	return nil, "", i
}

// matchWords reports whether items[i:] begins with these words, each
// one token, whatever its case and whether the lexer called it a
// keyword.
func matchWords(items []item, i int, words []string) bool {
	if i+len(words) > len(items) {
		return false
	}
	for n, w := range words {
		it := items[i+n]
		if it.tok == nil || !strings.EqualFold(it.tok.val, w) {
			return false
		}
	}
	return true
}

// upperLockOptions returns the clause's options with OF, NOWAIT, SKIP,
// and LOCKED uppercase. A table name between them keeps the case the
// lexer gave it. The lexer cannot do this, since a column named `of`
// or `locked` is legal everywhere else.
func upperLockOptions(items []item) []item {
	out := make([]item, len(items))
	for n, it := range items {
		out[n] = it
		if it.tok == nil || !lockOptions[strings.ToUpper(it.tok.val)] {
			continue
		}
		t := *it.tok
		t.val = strings.ToUpper(t.val)
		out[n] = item{tok: &t}
	}
	return out
}

// matchClauseHead returns the head tokens, canonical keyword, and the
// index after the head if items[i:] begins a recognized clause head.
func matchClauseHead(items []item, i int) ([]item, string, int) {
	if i >= len(items) || items[i].tok == nil || items[i].tok.kind != tkKeyword {
		return nil, "", i
	}
	switch items[i].tok.val {
	case "SELECT":
		if i+1 < len(items) && items[i+1].isKW("DISTINCT") {
			return items[i : i+2], "SELECT", i + 2
		}
		return items[i : i+1], "SELECT", i + 1
	case "FROM":
		// FROM in `IS [NOT] DISTINCT FROM` is part of that operator, not a
		// clause head; the preceding DISTINCT disambiguates it.
		if i > 0 && items[i-1].isKW("DISTINCT") {
			return nil, "", i
		}
		return items[i : i+1], "FROM", i + 1
	case "WHERE":
		return items[i : i+1], "WHERE", i + 1
	case "GROUP":
		if i+1 < len(items) && items[i+1].isKW("BY") {
			return items[i : i+2], "GROUP BY", i + 2
		}
	case "ORDER":
		if i+1 < len(items) && items[i+1].isKW("BY") {
			return items[i : i+2], "ORDER BY", i + 2
		}
	case "HAVING":
		return items[i : i+1], "HAVING", i + 1
	case "LIMIT":
		return items[i : i+1], "LIMIT", i + 1
	case "OFFSET":
		return items[i : i+1], "OFFSET", i + 1
	case "RETURNING":
		return items[i : i+1], "RETURNING", i + 1
	case "ON":
		if i+1 < len(items) && items[i+1].isKW("CONFLICT") {
			return items[i : i+2], "ON CONFLICT", i + 2
		}
	case "INSERT":
		if i+1 < len(items) && items[i+1].isKW("INTO") {
			return items[i : i+2], "INSERT INTO", i + 2
		}
	case "UPDATE":
		return items[i : i+1], "UPDATE", i + 1
	case "DELETE":
		if i+1 < len(items) && items[i+1].isKW("FROM") {
			return items[i : i+2], "DELETE FROM", i + 2
		}
	case "SET":
		return items[i : i+1], "SET", i + 1
	case "VALUES":
		return items[i : i+1], "VALUES", i + 1
	case "UNION":
		if i+1 < len(items) && items[i+1].isKW("ALL") {
			return items[i : i+2], "UNION ALL", i + 2
		}
		return items[i : i+1], "UNION", i + 1
	case "INTERSECT":
		return items[i : i+1], "INTERSECT", i + 1
	case "EXCEPT":
		return items[i : i+1], "EXCEPT", i + 1
	case "FOR":
		if head, key, after := matchLockingHead(items, i); head != nil {
			return head, key, after
		}
	case "WITH":
		if i+1 < len(items) && items[i+1].tok != nil &&
			items[i+1].tok.kind == tkIdent && strings.EqualFold(items[i+1].tok.val, "ordinality") {
			return nil, "", i
		}
		// Storage clause `WITH (param = value)` (e.g. CREATE INDEX ... WITH
		// (fastupdate = off)) is followed directly by a paren group, not a
		// CTE name. It is not a clause head; keep it inline in the statement.
		if i+1 < len(items) && items[i+1].grp != nil {
			return nil, "", i
		}
		return items[i : i+1], "WITH", i + 1
	}
	return nil, "", i
}
