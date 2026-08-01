# pgfmt

A formatter for Postgres SQL files. One style, no options, so a diff
shows what the query changed and never how it was typed.

```sh
go get -tool github.com/croaky/pgfmt/cmd/pgfmt

go tool pgfmt -c file.sql ...   # check; exit 1 if any file would change
go tool pgfmt -w file.sql ...   # format in place
go tool pgfmt file.sql ...      # format to stdout
go tool pgfmt < file.sql        # format stdin to stdout
```

`-c` writes nothing and reports only formatting, never your edits, so a
clean working tree is not required.

The library is the same thing without the flags:

```go
out, err := pgfmt.Format(src)
```

## The style

Keywords uppercase. Two spaces per nesting level. A clause keyword owns
its line and its operands are indented under it, one per line, so adding
a column is a one-line diff:

```sql
SELECT
  id,
  name
FROM
  t
  JOIN u
    ON u.id = t.uid
WHERE
  t.x = 1
ORDER BY
  t.id DESC;
```

`CASE` always expands. An expression that fits stays on its line; one
that does not wraps at its arguments. Header comments above a statement
pass through. A trailing `--` comment does not: the printer joins tokens
onto a line, and a line comment would swallow what followed.

## Correctness

`Format` lexes its own output and compares the token stream to the
input's, ignoring comments. A mismatch is an error rather than a written
file, so a bug in the printer costs a failed check and not a query that
means something else.
