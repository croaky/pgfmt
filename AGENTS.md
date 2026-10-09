# Agents guide

pgfmt formats Postgres SQL in one fixed style. See `README.md` for the
style and the command.

## Writing

Write every word in ASD-STE100 Simplified Technical English (STE):
Markdown docs, code comments, commit messages, change descriptions,
and replies in an agent conversation.
See <https://en.wikipedia.org/wiki/Simplified_Technical_English>.

STE is a controlled English for technical writing: one meaning per
word, one idea per sentence, and the actor named. It is not a house
style. It exists so every reader reads a sentence the same way. That
includes a tired reader, a reader in a second language, and an agent
that matches on words.

- One idea per sentence. Keep an instruction to 20 words and a
  description to 25.
- Active voice, present tense, and the actor named: say what acts,
  rather than writing "the token is refused".
- One word, one meaning. Keep a term the same everywhere rather than
  varying it for tone.
- Use the simple verb, not a noun made from it: "run the formatter",
  not "perform execution of the formatter".
- Cut what carries nothing: "simply", "just", "note that", "in order
  to".
- Put a warning or a limit before the step it applies to.
- STE limits a sentence, not a text. Keep each sentence short, but
  keep the sentence that defines a term or connects a cause to its
  effect.
- STE permits technical names. Name the clause, the token, or the
  function, not a vague noun such as "the snapshot".

Apply it to prose, not to code: an identifier, a command, and a quoted
error message stay as they are.

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
entries whose inputs you touched before committing, since a check that
fails locally fails there. Read the commands from the `Checkfile`
itself rather than from here, so the two cannot drift.

Two of the tools it names can write the fix. Run them that way first,
then let the checks confirm:

```sh
go run golang.org/x/tools/cmd/goimports@v0.45.0 -local "$(go list -m)" -w .
dprint fmt
```

The `lint` and `fmt` jobs only report, because a CI job that rewrites
source has nowhere to put it.

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

## Changes

Work happens on a sockeye change. `soc checkout` allocates one and
prints a worktree; `soc edit` sets its title and description. Do the
edit before the code, not after. A change with neither is a blank row on
the dashboard and a blank `soc show`, so nobody looking at either can
tell what it is or whether it overlaps what they are about to start. A
rough sentence beats an empty one, and the description gets rewritten
before the merge anyway.

Push with `soc push --wait`, which waits for the checks, rather than
sleeping and then reading. The server holds the request open
and answers within a second of the last check, so a sleep is either time
spent waiting for an answer that already arrived or too short to reach
one. Too short is the worse half: a `soc show` that lands before the
push is recorded reports the previous commit's checks, green, about the
wrong code. `--wait` follows the commit in the worktree it runs in,
exits nonzero when a check failed, and gives up after ten minutes
(`--timeout`). If `main` moved ahead, run `soc sync` to rebase the change,
then `soc push --force`. A plain push after a sync stops and says so.

## Commits

- Prefix with the stage the change acts on: `lex:`, `tree:`, `clause:`,
  `emit:`, `cmd:`, `doc:`, `ci:`. Not `pgfmt:` — every commit here is
  pgfmt, so it says nothing.
- Imperative mood, lowercase except proper nouns. Hard-wrap at 72.
- Include _why_, not just _what_. See `git log` for examples.
- Write the subject for a teammate who has not read the code. Name the
  clause, the flag, or the output, not a term the change makes up.
- Write the body as a change description (below). The first push of a
  branch with one commit copies the commit message to the change.
- Sign your work with a `Co-Authored-By` trailer.

## Change descriptions

sockeye squashes a change into one commit whose message is the
change's title and description (`soc edit`). Write the description as
that commit message.

Write for an engineer reading it a year from now. They know Go and
Git. They did not see your conversation, and they do not have the diff
open.

Put the most important fact first. A reader who stops after any
paragraph has the most important part so far. Use this order, and
leave out a part that does not apply:

1. The need. Name who uses what, what went wrong or was missing, and
   what the change does about it.
2. What a user sees now. Name the flag, the clause, or the output.
   Say what stops happening and what a user does differently. If no
   user sees the change, say what it protects: a test, a release, or a
   cost.
3. How it works. Name each part at its first mention, with its
   kind: "the `Format` Go function", "the `lint` check".
   Give the cause of a bug or the rule a feature applies, and the
   numbers that size the effect.
4. Next steps: a command to run, a follow-up plan by its path, or a
   known gap.

Most changes fit in three to five short paragraphs. To keep a
description short and clear:

- Define a project term at its first use, or use a plainer word.
- Leave out file lists, line counts, and code sizes. The diff shows
  them.
- Leave out what a reader of `main` cannot use: options you did not
  take, each edge case the tests cover, and "tests pass". Put a design
  argument in a code comment or a plan, and give its path.
- Use plain paragraphs. Git strips a `#` line as a comment, so a
  Markdown header disappears.
- Hard-wrap at 72 columns. Don't backslash-escape backticks. Use a
  quoted heredoc such as `<<'EOF'` with `soc edit`.
- Keep `Co-Authored-By` on the commits, not in the description. The
  merge collects the trailers from the commits it squashes.

## Releases

sockeye is origin and holds no tags. `scripts/tag vX.Y.Z` publishes one
annotated tag to the GitHub mirror, which is what a `go get` resolves.
