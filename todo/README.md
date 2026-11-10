# Todo

Each `todo/NNN-name.md` file is one change, numbered in the order the
changes should land. Open a file as one sockeye change, land it, and
delete the file in the commit that finishes it. Start the next file
past the highest number already used.

## Security findings

`claudesec show <id>` prints the attack path and the fix criteria of a
finding. Match a finding to its plan by its ID, not by its title.

- `find_01EwuDhhzBPEsE19q6yhBWpV`: `lex` splits a compound operator.
  Fix in [003](003-keep-operators-and-prefixed-strings-whole.md).
- `find_01C59XCLXF98amxFcnccX1ZS`: `lex` splits a prefixed string
  literal. Fix in
  [003](003-keep-operators-and-prefixed-strings-whole.md).
