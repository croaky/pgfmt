// emit.go prints the item tree as formatted SQL: clause emitters,
// expression and list wrapping, and CASE layout.

package pgfmt

import (
	"bytes"
	"strings"
)

const lineLimit = 80

// pairedFns wraps two args per line when wrapping is needed.
var pairedFns = map[string]bool{
	"jsonb_build_object": true,
	"json_build_object":  true,
	"jsonb_object":       true,
	"json_object":        true,
	"hstore":             true,
}

// ----------------------------------------------------------------------
// Inline rendering.

// inline renders items as a single-line string.
func inline(items []item) string {
	var b strings.Builder
	for i := range items {
		if i > 0 && needSpace(items[i-1], items[i]) {
			b.WriteByte(' ')
		}
		writeInlineItem(&b, items[i])
	}
	return b.String()
}

func writeInlineItem(b *strings.Builder, it item) {
	if it.tok != nil {
		b.WriteString(it.tok.val)
		return
	}
	if it.grp != nil {
		b.WriteByte('(')
		b.WriteString(inline(it.grp.items))
		b.WriteByte(')')
	}
}

func startsWithTxnControl(items []item, i int) bool {
	for i < len(items) && items[i].isComment() {
		i++
	}
	if i >= len(items) {
		return false
	}
	for ; i < len(items); i++ {
		if items[i].tok != nil {
			return isTxnControlKeyword(items[i].tok.val)
		}
	}
	return false
}

type txnRole int

const (
	txnRoleNone txnRole = iota
	txnRoleBegin
	txnRoleEnd
)

func stmtTxnRole(items []item) txnRole {
	for _, it := range items {
		if it.tok == nil {
			continue
		}
		switch {
		case strings.EqualFold(it.tok.val, "BEGIN"):
			return txnRoleBegin
		case strings.EqualFold(it.tok.val, "COMMIT"), strings.EqualFold(it.tok.val, "ROLLBACK"):
			return txnRoleEnd
		default:
			return txnRoleNone
		}
	}
	return txnRoleNone
}

func isTxnControlKeyword(v string) bool {
	return strings.EqualFold(v, "BEGIN") ||
		strings.EqualFold(v, "COMMIT") ||
		strings.EqualFold(v, "ROLLBACK")
}

func emitCaseValue(b *bytes.Buffer, items []item, indent int) {
	s := inline(items)
	if currentCol(b)+len(s) <= lineLimit && !hasSubquery(items) {
		b.WriteString(s)
		return
	}
	emitExpr(b, items, indent)
}

// needSpace reports whether a space goes between prev and next.
func needSpace(prev, next item) bool {
	switch itemHead(next) {
	case ",", ";", ")", "]", "[", ".", "::":
		return false
	}
	switch itemTail(prev) {
	case "(", "[", ".", "::":
		return false
	}
	if next.grp != nil && next.grp.kind == pkFunc {
		return false
	}
	return true
}

func itemHead(it item) string {
	if it.tok != nil {
		return it.tok.val
	}
	if it.grp != nil {
		return "("
	}
	return ""
}

func itemTail(it item) string {
	if it.tok != nil {
		return it.tok.val
	}
	if it.grp != nil {
		return ")"
	}
	return ""
}

// ----------------------------------------------------------------------
// Top-level printer.

// pTop walks the token tree at top level and emits formatted SQL.
// Items at the top level are: comments and statements (ending in ';').
// Blank lines between statements are preserved.
func pTop(b *bytes.Buffer, items []item) {
	i := 0
	first := true
	txnActive := false
	for i < len(items) {
		nextTxnControl := startsWithTxnControl(items, i)
		if !first && items[i].blanksBefore() >= 2 && !(txnActive || nextTxnControl) {
			b.WriteByte('\n')
		}
		first = false
		for i < len(items) && items[i].isComment() {
			b.WriteString(items[i].tok.val)
			b.WriteByte('\n')
			i++
		}
		if i >= len(items) {
			break
		}
		start := i
		for i < len(items) {
			if items[i].isTok(";") {
				i++
				break
			}
			i++
		}
		stmt := items[start:i]
		pStmt(b, stmt, 0)
		switch stmtTxnRole(stmt) {
		case txnRoleBegin:
			txnActive = true
		case txnRoleEnd:
			txnActive = false
		}
	}
}

// pStmt prints a clause-based SQL statement at the given base indent.
// All statement kinds (SELECT/INSERT/UPDATE/DELETE/WITH) flow through
// splitClauses + emitClause; unrecognized starts fall back to inline.
func pStmt(b *bytes.Buffer, items []item, base int) {
	for _, c := range splitClauses(items) {
		emitClause(b, c, base)
	}
}

func writeIndent(b *bytes.Buffer, n int) {
	for range n {
		b.WriteByte(' ')
	}
}

// writeTerminator turns the buffer's trailing newline into "<end>\n" so
// ";" hugs the previous line. O(1) via bytes.Buffer.Truncate.
func writeTerminator(b *bytes.Buffer, end string) {
	if end == "" {
		return
	}
	if b.Len() > 0 && b.Bytes()[b.Len()-1] == '\n' {
		b.Truncate(b.Len() - 1)
	}
	b.WriteString(end)
	b.WriteByte('\n')
}

// currentCol returns the column position of the cursor (bytes since the
// last newline).
func currentCol(b *bytes.Buffer) int {
	bs := b.Bytes()
	idx := bytes.LastIndexByte(bs, '\n')
	if idx < 0 {
		return len(bs)
	}
	return len(bs) - idx - 1
}

// ----------------------------------------------------------------------
// Per-clause emit.

func emitClause(b *bytes.Buffer, c clause, base int) {
	switch c.keyword {
	case "SELECT":
		writeIndent(b, base)
		b.WriteString(inline(c.head))
		b.WriteByte('\n')
		emitCommaList(b, c.body, base+2)
		writeTerminator(b, c.trailer)
	case "FROM":
		emitFromClause(b, c, base)
	case "WHERE", "HAVING":
		writeIndent(b, base)
		b.WriteString(c.keyword)
		b.WriteByte('\n')
		emitPredicateList(b, c.body, base+2)
		writeTerminator(b, c.trailer)
	case "GROUP BY", "ORDER BY":
		writeIndent(b, base)
		b.WriteString(c.keyword)
		b.WriteByte('\n')
		emitCommaList(b, c.body, base+2)
		writeTerminator(b, c.trailer)
	case "LIMIT", "OFFSET":
		writeIndent(b, base)
		b.WriteString(c.keyword)
		if len(c.body) > 0 {
			b.WriteByte(' ')
			b.WriteString(inline(c.body))
		}
		b.WriteByte('\n')
		writeTerminator(b, c.trailer)
	case "RETURNING":
		writeIndent(b, base)
		b.WriteString("RETURNING\n")
		emitCommaList(b, c.body, base+2)
		writeTerminator(b, c.trailer)
	case "ON CONFLICT":
		emitOnConflict(b, c, base)
	case "INSERT INTO":
		emitInsertInto(b, c, base)
	case "UPDATE":
		writeIndent(b, base)
		b.WriteString("UPDATE\n")
		writeIndent(b, base+2)
		b.WriteString(inline(c.body))
		b.WriteByte('\n')
		writeTerminator(b, c.trailer)
	case "DELETE FROM":
		usingIdx := indexTopKeyword(c.body, "USING")
		deleteTarget := c.body
		usingBody := []item(nil)
		if usingIdx >= 0 {
			deleteTarget = c.body[:usingIdx]
			if usingIdx+1 < len(c.body) {
				usingBody = c.body[usingIdx+1:]
			}
		}
		writeIndent(b, base)
		b.WriteString("DELETE FROM\n")
		writeIndent(b, base+2)
		b.WriteString(inline(deleteTarget))
		b.WriteByte('\n')
		if usingIdx >= 0 {
			writeIndent(b, base)
			b.WriteString("USING\n")
			writeIndent(b, base+2)
			b.WriteString(inline(usingBody))
			b.WriteByte('\n')
		}
		writeTerminator(b, c.trailer)
	case "SET":
		writeIndent(b, base)
		b.WriteString("SET\n")
		emitCommaList(b, c.body, base+2)
		writeTerminator(b, c.trailer)
	case "VALUES":
		emitValues(b, c, base)
	case "UNION ALL", "UNION", "INTERSECT", "EXCEPT":
		writeIndent(b, base)
		b.WriteString(c.keyword)
		b.WriteByte('\n')
		if len(c.body) > 0 {
			if c.body[0].grp != nil && c.body[0].grp.kind == pkSubquery {
				emitParenSubqueryOperand(b, c.body, base)
			} else {
				pStmt(b, c.body, base)
			}
		}
		writeTerminator(b, c.trailer)
	case "WITH":
		emitWith(b, c, base)
	default:
		// Fallback for unrecognized statement (e.g. DROP TABLE).
		if len(c.head) == 0 && len(c.body) > 0 && c.body[0].grp != nil && c.body[0].grp.kind == pkSubquery {
			emitParenSubqueryOperand(b, c.body, base)
			writeTerminator(b, c.trailer)
			return
		}
		writeIndent(b, base)
		if len(c.head) > 0 {
			b.WriteString(inline(c.head))
		}
		if len(c.body) > 0 {
			if len(c.head) > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(inline(c.body))
		}
		if len(c.head) == 0 && len(c.body) == 0 {
			b.WriteByte(' ')
		}
		b.WriteByte('\n')
		writeTerminator(b, c.trailer)
	}
}

// emitParenSubqueryOperand prints a parenthesized subquery operand: the
// opening paren on its own line, the sub-statement indented, and the closing
// paren back at base, with any trailing items (e.g. a FROM alias) inline
// after it. body[0] must be a pkSubquery group.
func emitParenSubqueryOperand(b *bytes.Buffer, body []item, base int) {
	writeIndent(b, base)
	b.WriteString("(\n")
	pStmt(b, body[0].grp.items, base+2)
	writeIndent(b, base)
	b.WriteByte(')')
	if len(body) > 1 {
		b.WriteByte(' ')
		b.WriteString(inline(body[1:]))
	}
	b.WriteByte('\n')
}

// ----------------------------------------------------------------------
// List and expression emit.

// emitCommaList emits items split on top-level commas, one per line.
func emitCommaList(b *bytes.Buffer, items []item, indent int) {
	parts := splitTopComma(items)
	for i, p := range parts {
		writeIndent(b, indent)
		emitExpr(b, p, indent)
		if i < len(parts)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
}

func splitTopComma(items []item) [][]item {
	var out [][]item
	var cur []item
	for _, it := range items {
		if it.tok != nil && it.tok.val == "," {
			out = append(out, cur)
			cur = nil
			continue
		}
		cur = append(cur, it)
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

// emitExpr emits a single expression at the given indent. Handles nested
// groups (functions, predicate groups, subqueries) and CASE.
func emitExpr(b *bytes.Buffer, items []item, indent int) {
	if len(items) > 0 && items[0].isComment() {
		b.WriteString(items[0].tok.val)
		b.WriteByte('\n')
		writeIndent(b, indent)
		emitExpr(b, items[1:], indent)
		return
	}
	s := inline(items)
	fits := currentCol(b)+len(s) <= lineLimit && !hasSubquery(items)
	// Long top-level `+` chains (3+ operands) wrap one operand per line so a
	// trailing CASE doesn't trap the preceding chain in a single inline run.
	// Skip when AND/OR are present so predicate splitting drives the layout.
	if !fits && !hasTopAndOr(items) && hasTopArithChain(items) {
		parts := splitArithOps(items)
		for i, p := range parts {
			if i > 0 {
				b.WriteByte('\n')
				writeIndent(b, indent)
				b.WriteString(p.conn)
				b.WriteByte(' ')
			}
			emitExpr(b, p.body, indent)
		}
		return
	}
	if cs, ce := findCaseEnd(items); cs >= 0 {
		if cs > 0 {
			b.WriteString(inline(items[:cs]))
			b.WriteByte(' ')
		}
		emitCase(b, items[cs:ce+1], indent)
		if ce+1 < len(items) {
			b.WriteByte(' ')
			b.WriteString(inline(items[ce+1:]))
		}
		return
	}
	if fits {
		b.WriteString(s)
		return
	}
	if hasTopAndOr(items) {
		parts := splitPredicates(items)
		for i, p := range parts {
			if i > 0 {
				b.WriteByte('\n')
				writeIndent(b, indent)
				b.WriteString(p.conn)
				b.WriteByte(' ')
			}
			emitExpr(b, p.body, indent)
		}
		return
	}
	emitWrapped(b, items, indent)
}

// hasTopArithChain reports whether items contain 2+ top-level binary `+`
// operators, indicating a real chain rather than a single `x + 1`.
func hasTopArithChain(items []item) bool {
	n := 0
	for i, it := range items {
		if it.tok != nil && it.tok.kind == tkOp && it.tok.val == "+" && i > 0 {
			n++
			if n >= 2 {
				return true
			}
		}
	}
	return false
}

// splitArithOps splits on top-level binary `+`. Leading `+` (unary) stays
// attached to its operand.
func splitArithOps(items []item) []predPart {
	var out []predPart
	cur := predPart{}
	for i, it := range items {
		if it.tok != nil && it.tok.kind == tkOp && it.tok.val == "+" && i > 0 && len(cur.body) > 0 {
			out = append(out, cur)
			cur = predPart{conn: it.tok.val}
			continue
		}
		cur.body = append(cur.body, it)
	}
	if len(cur.body) > 0 {
		out = append(out, cur)
	}
	return out
}

func hasSubquery(items []item) bool {
	for _, it := range items {
		if it.grp != nil && it.grp.kind == pkSubquery {
			return true
		}
	}
	return false
}

func hasTopAndOr(items []item) bool {
	for _, it := range items {
		if it.tok != nil && it.tok.kind == tkKeyword && (it.tok.val == "AND" || it.tok.val == "OR") {
			return true
		}
	}
	return false
}

// findCaseEnd returns the indices of a matching CASE...END at top level
// (groups are already collapsed into item{grp:...}). Returns (-1, -1) if
// there is no CASE.
func findCaseEnd(items []item) (start, end int) {
	start = -1
	depth := 0
	for i, it := range items {
		if it.isKW("CASE") {
			if start < 0 {
				start = i
			}
			depth++
		} else if it.isKW("END") {
			depth--
			if depth == 0 {
				return start, i
			}
		}
	}
	return -1, -1
}

// emitWrapped emits items inline with selective group expansion.
func emitWrapped(b *bytes.Buffer, items []item, indent int) {
	for i := range items {
		if i > 0 && needSpace(items[i-1], items[i]) {
			b.WriteByte(' ')
		}
		if items[i].tok != nil {
			b.WriteString(items[i].tok.val)
			continue
		}
		if items[i].grp != nil {
			emitGroup(b, items[i].grp, indent)
		}
	}
}

// emitGroup decides inline vs wrapped based on length and group kind.
func emitGroup(b *bytes.Buffer, g *group, indent int) {
	// Subqueries always wrap; the goldens never inline a SELECT inside parens.
	if g.kind == pkSubquery {
		emitSubqueryWrapped(b, g, indent)
		return
	}
	inlineStr := "(" + inline(g.items) + ")"
	if currentCol(b)+len(inlineStr) <= lineLimit {
		b.WriteString(inlineStr)
		return
	}
	switch g.kind {
	case pkFunc:
		emitFuncWrapped(b, g, indent)
	case pkPred:
		emitPredWrapped(b, g, indent)
	default:
		if isWindowSpec(g.items) {
			emitWindowSpecWrapped(b, g.items, indent)
			return
		}
		emitListWrapped(b, g, indent)
	}
}

// emitFuncWrapped wraps a function call across multiple lines, optionally
// in paired form (two args per line) for paired-args functions.
func emitFuncWrapped(b *bytes.Buffer, g *group, indent int) {
	b.WriteString("(\n")
	args := splitTopComma(g.items)
	inner := indent + 2
	if pairedFns[g.fn] && len(args)%2 == 0 && len(args) >= 4 {
		for i := 0; i < len(args); i += 2 {
			writeIndent(b, inner)
			emitExpr(b, args[i], inner)
			b.WriteString(", ")
			emitExpr(b, args[i+1], inner)
			if i+2 < len(args) {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
	} else {
		for i, a := range args {
			writeIndent(b, inner)
			emitExpr(b, a, inner)
			if i < len(args)-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
	}
	writeIndent(b, indent)
	b.WriteByte(')')
}

func isWindowSpec(items []item) bool {
	return indexTopKeywordSeq(items, "ORDER", "BY") >= 0
}

func emitWindowSpecWrapped(b *bytes.Buffer, items []item, indent int) {
	orderIdx := indexTopKeywordSeq(items, "ORDER", "BY")
	if orderIdx < 0 {
		emitListWrapped(b, &group{items: items}, indent)
		return
	}

	b.WriteString("(\n")
	inner := indent + 2

	prefix := items[:orderIdx]
	if len(prefix) > 0 {
		writeIndent(b, inner)
		emitExpr(b, prefix, inner)
		b.WriteByte('\n')
	}

	writeIndent(b, inner)
	b.WriteString("ORDER BY\n")

	orderItems := items[orderIdx+2:]
	parts := splitTopComma(orderItems)
	for i, p := range parts {
		writeIndent(b, inner+2)
		emitExpr(b, p, inner+2)
		if i < len(parts)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}

	writeIndent(b, indent)
	b.WriteByte(')')
}

// emitListWrapped wraps a comma-separated list (VALUES, IN, etc.).
func emitListWrapped(b *bytes.Buffer, g *group, indent int) {
	b.WriteString("(\n")
	parts := splitTopComma(g.items)
	inner := indent + 2
	for i, p := range parts {
		writeIndent(b, inner)
		emitExpr(b, p, inner)
		if i < len(parts)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	writeIndent(b, indent)
	b.WriteByte(')')
}

// emitPredWrapped wraps a parenthesized predicate group with AND/OR
// continuations.
func emitPredWrapped(b *bytes.Buffer, g *group, indent int) {
	b.WriteString("(\n")
	emitPredicateList(b, g.items, indent+2)
	writeIndent(b, indent)
	b.WriteByte(')')
}

// emitSubqueryWrapped emits a sub-statement.
func emitSubqueryWrapped(b *bytes.Buffer, g *group, indent int) {
	b.WriteString("(\n")
	pStmt(b, g.items, indent+2)
	writeIndent(b, indent)
	b.WriteByte(')')
}

// emitPredicateList emits predicates split on top-level AND/OR.
// A parenthesized predicate child always force-wraps for visual
// consistency with its siblings.
func emitPredicateList(b *bytes.Buffer, items []item, indent int) {
	parts := splitPredicates(items)
	for i, p := range parts {
		writeIndent(b, indent)
		if i > 0 {
			b.WriteString(p.conn)
			b.WriteByte(' ')
		}
		if len(p.body) == 1 && p.body[0].grp != nil && p.body[0].grp.kind == pkPred {
			emitPredWrapped(b, p.body[0].grp, indent)
		} else {
			emitExpr(b, p.body, indent)
		}
		b.WriteByte('\n')
	}
}

type predPart struct {
	conn string // "" for first, "AND" or "OR" otherwise
	body []item
}

func splitPredicates(items []item) []predPart {
	var out []predPart
	cur := predPart{}
	for _, it := range items {
		if it.tok != nil && it.tok.kind == tkKeyword && (it.tok.val == "AND" || it.tok.val == "OR") {
			out = append(out, cur)
			cur = predPart{conn: it.tok.val}
			continue
		}
		cur.body = append(cur.body, it)
	}
	if len(cur.body) > 0 {
		out = append(out, cur)
	}
	return out
}

// ----------------------------------------------------------------------
// FROM clause and JOINs.

func emitFromClause(b *bytes.Buffer, c clause, base int) {
	parts := splitFromJoins(c.body)
	// FROM (SELECT ...) alias: open paren ends FROM line, body at base+2,
	// closer at base, alias trails.
	if len(parts) == 1 && len(parts[0]) > 0 && parts[0][0].grp != nil &&
		parts[0][0].grp.kind == pkSubquery {
		writeIndent(b, base)
		b.WriteString("FROM (\n")
		pStmt(b, parts[0][0].grp.items, base+2)
		writeIndent(b, base)
		b.WriteByte(')')
		if len(parts[0]) > 1 {
			b.WriteByte(' ')
			b.WriteString(inline(parts[0][1:]))
		}
		b.WriteByte('\n')
		writeTerminator(b, c.trailer)
		return
	}
	writeIndent(b, base)
	b.WriteString("FROM\n")
	for _, p := range parts {
		if onIdx := joinOnIndex(p); onIdx >= 0 {
			emitJoinPart(b, p, onIdx, base+2)
			continue
		}
		writeIndent(b, base+2)
		emitExpr(b, p, base+2)
		b.WriteByte('\n')
	}
	writeTerminator(b, c.trailer)
}

func joinOnIndex(items []item) int {
	if len(items) == 0 {
		return -1
	}
	if !(items[0].isKW("JOIN", "LEFT", "RIGHT", "INNER", "OUTER", "FULL", "CROSS")) {
		return -1
	}
	for i := range len(items) {
		if items[i].isKW("ON") {
			return i
		}
	}
	return -1
}

func emitJoinPart(b *bytes.Buffer, items []item, onIdx, indent int) {
	head := items[:onIdx]
	pred := items[onIdx+1:]

	writeIndent(b, indent)
	emitExpr(b, head, indent)
	b.WriteByte('\n')

	writeIndent(b, indent+2)
	b.WriteString("ON ")
	parts := splitJoinAnd(partsWithoutLeadingAnd(pred))
	if len(parts) == 0 {
		b.WriteByte('\n')
		return
	}
	emitExpr(b, parts[0], indent+2)
	b.WriteByte('\n')
	for _, p := range parts[1:] {
		writeIndent(b, indent+2)
		b.WriteString("AND ")
		emitExpr(b, p, indent+2)
		b.WriteByte('\n')
	}
}

func partsWithoutLeadingAnd(items []item) []item {
	if len(items) > 0 && items[0].isKW("AND") {
		return items[1:]
	}
	return items
}

func splitJoinAnd(items []item) [][]item {
	var out [][]item
	var cur []item
	for _, it := range items {
		if it.isKW("AND") {
			if len(cur) > 0 {
				out = append(out, cur)
			}
			cur = nil
			continue
		}
		cur = append(cur, it)
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

// splitFromJoins splits the FROM body where each JOIN starts a new entry.
// The first part is the base table (or sub-select); subsequent parts each
// begin with a JOIN keyword chain.
func splitFromJoins(items []item) [][]item {
	var out [][]item
	var cur []item
	for i := range len(items) {
		it := items[i]
		startHere := false
		if it.isKW("JOIN") {
			if len(cur) == 0 || !cur[len(cur)-1].isKW("LEFT", "RIGHT", "INNER", "OUTER", "FULL", "CROSS") {
				startHere = true
			}
		} else if it.isKW("LEFT", "RIGHT", "INNER", "OUTER", "FULL", "CROSS") && isJoinAhead(items, i) {
			startHere = true
		}
		if startHere && len(cur) > 0 {
			out = append(out, cur)
			cur = nil
		}
		cur = append(cur, it)
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

func isJoinAhead(items []item, i int) bool {
	for j := i + 1; j < len(items) && j < i+4; j++ {
		if items[j].isKW("JOIN") {
			return true
		}
		if items[j].tok != nil && items[j].tok.kind != tkKeyword {
			return false
		}
	}
	return false
}

// ----------------------------------------------------------------------
// INSERT, ON CONFLICT, VALUES, WITH.

// emitOnConflict handles ON CONFLICT [(cols)] DO {NOTHING | UPDATE SET ...}.
func emitOnConflict(b *bytes.Buffer, c clause, base int) {
	writeIndent(b, base)
	b.WriteString("ON CONFLICT")
	body := c.body
	if len(body) > 0 && body[0].grp != nil && body[0].grp.kind == pkList {
		b.WriteByte(' ')
		emitListWrapped(b, body[0].grp, base)
		body = body[1:]
	}
	b.WriteByte('\n')
	if len(body) == 0 {
		writeTerminator(b, c.trailer)
		return
	}

	doIdx := -1
	for i, it := range body {
		if it.isKW("DO") {
			doIdx = i
			break
		}
	}

	pred := body
	action := []item(nil)
	if doIdx >= 0 {
		pred = body[:doIdx]
		action = body[doIdx:]
	}

	if len(pred) > 0 {
		if pred[0].isKW("WHERE") {
			writeIndent(b, base)
			b.WriteString("WHERE\n")
			emitPredicateList(b, pred[1:], base+2)
		} else {
			writeIndent(b, base+2)
			b.WriteString(inline(pred))
			b.WriteByte('\n')
		}
	}

	if len(action) >= 2 && action[0].isKW("DO") && action[1].isKW("NOTHING") {
		writeIndent(b, base)
		b.WriteString("DO NOTHING\n")
		writeTerminator(b, c.trailer)
		return
	}
	if len(action) >= 3 && action[0].isKW("DO") && action[1].isKW("UPDATE") && action[2].isKW("SET") {
		writeIndent(b, base)
		if len(pred) == 0 {
			b.WriteString("DO UPDATE SET\n")
		} else {
			b.WriteString("DO UPDATE\n")
			writeIndent(b, base)
			b.WriteString("SET\n")
		}
		emitCommaList(b, action[3:], base+2)
		writeTerminator(b, c.trailer)
		return
	}

	if len(action) > 0 {
		writeIndent(b, base)
		b.WriteString(inline(action))
		b.WriteByte('\n')
	}
	writeTerminator(b, c.trailer)
}

// emitInsertInto handles INSERT INTO <table> [(col list)]. Column list
// wraps when 2+ columns; single-column stays inline.
func emitInsertInto(b *bytes.Buffer, c clause, base int) {
	writeIndent(b, base)
	b.WriteString("INSERT INTO ")
	body := c.body
	if len(body) == 0 || body[0].tok == nil {
		// Defensive: emit inline rather than panic on unexpected shape.
		b.WriteString(inline(body))
		b.WriteByte('\n')
		writeTerminator(b, c.trailer)
		return
	}
	b.WriteString(body[0].tok.val)
	body = body[1:]
	if len(body) > 0 && body[0].grp != nil && body[0].grp.kind == pkList {
		parts := splitTopComma(body[0].grp.items)
		if len(parts) == 1 {
			b.WriteString(" (")
			b.WriteString(inline(parts[0]))
			b.WriteByte(')')
		} else {
			b.WriteString(" (\n")
			for i, p := range parts {
				writeIndent(b, base+2)
				b.WriteString(inline(p))
				if i < len(parts)-1 {
					b.WriteByte(',')
				}
				b.WriteByte('\n')
			}
			writeIndent(b, base)
			b.WriteByte(')')
		}
		body = body[1:]
	}
	b.WriteByte('\n')
	if len(body) > 0 {
		writeIndent(b, base)
		b.WriteString(inline(body))
		b.WriteByte('\n')
	}
	writeTerminator(b, c.trailer)
}

// emitValues handles VALUES (tuple1), (tuple2), ...
// Single-tuple, single-arg case indents VALUES +2 from base; otherwise
// VALUES sits at column 0 with each tuple wrapped one-per-line.
func emitValues(b *bytes.Buffer, c clause, base int) {
	parts := splitTopComma(c.body)
	if len(parts) == 1 && len(parts[0]) == 1 && parts[0][0].grp != nil {
		g := parts[0][0].grp
		if tup := splitTopComma(g.items); len(tup) == 1 {
			writeIndent(b, base+2)
			b.WriteString("VALUES (")
			b.WriteString(inline(tup[0]))
			b.WriteString(")\n")
			writeTerminator(b, c.trailer)
			return
		}
	}
	writeIndent(b, base)
	b.WriteString("VALUES")
	for i, p := range parts {
		if i == 0 {
			b.WriteByte(' ')
		} else {
			b.WriteString(",\n")
			writeIndent(b, base)
		}
		if len(p) == 1 && p[0].grp != nil {
			emitListWrapped(b, p[0].grp, base)
		} else {
			b.WriteString(inline(p))
		}
	}
	b.WriteByte('\n')
	writeTerminator(b, c.trailer)
}

// emitWith handles WITH [RECURSIVE] name AS (subquery)
// [, name2 AS (subquery)] <body>.
func emitWith(b *bytes.Buffer, c clause, base int) {
	writeIndent(b, base)
	b.WriteString("WITH")
	body := c.body
	if len(body) > 0 && isRecursiveModifier(body[0]) {
		b.WriteString(" RECURSIVE")
		body = body[1:]
	}
	firstCTE := true
	for len(body) > 0 {
		if body[0].isComment() {
			b.WriteString(body[0].tok.val)
			b.WriteByte('\n')
			writeIndent(b, base)
			body = body[1:]
			firstCTE = false
			continue
		}
		name := body[0]
		if name.tok == nil {
			break
		}
		if firstCTE {
			b.WriteByte(' ')
		}
		b.WriteString(name.tok.val)
		body = body[1:]

		if len(body) > 1 && body[0].grp != nil && body[1].isKW("AS") {
			b.WriteByte('(')
			b.WriteString(inline(body[0].grp.items))
			b.WriteByte(')')
			body = body[1:]
		}

		if len(body) < 2 || !body[0].isKW("AS") || body[1].grp == nil {
			break
		}
		b.WriteString(" AS (\n")
		pStmt(b, body[1].grp.items, base+2)
		writeIndent(b, base)
		b.WriteByte(')')
		body = body[2:]
		if len(body) > 0 && body[0].isTok(",") {
			body = body[1:]
			b.WriteString(",\n")
			writeIndent(b, base)
			firstCTE = false
			continue
		}
		break
	}
	b.WriteByte('\n')
	if len(body) > 0 {
		pStmt(b, body, base)
	}
	writeTerminator(b, c.trailer)
}

func isRecursiveModifier(it item) bool {
	return it.tok != nil && strings.EqualFold(it.tok.val, "recursive")
}

// ----------------------------------------------------------------------
// CASE block.

// emitCase emits a CASE expression in the locked form:
//
//	col = CASE
//	  WHEN cond THEN
//	    value
//	  ELSE
//	    value
//	  END
//
// A simple CASE keeps its operand on the CASE line:
//
//	CASE col
//	  WHEN 'a' THEN
//	    1
//	  END
//
// `indent` is the column where the line containing CASE starts.
func emitCase(b *bytes.Buffer, items []item, indent int) {
	whenIndent := indent + 2
	valueIndent := indent + 4
	i, n := 1, len(items)
	opStart := i
	i = nextCaseToken(items, i, "WHEN", "ELSE")
	b.WriteString("CASE")
	if i > opStart {
		b.WriteByte(' ')
		b.WriteString(inline(items[opStart:i]))
	}
	b.WriteByte('\n')
	for i < n {
		switch {
		case items[i].isKW("WHEN"):
			i++
			condStart := i
			i = nextCaseToken(items, i, "THEN")
			cond := items[condStart:i]
			if i < n && items[i].isKW("THEN") {
				i++
			}
			valStart := i
			i = nextCaseToken(items, i, "WHEN", "ELSE")
			writeIndent(b, whenIndent)
			b.WriteString("WHEN ")
			emitWhenCondition(b, cond, whenIndent, valueIndent)
			b.WriteString(" THEN\n")
			writeIndent(b, valueIndent)
			emitCaseValue(b, items[valStart:i], valueIndent)
			b.WriteByte('\n')
		case items[i].isKW("ELSE"):
			i++
			elseStart := i
			i = nextCaseToken(items, i)
			writeIndent(b, whenIndent)
			b.WriteString("ELSE\n")
			writeIndent(b, valueIndent)
			emitCaseValue(b, items[elseStart:i], valueIndent)
			b.WriteByte('\n')
		case items[i].isKW("END"):
			writeIndent(b, whenIndent)
			b.WriteString("END")
			return
		}
	}
}

// nextCaseToken scans items from i for the next top-level keyword in
// stops, skipping nested CASE ... END blocks so a CASE embedded in a
// branch value or WHEN condition does not terminate the enclosing CASE
// early. A top-level END always stops the scan because it closes the
// enclosing CASE.
func nextCaseToken(items []item, i int, stops ...string) int {
	depth := 0
	for ; i < len(items); i++ {
		switch {
		case items[i].isKW("CASE"):
			depth++
		case items[i].isKW("END"):
			if depth > 0 {
				depth--
				continue
			}
			return i
		case depth == 0 && items[i].isKW(stops...):
			return i
		}
	}
	return i
}

// emitWhenCondition emits a CASE WHEN condition. Short conditions inline;
// long ones (AND/OR chain, NOT EXISTS subquery) wrap with continuations at
// the value column. Subqueries in the condition always force wrap.
func emitWhenCondition(b *bytes.Buffer, items []item, whenIndent, valueIndent int) {
	s := inline(items)
	if currentCol(b)+len(s) <= lineLimit && !hasSubquery(items) {
		b.WriteString(s)
		return
	}
	parts := splitPredicates(items)
	if len(parts) == 0 {
		b.WriteString(s)
		return
	}
	emitExpr(b, parts[0].body, whenIndent)
	for _, p := range parts[1:] {
		b.WriteByte('\n')
		writeIndent(b, valueIndent)
		b.WriteString(p.conn)
		b.WriteByte(' ')
		emitExpr(b, p.body, valueIndent)
	}
}
