# Lex operator runs that a block comment cuts in linear time

Fixes `find_01FMa4jqWZXzfAJC45pQNquP`.

Suggested title: `security: lex operator runs cut by block comments linearly`.

## Problem

A caller that passes untrusted SQL to the `Format` Go function can pin
one CPU core for minutes. The input is an operator byte and an empty
block comment, repeated with no space: `+/**/+/**/...`.

The `matchOp` Go function scans the full run of `opChars` first. Then
it searches that run for `--` and `/*` and cuts the operator there.
`/` and `*` are in `opChars`, so the run continues through each
comment. `lex` reads the comment, then calls `matchOp` again, which
scans the rest of the same run again. The total cost is about `n²/10`
steps for `n` bytes.

Measured on `main` (3ff82e0): 10,000 repeats (50 KB) take 0.78 s, and
40,000 repeats (200 KB) take 12.3 s. Four times the input takes 16
times as long, so 1 MB takes about 5 minutes.

A cut at `--` is linear already, because the line comment reads to
the end of the line and so consumes the rest of the run. Only the cut
at `/*` is quadratic. The fix in 3ff82e0 made the `+` and `-` tail
linear, but not this cut.

## Steps

1. In `matchOp`, stop the scan at the first `--` or `/*`. Test the
   byte pair in the scan loop, before the loop takes the next byte.
   Remove the `strings.Index` loop that searched the run after the
   scan. Each call then reads only the bytes that `lex` consumes, plus
   one byte of lookahead.
2. Keep the tokens the same. The cut point, the `+` and `-` tail rule,
   and one token for each tail byte do not change. The current table
   entries for operators and comments must pass with no edit.
3. Extend `TestLexOperatorRunTime` with a second input,
   `"+/**/"` repeated 10,000 and 80,000 times, and the same 24 times
   limit. Check the count and values of the tokens, as the `+` case
   does.
4. Update the `matchOp` comment: name the cut at `/*` as a cause of
   quadratic time when the scan reads past it.

## Cost

No speed cost: the scan reads fewer bytes, and `BenchmarkLex` must
stay within noise. The code gets smaller by a few lines. No new
dependency and no new part. The new timing case adds about 0.1 s to
the test run.

## Options not taken

- An input size limit in `Format` or `cmd/pgfmt`. It adds a flag or a
  constant that every caller must know, and it refuses large files
  that are not attacks. It also leaves the quadratic path for inputs
  under the limit.
- Accept the risk. The fix is smaller than the code it replaces, so
  acceptance saves nothing.
