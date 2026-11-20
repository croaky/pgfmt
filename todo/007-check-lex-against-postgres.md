# Check lex against the Postgres scanner

Fixes no finding. It finds the next disagreement between `lex` and
Postgres before a scanner or an attacker does. It can run in parallel
with [005](005-refuse-setting-dependent-strings.md): it adds files and
edits only the root `Checkfile`.

Suggested title: `ci: check lex against the Postgres scanner`.

## Problem

`Format` checks its output by a second pass of `lex`. That check
cannot see a token that `lex` ends at a different byte than Postgres
does, because both passes make the same mistake. The four findings
that `bb28ea3` fixed, the findings that `04fac75` fixed, and the three
that 005 fixes all had this cause.

Plan 001 in `ca8bd01` did not take a check against the real Postgres
scanner. It said the cost was more than the risk after its fix. The
three later findings show that the risk stayed.

## Decide first

Two oracles can read SQL the way Postgres does. Each claim below is a
hypothesis until a short test confirms it.

- `pg_query_go`, which wraps the Postgres scanner from `libpg_query`.
  It needs cgo and a C build. Its `Scan` function returns tokens with
  byte offsets. Confirm that it can read a file with
  `standard_conforming_strings = off`, and with a client encoding such
  as SJIS. If it reads only UTF-8 with the setting `on`, it does not
  check the cases of 005.
- `psql` against Postgres 17 and 18. It reads both settings and each
  client encoding, as a real run does. It splits statements but does
  not return tokens, so the check compares the statements that `psql`
  sends for the input and for the output. Confirm that the sockeye
  runners have both servers. `runner/exec.go` in sockeye runs `psql`,
  which suggests a server on each runner.

Pick the oracle that reads both settings. If both do, pick
`pg_query_go`: it needs no server and gives tokens, not statements.

## Steps

1. Add the check in a separate module, `oracle/` with its own
   `go.mod`. The root module keeps only the standard library and
   `github.com/croaky/is`, and no module that requires pgfmt gets the
   cgo dependency or a Postgres client.
2. Write a fuzz target. For each input that `Format` accepts, the
   oracle reads the input and the output under each reading:
   `standard_conforming_strings` `on` and `off`, and UTF-8 and SJIS.
   Under each reading, the input and the output must give the same
   tokens or statements, with whitespace, keyword case, and dropped
   comments left out.
3. Seed the corpus from the table in `pgfmt_test.go` and from the
   inputs in the fix criteria of each finding
   (`claudesec show <id>`).
4. Add an `oracle` entry to the root `Checkfile` that runs the seed
   corpus on each push. Add a `weekly_oracle` entry that fuzzes for a
   fixed time against `main`.

## Cost

A C build or two Postgres servers in CI. The push entry runs the seed
corpus only, which keeps it to a few seconds after the first build.
The weekly entry spends a fixed time on one runner slot. It adds one
module and one `Checkfile` entry to maintain.

## Options not taken

- A second lexer in Go, written from `scan.l`, as the oracle. Both
  lexers would come from the same reading of `scan.l`, so they can
  share a mistake.
- Fuzz `Format` for idempotence only. It finds layout bugs, not a
  token that ends at the wrong byte.
