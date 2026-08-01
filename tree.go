// tree.go builds the parsed item/group tree and classifies parenthesized
// groups so the printer can choose a wrap style.

package pgfmt

import "slices"

// parenKind classifies parenthesized groups so the printer can choose the
// right wrap style.
type parenKind int

const (
	pkFunc parenKind = iota
	pkList
	pkSubquery
	pkPred
)

type item struct {
	tok *token
	grp *group
}

type group struct {
	kind  parenKind
	items []item
	fn    string // function name if pkFunc
}

func (it item) isTok(vals ...string) bool {
	if it.tok == nil {
		return false
	}
	return slices.Contains(vals, it.tok.val)
}

func (it item) isKW(vals ...string) bool {
	if it.tok == nil || it.tok.kind != tkKeyword {
		return false
	}
	return slices.Contains(vals, it.tok.val)
}

func (it item) isComment() bool { return it.tok != nil && it.tok.kind == tkComment }

func (it item) blanksBefore() int {
	if it.tok != nil {
		return it.tok.blanksBefore
	}
	if it.grp != nil && len(it.grp.items) > 0 {
		return it.grp.items[0].blanksBefore()
	}
	return 0
}

// buildTree converts a flat token slice into a tree of items, grouping
// matched parens.
//
// Comments inside parenthesized groups are dropped: the printer joins
// nested items via inline(), and a `--` line-comment would swallow
// the rest of the line. Only top-level comments survive, where pTop
// emits them on their own lines before the following statement.
func buildTree(toks []token) []item {
	pos := 0
	var build func(depth int) []item
	build = func(depth int) []item {
		var out []item
		for pos < len(toks) {
			t := &toks[pos]
			if t.kind == tkEOF {
				return out
			}
			if t.kind == tkPunct && t.val == "(" {
				pos++
				children := build(depth + 1)
				grp := &group{items: children}
				grp.kind, grp.fn = classify(out, grp)
				out = append(out, item{grp: grp})
				continue
			}
			if t.kind == tkPunct && t.val == ")" {
				pos++
				return out
			}
			if depth > 0 && t.kind == tkComment {
				pos++
				continue
			}
			out = append(out, item{tok: t})
			pos++
		}
		return out
	}
	return build(0)
}

// classify decides what kind of parenthesized group g is, based on what
// precedes it and what it contains. Defaults to pkList so unknown groups
// wrap as a list rather than crashing the printer.
func classify(ctx []item, g *group) (parenKind, string) {
	last1 := lastTok(ctx, 0)
	last2 := lastTok(ctx, 1)
	if first := firstToken(g.items); first != nil && first.kind == tkKeyword {
		switch first.val {
		case "SELECT", "WITH":
			return pkSubquery, ""
		}
	}
	if last1 != nil && last1.kind == tkKeyword {
		switch last1.val {
		case "EXISTS", "UNION", "INTERSECT", "EXCEPT":
			return pkSubquery, ""
		case "ANY", "ALL", "IN", "VALUES", "INTO", "CONFLICT":
			if last1.val == "ALL" && last2 != nil && last2.kind == tkKeyword && last2.val == "UNION" {
				return pkSubquery, ""
			}
			return pkList, ""
		}
	}
	// INSERT INTO table (...): last1 ident, last2 INTO.
	if last1 != nil && last1.kind == tkIdent && last2 != nil && last2.kind == tkKeyword && last2.val == "INTO" {
		return pkList, ""
	}
	// Function call: ident immediately before. EXTRACT uses FROM inside its
	// arg list (EXTRACT(EPOCH FROM expr)); treating ident-then-paren as a
	// function call uniformly is the only ambiguity in our codebase.
	if last1 != nil && last1.kind == tkIdent {
		return pkFunc, last1.val
	}
	// Parenthesized set operation: (SELECT ...) UNION ALL (SELECT ...) wrapped
	// in outer parens, e.g. FROM ((...) UNION ALL (...)) s. The leading operand
	// is a subquery group and a set operator joins the operands at top level.
	// Guarded by the function-call check above so coalesce((SELECT ...), x) and
	// friends stay function calls.
	if len(g.items) > 0 && g.items[0].grp != nil && g.items[0].grp.kind == pkSubquery &&
		hasTopSetOp(g.items) {
		return pkSubquery, ""
	}
	for _, it := range g.items {
		if it.tok != nil && it.tok.kind == tkPunct && it.tok.val == "," {
			return pkList, ""
		}
	}
	for _, it := range g.items {
		if it.tok != nil && it.tok.kind == tkKeyword && (it.tok.val == "AND" || it.tok.val == "OR") {
			return pkPred, ""
		}
	}
	return pkList, ""
}

// hasTopSetOp reports whether items contain a top-level set-operation keyword
// (groups are already collapsed into item{grp:...}, so any keyword here is at
// the top level of this group).
func hasTopSetOp(items []item) bool {
	for _, it := range items {
		if it.isKW("UNION", "INTERSECT", "EXCEPT") {
			return true
		}
	}
	return false
}

func firstToken(items []item) *token {
	for _, it := range items {
		if it.tok != nil {
			return it.tok
		}
	}
	return nil
}

func lastTok(items []item, back int) *token {
	seen := 0
	for _, it := range slices.Backward(items) {
		if it.tok != nil {
			if seen == back {
				return it.tok
			}
			seen++
			continue
		}
		if it.grp != nil {
			if seen == back {
				return nil
			}
			seen++
		}
	}
	return nil
}
