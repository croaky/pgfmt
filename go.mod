module github.com/croaky/pgfmt

go 1.26

// Published in error and withdrawn. The module proxy keeps a copy of
// whatever it has already served, so saying so here is the only way to
// stop a resolver from choosing it.
retract v0.1.0
