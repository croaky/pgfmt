module github.com/croaky/pgfmt/oracle

go 1.27.3

replace github.com/croaky/pgfmt => ..

require (
	github.com/croaky/pgfmt v0.0.0-00010101000000-000000000000
	github.com/pganalyze/pg_query_go/v6 v6.2.5
)

require google.golang.org/protobuf v1.33.0 // indirect
