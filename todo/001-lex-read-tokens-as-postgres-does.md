# Read tokens the way Postgres reads them

Fixes `find_01FtMt8bon8i8agQ2NmiQPqi`,
`find_01BJTpTpH35pzyDJ8V2XhU4L`, `find_01EuiEKRsgGd1bC9oD4ewzJ5`,
and `find_01MR63yy4k5BmciULQdu4cJX`.

Suggested title: `lex: end strings, comments, and names where Postgres
does`.

## Problem

`lex` and Postgres disagree on where four kinds of token end. After
the disagreement, `lex` reads code as string or comment, or reads
string or comment as code. The emitter then prints the wrong reading,
and Postgres reads the output that way. The round-trip check in
`Format` lexes the output with the same `lex`, so it passes.

All four findings have one cause and touch one function. One change
fixes them. The change adds about 30 lines to `lex.go`. It costs no
speed, no dependency, and no new check.

## Steps

1. Identifier. Postgres continues an identifier through `$`, but does
   not start one with `$`. Make the identifier loop in `lex` accept
   `$`. Look up the whole run in `keywords`. Then `like$t$` is one
   identifier, and a `$` against a word never opens a dollar quote or
   a `$1` parameter. Leave `dollarTag` as it is.
2. `E''` string. A backslash consumes the next byte, so `\'` never
   ends the string and never pairs with the next quote. An
   unterminated string stays an error.
3. Block comment. Keep a depth counter. `/*` raises it and `*/`
   lowers it. The comment ends at depth zero. A comment open at end
   of input stays an error.
4. Line comment. A `--` comment ends at the first CR or LF. A bare CR
   counts as a line break for `blanks`, so the trailing or standalone
   decision sees it. Count CR LF as one line break.
5. String continuation. Postgres joins two strings that only
   whitespace with a newline separates, and applies the `E` rules of
   the first to both. Make `lex` return an error for that input. The
   option to keep both segments as one token needs more code for a
   rare form. Search the two repos for it before the change lands.
6. README. State that pgfmt reads plain strings with
   `standard_conforming_strings = on`. The threat model accepts the
   other setting.

Add a table entry in `pgfmt_test.go` for each step. Each entry shows
the hide case or the un-hide case from the finding, and the output or
the error. Step 5 and an unterminated nested comment are error cases.

## Options for the comments that pgfmt drops

`lex` drops a trailing `--` comment, and `buildTree` drops each
comment inside parentheses. The round-trip check cannot see comments.
Once `lex` ends each comment where Postgres does, a dropped comment
is a whole token, and its loss changes no code.

- Fix `lex` only. This is the recommendation. It is the smallest
  change, and it meets the fix criteria of each finding.
- Also make `Format` return an error when it drops a comment whose
  text holds `/*`, `*/`, or CR. This adds about 5 lines. It catches a
  future gap in `lex`. It refuses some honest files, such as a
  trailing `-- see /* x */`.
- Remove the drop. `Format` returns an error for a trailing comment
  or a comment inside parentheses. This removes code, but it refuses
  files the two repos format now. Each repo must move those comments
  first.
- Compare the output with the real Postgres lexer through
  `pg_query_go`. This adds a cgo dependency and a C build to CI. The
  cost is more than the risk after the fix. Do not take it.
