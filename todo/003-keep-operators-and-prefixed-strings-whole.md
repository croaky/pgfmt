# Keep compound operators and prefixed strings whole

Fixes `find_01EwuDhhzBPEsE19q6yhBWpV` and
`find_01C59XCLXF98amxFcnccX1ZS`.

Suggested title: `security: keep compound operators and prefixed
strings whole`.

## Problem

`lex` splits two kinds of token that Postgres reads as one. The
printer then puts a space between the parts, and Postgres reads the
output in a different way. The round-trip check in `Format` lexes the
output with the same `lex`, so it passes.

- Operators. `matchOp` takes the longest entry of a fixed table that
  starts the input. Postgres takes the whole run of operator
  characters. So `a << 2` prints as `a < < 2`, `a <-> b` as
  `a < -> b`, and `x ~~ y` as `x ~ ~ y`.
- Prefixed strings. `lex` knows only the `E` prefix. `X'1F'` prints as
  `x '1F'`, which Postgres reads as a typed literal, not a bit string.
  `B''` and `N''` split the same way. `U&''` is an error today, but
  an operator fix alone makes it `u & ''`, an operator expression.

Both findings are in the same branch of `lex`, so one change fixes
them. The change replaces the operator table with a short rule. It
costs no speed and no dependency.

## Steps

1. Operators. Take the longest run of the Postgres operator characters
   ``~ ! @ # ^ & | ` ? + - * / % < > =``. Cut the run before an
   embedded `--` or `/*`. If the run has more than one character and
   ends in `+` or `-`, remove each trailing `+` and `-`, unless the run
   holds one of ``~ ! @ # ^ & | ` ? %``. These are the rules in the
   Postgres `scan.l`. Keep `::` and `.`, which are not operator
   characters. Remove the `operators` table.
2. Prefixed strings. At the start of a token, read `B'`, `X'`, `N'`,
   and `U&'` as one string token. `B''` and `X''` end at the first
   quote. `N''` and `U&''` use the plain-string rules. Print the prefix
   in uppercase, as `E` is now. Refuse a continued string for each
   prefix, as for `E''` and plain strings.
3. README. Add both rules to the list of lexer rules in "Correctness".

Add a table entry in `pgfmt_test.go` for each form: `<<`, `<<=`,
`<->`, `~~`, `!~~`, `#>>`, `!=-1`, `x - -1`, `x <- 1`, `B'101'`,
`X'1F'`, `N'abc'`, `U&'d\0061t'`, and a name that ends in `x` before a
quote.

## Behavior change

`#`, `&`, `|`, `^`, `` ` ``, and `@` alone were errors. They now lex
as operators. A tag notes it.
