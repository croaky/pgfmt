# Refuse to drop a comment that holds a quote

Fixes no finding alone. It removes the step that turned each of
`find_01CJeiFgwPhMJbA7DBLiZR7g`, `find_01LzCutBxyeFojkhjihtF7u5`, and
`find_01RTfkB39UU8XXsWYQrCLvED` into changed SQL with no error. Land it
after [005](005-refuse-setting-dependent-strings.md), which edits the
same part of `lex.go`.

Suggested title: `lex, tree: refuse to drop a comment that holds a
quote`.

## Problem

`lex` drops a trailing `--` comment (`lex.go`, the line comment
branch), and `buildTree` drops each comment inside parentheses
(`tree.go`, the `depth > 0` case). `sameTokens` skips comments, so the
round-trip check in `Format` cannot see the drop.

When `lex` and Postgres agree on where each token ends, a dropped
comment is a whole comment, and its loss changes no SQL. When they
disagree, the dropped text can hold a quote that Postgres reads as
live. Each of the three findings puts its quote there. The fixes in
`bb28ea3`, `04fac75`, and 005 each close one disagreement. A
disagreement that nobody has found yet uses the same drop.

## Steps

1. Add one function that reports whether comment text holds a byte
   that can end or start a token in another reading: `'`, `"`, `$`,
   `\`, `/*`, or `*/`.
2. In the line comment branch of `lex`, return an error when a
   trailing comment holds such a byte, in place of dropping it. Give
   the offset.
3. In `buildTree`, return the same error for a comment inside
   parentheses. `buildTree` returns no error today, so `Format` checks
   the tokens for it before `buildTree` runs, in the pass that
   `checkDepth` already makes.
4. The error says to move the comment to its own line above the
   statement, where `pTop` keeps it.
5. README. Say which trailing and nested comments pgfmt refuses, next
   to the sentence that says a trailing `--` comment does not pass
   through.

## Tests

Add table entries in `pgfmt_test.go`:

- `select 1; -- it's done` is an error.
- `select 1; -- done` formats, and drops the comment as today.
- `select f(1 /* a 'b' */);` is an error, and
  `select f(1 /* a b */);` formats.
- A comment with a quote on its own line above a statement formats and
  passes through.

## Cost

About 15 lines. No speed cost: the check reads only the comments that
`lex` and `buildTree` drop.

It refuses an honest trailing comment with an apostrophe, such as
`-- don't`. Committed query files in the repos that format SQL with
pgfmt hold no trailing comment, because pgfmt drops each one before
the commit. An author meets the error while writing and moves the
comment above the statement.

## Options not taken

- Make `sameTokens` compare comments, so each drop fails the check. It
  refuses every trailing comment, also `-- done`, and the repos would
  need to move every one.
- Move a trailing comment to its own line in place of dropping it. The
  quote stays in the output, so a reading that disagrees with `lex`
  still finds it, at a new place.
- Keep the drop with no check. Each future disagreement in `lex` then
  becomes changed SQL with no error, as these three findings did.
