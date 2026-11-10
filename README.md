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
clean working tree is not required. Without a flag, pgfmt prints every
named file, formatted, so `pgfmt in.sql > out.sql` always fills out.sql.

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

## Postgres versions

pgfmt reads the Postgres 17 and 18 grammar, and is tested against those
two servers. It accepts what they accept, so a file it formats can hold
a form an older server rejects: the underscore separator of `1_000_000`
and the radix prefixes of `0x1f`, `0o777`, and `0b1010` all arrived in
Postgres 16.

pgfmt reads a plain `'...'` string with `standard_conforming_strings`
on, the Postgres default since 9.1. With the setting off, a backslash
before a quote ends a string at a different byte. Do not format SQL
for a server with the setting off.

## Correctness

`Format` lexes its own output and compares the token stream to the
input's, ignoring comments. A mismatch is an error rather than a written
file, so a bug in the printer costs a failed check and not a query that
means something else.

That check cannot see a token the lexer split, since both halves survive
the trip. So the lexer refuses instead where a split would hide: a
letter against the end of a number, such as `1e`, is an error, and
Postgres rejects the same input.

The check also cannot see a token that ends at a different byte than
in Postgres. So the lexer ends each token where Postgres does. A name
continues through `$`, so `like$1` is one name. In an `E'...'` string,
a backslash escapes the next byte. Block comments nest. A `--` comment
ends at CR or LF. Postgres joins two strings that a newline separates,
and pgfmt refuses that form with an error.

## GitHub repo is a mirror

Development happens on [sockeye](https://sockeye.sh), a
self-hosted review and CI server, which holds in progress branches.
GitHub receives `main` and the tags so `go get` works.

## License

MIT
