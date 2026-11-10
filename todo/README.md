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
`claudesec resolve --reason fixed <id>`.

### Open findings

Each finding is high in the scanner. The plan says what to do and why.

- `find_01L24cYpsPSZrLvY9LhVkJVv`: unbounded recursion overflows the
  goroutine stack. Fix in [002](002-limit-nesting-depth.md).
- `find_016ETjPcF26mYEzKu9Donz6S`: `inline` makes CPU time cubic in
  nesting depth. Fix in [002](002-limit-nesting-depth.md).
- `find_01K9Kdt3bHGeWBLPUsFfSZg3`: indent makes output quadratic in
  nesting depth. Fix in [002](002-limit-nesting-depth.md).
