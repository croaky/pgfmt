# Agents guide

pgfmt formats Postgres SQL in one fixed style. See `README.md` for the
style and the command.

## Architecture

One package at the repo root, standard library only, plus
`cmd/pgfmt` for the flags. Source moves through three stages:

- `lex.go` — SQL text to tokens. Strings, dollar quotes, comments, and
  operators; it knows no grammar.
- `tree.go` — tokens to a nesting tree, with `clause.go` splitting a
  statement into its clauses.
- `emit.go` — the tree to text. Every layout decision is here.

`pgfmt.go` is `Format`, which runs the three and then re-lexes its own
output to prove the tokens survived.

## Checks

The root `Checkfile` is the list, and CI runs it on every push. Run the
same things before committing, since a check that fails locally fails
there:

```sh
goimports -local "$(go list -m)" -w .
go vet ./...
go test -race -cover ./...
git ls-files -z '*.go' | xargs -0 gopls check -severity=hint
```

The local `goimports` writes; the `lint` job only reports, because a CI
job that rewrites source has nowhere to put it.

The package imports nothing outside the standard library. The only
dependency is `github.com/croaky/is`, used for test assertions. Taking
another is a design decision, not a step.

## Tests

Red/green TDD. `pgfmt_test.go` is a table of input and output, one entry
per rule of the style. A layout change is a new entry, and an entry that
changes is a style change: two repos have their SQL formatted by this,
so the diff lands there.

Assertions come from `github.com/croaky/is`: `is := is.New(t)`, then
`is.Eq(got, want)`, `is.NoErr(err)`, `is.HasErr(err)`. Pick the helper
that names the check; `is.True` is for a predicate with no want.

## Commits

- Prefix with the stage the change acts on: `lex:`, `tree:`, `clause:`,
  `emit:`, `cmd:`, `doc:`, `ci:`. Not `pgfmt:` — every commit here is
  pgfmt, so it says nothing.
- Imperative mood, lowercase except proper nouns. Hard-wrap at 72.
- Include _why_, not just _what_. See `git log` for examples.
- Sign your work with a `Co-Authored-By` trailer.

## Releases

cibot is origin and holds no tags. `scripts/tag vX.Y.Z` publishes one
annotated tag to the GitHub mirror, which is what a `go get` resolves.
