# Refuse strings whose end depends on a session setting

Fixes `find_01CJeiFgwPhMJbA7DBLiZR7g`,
`find_01LzCutBxyeFojkhjihtF7u5`, and `find_01RTfkB39UU8XXsWYQrCLvED`.

Suggested title: `security: refuse strings whose end depends on a
setting`.

## Problem

A contributor can write a SQL file that `pgfmt -w` rewrites into SQL
that Postgres reads differently, and `Format` reports success. Each
finding uses a byte that `lex` reads in one way and a Postgres client
reads in another way:

- `find_01CJei...`: in an `E'...'` string, `quoteEnd` reads every
  0x5C byte as a backslash. Under `client_encoding` SJIS, BIG5, GBK,
  GB18030, SHIFT_JIS_2004, JOHAB, or UHC, 0x5C can be the second byte
  of a two-byte character, and Postgres does not read it as a
  backslash. A valid UTF-8 file can be read this way: the last byte of
  `Á` (C3 81) is a lead byte in SJIS.
- `find_01LzCut...`: `quoteEnd` ends a plain or `N'...'` string with
  `standard_conforming_strings = on`. The file can run
  `SET standard_conforming_strings = off`, and `psql` then ends a
  string such as `'a\'` at a later quote.
- `find_01RTfk...`: `lex` accepts a NUL byte in a comment, a string,
  or a dollar-quoted body. `psql` discards the rest of a line after a
  NUL and joins the next line to it.

In each case, the trailing comment that `lex` drops holds a quote that
the other reading makes live. `README.md` says pgfmt assumes
`standard_conforming_strings = on`, but it does not say that pgfmt
assumes UTF-8, and no code enforces either assumption.

## Steps

1. Input bytes. At the start of `Format`, return an error for a NUL
   byte and for input that is not valid UTF-8 (`utf8.ValidString`).
   Give the offset of the first bad byte. `cmd/pgfmt` then leaves the
   file as it is, as it does for each other error.
2. `E'...'` string. In `quoteEnd`, return an error when a backslash
   directly follows a byte of 0x80 or above in the body. Under UTF-8
   that byte is the last byte of a character, and under a multibyte
   client encoding it can be a lead byte. The error says to write the
   character as a `\u` escape or to put an ASCII byte between it and
   the backslash.
3. Plain and `N'...'` string. In `quoteEnd`, find the end under both
   readings of `standard_conforming_strings`. With `off`, a backslash
   escapes the next byte. Return an error when the two ends differ.
   `'a\\'` and `'\n'` end at the same byte under both readings and
   still format. `'a\'` followed by more quoted text does not.
4. Under the `off` reading, a plain string meets the same multibyte
   problem as an `E'...'` string. Apply the rule of step 2 to the
   `off` reading of a plain string too.
5. Leave `B'...'`, `X'...'`, dollar-quoted, and `U&'...'` strings as
   they are. A backslash does not escape in the first three. Postgres
   refuses a `U&'...'` string when `standard_conforming_strings` is
   `off`, so that case gives an error and no other SQL.
6. README. State that pgfmt reads its input as UTF-8 and refuses a NUL
   byte. Replace "Do not format SQL for a server with the setting off"
   with the rule from steps 2 to 4: pgfmt refuses each string whose end
   depends on `client_encoding` or `standard_conforming_strings`.
7. Update the `quoteEnd` comment. Name the two settings and say why a
   string that depends on one is an error and not a guess.

## Tests

Add table entries in `pgfmt_test.go`, one for each fix criterion:

- `E'\x95\x5c'` followed by more quoted text on the same line: error.
  The input is not valid UTF-8, so step 1 refuses it.
- `E'Á\'` (C3 81 5C) followed by `-- '` on the same line: error, from
  step 2. This input is valid UTF-8.
- `E'caf\u00e9\n'` formats, and `E'café\n'` is an error.
- A plain string whose body ends in an odd run of backslashes, followed
  by a same-line `--` comment that holds a quote: error.
- `'a\\'` and `'\n'` format with no change.
- A NUL in a trailing `--` comment, in a nested block comment, and in a
  string literal: error for each.
- The existing `select E'\'' as x; -- ' ; drop table t;` entry still
  formats. The byte before its backslash is ASCII.

In `cmd/pgfmt/main_test.go`, check that `pgfmt -w` leaves a file with a
NUL byte untouched and exits nonzero.

Add one table test for the claim that steps 2 to 4 rest on. In each of
the encodings above, a second byte can be 0x5C, but no second byte is
`'`, `"`, `$`, `-`, `*`, `/`, CR, LF, or NUL. So once both readings
end each string at the same byte, every comment and every dollar quote
ends at the same byte under each encoding, and dropping a comment
changes nothing. Write the test from the second-byte ranges of each
encoding in the Postgres source (`src/common/wchar.c`).

## Cost

Two linear scans of the input, and a second end search only in a
plain string that holds a backslash. `BenchmarkFormat` must stay
within noise. About 30 lines in `lex.go` and `pgfmt.go`. No new
dependency and no new part.

It refuses honest input of two forms: a non-ASCII character directly
before a backslash in a string, and a plain string whose end differs
under `off`. On 2026-11-20, the repos that format SQL with pgfmt had
neither form in 2,184 query files, and no NUL byte.

## Options not taken

- Refuse every backslash in a plain string. It is simpler, but it
  refuses `'\n'` in a `LIKE` pattern or a regular expression, which the
  fix criteria keep.
- Read the input in the client encoding that a header or a flag names.
  It adds a flag that every caller must know, and the file can still
  change the encoding with `SET`.
- Keep the accepted risk for `standard_conforming_strings = off`. The
  file turns the setting off itself, so the boundary that the risk
  named does not hold. See `todo/README.md`.
