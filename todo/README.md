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

After the change that fixes a finding lands, the owner runs
`claudesec resolve --reason fixed <id>`.

- `find_01FMa4jqWZXzfAJC45pQNquP`: `lex` takes quadratic time when a
  block comment cuts a run of operator bytes. Fix in
  [004](004-lex-operator-runs-cut-by-comments-linearly.md).

## Threat model

pgfmt is a public Go module and a CLI. It reads SQL text and writes
SQL text. It opens no socket, holds no secret, and runs no SQL. Any
module can import `Format`.

1. A contributor whose SQL file pgfmt formats. The boundary is review.
   A reviewer approves one text, and `pgfmt -w` must not change what
   Postgres reads. A slow `pgfmt -c` in CI gives the contributor no
   new power, because CI already runs the contributor's code.
2. A caller that passes untrusted text to `Format`, such as an editor
   or a service. The boundary is the caller's process. `Format` must
   return an error for bad input. It must not crash the process or use
   memory and CPU out of proportion to the input.

`find_01FMa4jqWZXzfAJC45pQNquP` is a break of adversary 2: the
attacker sends a few hundred KB, and the consequence is one core
pinned for minutes for each call.
