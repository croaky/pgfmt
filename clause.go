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
