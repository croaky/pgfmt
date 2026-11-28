# Security

Tier: single-user local tool and rendering library.

`pgfmt` is a Go library and CLI (`cmd/pgfmt`) that formats Postgres SQL
from stdin or local files. It uses only the Go standard library and
makes no network requests or database connections.

## Adversaries

1. Crafted or deeply nested SQL input that causes panics, token
   corruption, or unbounded CPU and memory use during lexing or layout.
2. A compromised dependency in `tools.mod` or `oracle/go.mod`.

## Accepted risks

Resolve scanner findings in these areas as `accepted_risk` instead of
adding defensive code:

- **Caller-supplied paths and in-place writes**: `cmd/pgfmt` reads and
  rewrites the file paths passed by the caller. Path confinement and
  symlink TOCTOU guards on the user's own files are out of scope.
- **Local user compromise**: An attacker who already runs commands as
  the user owns the process, environment, and files.
- **SQL execution semantics**: `pgfmt` only reformats SQL text and
  verifies token preservation. It never executes SQL or validates query
  safety.
