// tree.go builds the parsed item/group tree and classifies parenthesized
// groups so the printer can choose a wrap style.

package pgfmt

import (
	"errors"
	"fmt"
	"slices"
)

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

// maxDepth is the deepest nesting of parens and CASE that Format
// accepts. The tree and emit stages recurse once for each level, the
// indent grows two spaces for each level, and the fits-on-one-line
// measure in emitExpr reads the subtree again at each level. The limit
// bounds all three: the stack, the output at about 200 spaces of indent
// on a line, and the CPU time at input size times maxDepth. Hand-written
// SQL stays far below 100 levels.
const maxDepth = 100

// checkDepth returns an error if parens do not balance, as Postgres
// does, or if parens and CASE together nest deeper than maxDepth. It
// reads the tokens once, before buildTree recurses.
//
// It counts CASE for each paren level, as findCaseEnd and nextCaseToken
// pair CASE with END. An END lowers the count of its own level only,
// and only when that count is above zero, because END is also a
// statement. A ) drops the count of the level it closes. With one count
// for all levels, CASE WHEN (END) THEN CASE ... let each END in parens
// cancel a CASE that the emit stage still nests in.
func checkDepth(toks []token) error {
	// cases is the open CASE count over all levels, level is the count
	// of the current level, and outer holds the count of each outer
	// level. outer holds at most maxDepth+1 counts, because the check
	// below refuses more parens than that.
	parens, cases, level := 0, 0, 0
	var outer []int
	for _, t := range toks {
		switch {
		case t.kind == tkPunct && t.val == "(":
			parens++
			outer = append(outer, level)
			level = 0
		case t.kind == tkPunct && t.val == ")":
			if parens == 0 {
				return errors.New("pgfmt: ) without (")
			}
			parens--
			cases -= level
			level = outer[len(outer)-1]
			outer = outer[:len(outer)-1]
		case t.kind == tkKeyword && t.val == "CASE":
			cases++
			level++
		case t.kind == tkKeyword && t.val == "END" && level > 0:
			cases--
			level--
		}
		if parens+cases > maxDepth {
			return fmt.Errorf("pgfmt: parens and CASE nest deeper than %d", maxDepth)
		}
	}
	if parens > 0 {
		return errors.New("pgfmt: ( without )")
	}
	return nil
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
