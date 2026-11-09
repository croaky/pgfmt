# Todo

Each `todo/NNN-name.md` file is one change, numbered in the order the
changes should land. Open a file as one sockeye change, land it, and
delete the file in the commit that finishes it. Start the next file
past the highest number already used.

## Security findings

Claude Security scans the pgfmt project
(`spj_011Cgt9mmoUqC6ndyDhksPYf`). `claudesec show <id>` prints the
attack path and the fix criteria of a finding, so no copy lives here.
Match a finding to its plan by its ID, not by its title. Claude
Security can change a title.

After the change that fixes a finding lands, the owner runs
`claudesec resolve --reason fixed <id>`. A finding that this threat
model accepts gets `--reason accepted_risk` and a `--note` that names
the attacker, the consequence, and the boundary.

### Open findings

Each finding is high in the scanner. The plan says what to do and why.

- `find_01FtMt8bon8i8agQ2NmiQPqi`: the lexer ends an identifier at
  `$`. Fix in [001](001-lex-read-tokens-as-postgres-does.md).
- `find_01BJTpTpH35pzyDJ8V2XhU4L`: the lexer reads an `E''` string
  without backslash escapes. Fix in
  [001](001-lex-read-tokens-as-postgres-does.md). Accept the risk of
  a server with `standard_conforming_strings = off`.
- `find_01EuiEKRsgGd1bC9oD4ewzJ5`: the lexer does not nest block
  comments. Fix in [001](001-lex-read-tokens-as-postgres-does.md).
- `find_01MR63yy4k5BmciULQdu4cJX`: a bare CR does not end a `--`
  comment. Fix in [001](001-lex-read-tokens-as-postgres-does.md).
- `find_01L24cYpsPSZrLvY9LhVkJVv`: unbounded recursion overflows the
  goroutine stack. Fix in [002](002-limit-nesting-depth.md).
- `find_016ETjPcF26mYEzKu9Donz6S`: `inline` makes CPU time cubic in
  nesting depth. Fix in [002](002-limit-nesting-depth.md).
- `find_01K9Kdt3bHGeWBLPUsFfSZg3`: indent makes output quadratic in
  nesting depth. Fix in [002](002-limit-nesting-depth.md).

## Threat model

pgfmt is a public Go module and a CLI. It reads SQL text and writes
SQL text. It opens no socket, holds no secret, and runs no SQL. Two
repos format their query files with `pgfmt -w`, and their CI checks
the layout of those files. Any module can import `Format`.

pgfmt promises one thing: Postgres reads the output the same way it
reads the input. A break in that promise is a bug for every user. It
is a security bug when someone can choose the input.

### Adversaries, most likely first

1. A contributor whose SQL file pgfmt formats. The contributor can
   already write any SQL in a change. The boundary that pgfmt guards
   is review. A reviewer approves one text, and after `pgfmt -w` the
   database runs a different text. The consequence is SQL that no
   reviewer read, which runs with the role that applies the
   migration. The same bugs also damage honest files. A nested block
   comment or a file with CR line ends is enough.
2. A caller that passes untrusted text to `Format`, such as an editor
   or a service. No such caller is known. The boundary is the
   caller's process. `Format` must return an error for bad input. It
   must not crash the process or use memory and CPU out of
   proportion to the input. In the two repos, CI already runs the
   contributor's Go tests. A slow `pgfmt -c` there gives a
   contributor no new power. It only stops the contributor's own job.

### Accepted risks

- A server with `standard_conforming_strings = off`. pgfmt reads a
  plain `'...'` string with standard rules, which are the Postgres
  default since 9.1. On a server with the setting off, a backslash
  before a quote can end the string at a different byte. The
  attacker is a contributor. The consequence is the same as in
  adversary 1. The boundary is the server setting, which the
  deployer controls and pgfmt cannot see. Plan 001 adds a README
  line that names the requirement.
