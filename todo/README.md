# Todo

Each `todo/NNN-name.md` file is one change, numbered in the order the
changes should land. Open a file as one sockeye change, land it, and
delete the file in the commit that finishes it. Start the next file
past the highest number already used.

## Security findings

Claude Security scans the pgfmt project
(`spj_011Cgt9mmoUqC6ndyDhksPYf`). `claudesec show <id>` prints the
attack path and the fix criteria of a finding. Match a finding to its
plan by its ID, not by its title. Claude Security can change a title.

After the change that fixes a finding lands, bump pgfmt in each repo
that formats SQL with it. Then the owner runs
`claudesec resolve --reason fixed <id>`.

The scanner read all three findings at `f6efa2c`. Each one is still
present at that commit.

- `find_01CJeiFgwPhMJbA7DBLiZR7g` (medium): `quoteEnd` reads a 0x5C
  byte in an `E'...'` string as a backslash, also where a multibyte
  client encoding such as SJIS, BIG5, or GBK reads it as the second
  byte of a character. Fix in
  [005](005-refuse-setting-dependent-strings.md).
- `find_01LzCutBxyeFojkhjihtF7u5` (medium): a file that runs
  `SET standard_conforming_strings = off` makes a plain string end at
  a different byte in Postgres than in `quoteEnd`. A quote in a
  dropped trailing comment then becomes live. Fix in
  [005](005-refuse-setting-dependent-strings.md).
- `find_01RTfkB39UU8XXsWYQrCLvED` (low): `lex` accepts a NUL byte
  inside a comment or a string. `psql` discards the rest of the line
  after a NUL, so the formatted file runs statements that the input
  hid. Fix in [005](005-refuse-setting-dependent-strings.md).

[006](006-refuse-dropped-comments-with-quotes.md) and
[007](007-check-lex-against-postgres.md) fix no finding. They remove
the two steps that let each of these findings change SQL with no
error. 005 and 007 touch different files and can run in parallel. 006
edits `lex.go` near 005, so it lands after 005.

## One cause, three findings

`bb28ea3` and `04fac75` each made `lex` end a token where Postgres
does. These three findings go around those fixes with the same
pattern: a quote inside a trailing `--` comment, which `lex` drops.
A table entry in `pgfmt_test.go` checks the `bb28ea3` fix with that
pattern, after an `E'\''` string. The new findings reach the same
quote by a different reading of the string before it.

Three parts work together:

1. Postgres finds the end of a string from two session settings,
   `client_encoding` and `standard_conforming_strings`. `lex` assumes
   UTF-8 and `on`. The SQL file itself can change both settings with
   `SET`.
2. `Format` checks no bytes at its input. It accepts a NUL byte and
   input that is not valid UTF-8, which no Postgres client sends as
   written.
3. `lex` drops a trailing `--` comment, `buildTree` drops each comment
   inside parentheses, and `sameTokens` skips comments. The round-trip
   check in `Format` lexes the output with the same `lex`. So a `lex`
   that disagrees with Postgres passes its own check, and a dropped
   comment changes what Postgres reads with no error.

005 removes parts 1 and 2. 006 narrows part 3. 007 gives part 3 a
check that does not use `lex`.

## Threat model

pgfmt is a public Go module and a CLI. It reads SQL text and writes
SQL text. It opens no socket, holds no secret, and runs no SQL. Any
module can import `Format`.

1. A contributor whose SQL file pgfmt formats. The boundary is review.
   A reviewer approves one text, and `pgfmt -w` must not change what
   Postgres reads. The consequence of a break is SQL that no reviewer
   read, which runs with the role that runs the file. A slow
   `pgfmt -c` in CI gives the contributor no new power, because CI
   already runs the contributor's code.
2. A caller that passes untrusted text to `Format`, such as an editor
   or a service. The boundary is the caller's process. `Format` must
   return an error for bad input. It must not crash the process or use
   memory and CPU out of proportion to the input.

All three findings are breaks of adversary 1.

### The client that runs the file

How a formatted file runs decides whether a finding reaches a
database:

- `psql` sends one statement at a time. It applies a `SET` in the file
  to the statements after it, and it discards the rest of a line after
  a NUL byte. All three findings name `psql`.
- A Go program that runs one query file through pgx sends one
  statement with the extended protocol. Postgres refuses a second
  statement in that message, so a `SET` in a query file cannot change
  how a later statement is read. This is a hypothesis for the
  `client_encoding` case: pgx is expected to hold `client_encoding`
  at UTF8. A test that runs a query file with
  `SET client_encoding = 'SJIS'` through pgx decides it.

The repos that format SQL with pgfmt run it on query files that Go
code runs through pgx. A reader can still hand a query file to
`psql`. Any other caller of `Format` can run its output in any client.
pgfmt cannot see the client, so it must not depend on one.

### Accepted risks

None.

`ca8bd01` accepted the risk of a server with
`standard_conforming_strings = off`. It named the boundary as the
server setting, which the deployer controls. `find_01LzCut...`
shows that this boundary is wrong: the file itself turns the setting
off with `SET`, so the contributor controls it. This README
withdraws the acceptance. After 005, pgfmt refuses each string whose
end depends on the setting.
