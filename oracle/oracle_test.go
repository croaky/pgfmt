package oracle_test

import (
	"fmt"
	"strings"
	"testing"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/croaky/pgfmt"
)

type normTok struct {
	kind pg_query.Token
	text string
}

// scanTokens scans src with the Postgres 17 C lexer from libpg_query
// and returns the non-comment tokens normalized for case-insensitive
// SQL forms (keywords, unquoted identifiers, and string prefixes).
func scanTokens(src string) ([]normTok, error) {
	res, err := pg_query.Scan(src)
	if err != nil {
		return nil, err
	}
	out := make([]normTok, 0, len(res.Tokens))
	for _, tok := range res.Tokens {
		if tok.Token == pg_query.Token_SQL_COMMENT || tok.Token == pg_query.Token_C_COMMENT {
			continue
		}
		raw := src[tok.Start:tok.End]
		out = append(out, normTok{
			kind: tok.Token,
			text: normalizeText(tok, raw),
		})
	}
	return out, nil
}

func normalizeText(tok *pg_query.ScanToken, raw string) string {
	if tok.KeywordKind != pg_query.KeywordKind_NO_KEYWORD {
		return strings.ToLower(raw)
	}
	switch tok.Token {
	case pg_query.Token_IDENT:
		if len(raw) > 0 && raw[0] != '"' {
			return strings.ToLower(raw)
		}
	case pg_query.Token_SCONST, pg_query.Token_BCONST, pg_query.Token_XCONST, pg_query.Token_USCONST:
		if q := strings.IndexByte(raw, '\''); q > 0 {
			return strings.ToUpper(raw[:q]) + raw[q:]
		}
	}
	return raw
}

func checkOracle(t *testing.T, src string) {
	t.Helper()
	out, err := pgfmt.Format(src)
	if err != nil {
		return
	}
	inToks, inErr := scanTokens(src)
	outToks, outErr := scanTokens(out)
	if inErr != nil {
		if outErr == nil {
			t.Fatalf("pgfmt accepted input that Postgres scanner rejected:\nin:  %q\nout: %q\nerr: %v", src, out, inErr)
		}
		if errKind(inErr) != errKind(outErr) {
			t.Fatalf("Postgres scanner error changed:\nin:     %q\nout:    %q\ninErr:  %v\noutErr: %v", src, out, inErr, outErr)
		}
		return
	}
	if outErr != nil {
		t.Fatalf("Postgres scanner rejected formatted output:\nin:  %q\nout: %q\nerr: %v", src, out, outErr)
	}
	if len(inToks) != len(outToks) {
		t.Fatalf("Postgres token count changed (%d -> %d):\nin:  %q\nout: %q\ninToks:  %v\noutToks: %v",
			len(inToks), len(outToks), src, out, formatToks(inToks), formatToks(outToks))
	}
	for i := range inToks {
		if inToks[i] != outToks[i] {
			t.Fatalf("Postgres token %d changed (%v -> %v):\nin:  %q\nout: %q",
				i, inToks[i], outToks[i], src, out)
		}
	}
}

func errKind(err error) string {
	msg, _, _ := strings.Cut(err.Error(), " at or near ")
	return msg
}

func formatToks(toks []normTok) string {
	parts := make([]string, len(toks))
	for i, tok := range toks {
		parts[i] = fmt.Sprintf("%s(%q)", tok.kind.String(), tok.text)
	}
	return strings.Join(parts, " ")
}

var seeds = []string{
	"select id, name from t where x = 1 order by id desc;",
	"select a << 2, a <<= b, a <-> b, a ~~ b, a !~~ b, j #>> '{a}', @ x from t;",
	"select a !=-1, x - -1, x <- 1, a=-1 from t;",
	"select a +-- c\n b from t;",
	"select b'101', X'1F', n'abc', u&'d\\0061t', e'a' from t;",
	"select ax'1' from t;",
	"select 1 as x$t$;\n-- $t$; drop table t;",
	"select like$1 from t;",
	"select E'\\'' as x;\n-- ' ; drop table t;",
	"/* a /* b */ c */\nselect 1;",
	"select 1; -- x\rselect 2;",
	"select 1; -- x\r/* select 2; */\nselect 3;",
	"select 1; -- x\r\nselect 2;",
	"select 1_000_000, 0x1f, 0o777, 0b1010, 1.5e-3;",
	"select $body$ a $$ b $body$;",
	"select E'caf\\u00e9\\n', 'a\\\\', '\\n';",
	"select 1; -- done",
	"select f(1 /* a b */);",
	"-- it's a header\nselect 1;",
	"select E'\x95\x5c' as x; -- ' ; drop table t;",
	"select E'Á\\' as x; -- ' ; drop table t;",
	"select 'a\\' as x; -- ' ; drop table t;",
	"select 1; -- note\x00\nselect 2;",
	"select 1; -- it's done",
	"select f(1 /* a 'b' */);",
	"select 'a'\n'b';",
	"select 1e;",
	"select .5, .25e-2 from t;",
	"select a. 00000 from t;",
	"select 1 . a from t;",
	"select a . . b from t;",
	"0B0$$$$",
	"0x0$$$$",
	"0o0$$$$",
	"0b02",
	"0o08",
	"1$$$$",
	"1.5$$$$",
	"1e2$$$$",
	"..0",
	"1..2",
	". ..",
	"1._2",
	"E'0\\0'",
	"select $1abc;",
	"select $1_;",
	"select $1E'x';",
	"select $1e'a\\'b';",
	"select $1a$$b$$;",
	"select $1$$x$$;",
}

// FuzzOracle checks that for every input pgfmt.Format accepts, the
// Postgres 17 scanner reads the same non-comment tokens from input and
// output, or rejects both with the same error kind.
//
// pg_query.Scan uses UTF-8 with standard_conforming_strings = on. The
// off-setting and multibyte invariants stay covered by quoteEnd and
// TestMultibyteTrailBytesDisjointFromDelimiters in the root package.
func FuzzOracle(f *testing.F) {
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		checkOracle(t, src)
	})
}
