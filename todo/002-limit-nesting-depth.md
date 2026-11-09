# Limit nesting depth and render a line once

Fixes `find_01L24cYpsPSZrLvY9LhVkJVv`,
`find_016ETjPcF26mYEzKu9Donz6S`, and `find_01K9Kdt3bHGeWBLPUsFfSZg3`.

Suggested title: `tree, emit: refuse SQL nested deeper than a limit`.

## Problem

Nothing limits nesting depth. `buildTree` recurses once for each `(`
and accepts a `(` that never closes. The emitters recurse over the
same tree. CASE nests without a group. Three results follow:

- Stack. A few MB of `(` overflows the goroutine stack. The Go
  runtime stops the process, and `recover` cannot catch it.
- CPU. `inline` builds each group in a new builder and copies it into
  the parent. `emitExpr` and `emitGroup` call `inline` on the whole
  subtree at each level to measure it. The cost is cubic in depth.
- Output. The indent grows at each level and `writeIndent` writes it
  on each line. Output is quadratic in depth.

All three findings have one cause: depth with no limit. One limit
fixes the stack and the output, and makes the CPU cost bounded. One
change holds them.

## Steps

1. Depth limit. Before `buildTree`, make `Format` count depth in one
   pass over the tokens. `(` and `CASE` raise it. `)` and `END` lower
   it, with a floor of zero, because `END` is also a statement. Return
   an error when depth passes the limit. Return an error for a `(`
   that never closes or a `)` with no `(`, as Postgres does.
2. Linear `inline`. Make `writeInlineItem` write a nested group into
   the same builder, not into a new string. Each byte is then copied
   once. This removes code and makes normal input faster.
3. Comment loop. Make `emitExpr` skip leading comments in a loop, not
   by a call to itself for each comment.

Add an entry in `pgfmt_test.go` for each case: 10,000 `(`, closed and
not closed, and 10,000 nested `CASE`. Each case returns the depth
error in less than 100 ms.

## The limit

Use 100. Measure the deepest query in the two repos first, and keep
the limit well above it. An honest query deeper than the limit gets
an error. That is the one cost to users.

With the limit, the indent on a line is at most about 200 spaces.
Output is then at most a constant multiple of the input. The
measure in `emitExpr` costs at most input size times the limit.

## Options for the remaining CPU cost

- Accept size times 100. This is the recommendation. It needs no new
  code past steps 1 to 3.
- Make the measure stop at `lineLimit`. A width function returns as
  soon as the width passes 80. This adds about 20 lines and makes
  each measure constant. Take it only if a benchmark shows a need.
- Store the width of each group on the `group` struct. This adds a
  field and code to keep it correct. Do not take it.
- Limit the input size. This is one line, but it refuses large honest
  files and does not fix depth. Do not take it.
