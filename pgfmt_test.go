package pgfmt

import "testing"

// TestFormat covers each rule of the locked style with input → output.
func TestFormat(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{
			name: "keyword case + basic clauses",
			in:   "select id, name from t where x = 1 order by id desc;",
			want: "" +
				"SELECT\n" +
				"  id,\n" +
				"  name\n" +
				"FROM\n" +
				"  t\n" +
				"WHERE\n" +
				"  x = 1\n" +
				"ORDER BY\n" +
				"  id DESC;\n",
		},
		{
			name: "JOIN puts ON on a continuation line",
			in:   "select t.id from t join u on u.id = t.uid where t.x = 1;",
			want: "" +
				"SELECT\n" +
				"  t.id\n" +
				"FROM\n" +
				"  t\n" +
				"  JOIN u\n" +
				"    ON u.id = t.uid\n" +
				"WHERE\n" +
				"  t.x = 1;\n",
		},
		{
			name: "LATERAL is uppercased",
			in:   "select p.id from people p left join lateral (select 1) q on true;",
			want: "" +
				"SELECT\n" +
				"  p.id\n" +
				"FROM\n" +
				"  people p\n" +
				"  LEFT JOIN LATERAL (\n" +
				"    SELECT\n" +
				"      1\n" +
				"  ) q\n" +
				"    ON TRUE;\n",
		},
		{
			name: "DELETE USING keeps USING uppercase",
			in:   "delete from items using scoped where items.item_id = scoped.item_id;",
			want: "" +
				"DELETE FROM\n" +
				"  items\n" +
				"USING\n" +
				"  scoped\n" +
				"WHERE\n" +
				"  items.item_id = scoped.item_id;\n",
		},
		{
			name: "CASE always expands, END followed by AS keeps a space",
			in:   "select case when x = 1 then 'a' else 'b' end as v from t;",
			want: "" +
				"SELECT\n" +
				"  CASE\n" +
				"    WHEN x = 1 THEN\n" +
				"      'a'\n" +
				"    ELSE\n" +
				"      'b'\n" +
				"    END AS v\n" +
				"FROM\n" +
				"  t;\n",
		},
		{
			name: "simple CASE keeps its operand on the CASE line",
			in:   "select 1 from t order by case t.status when 'a' then 1 when 'b' then 2 else 8 end, t.id;",
			want: "" +
				"SELECT\n" +
				"  1\n" +
				"FROM\n" +
				"  t\n" +
				"ORDER BY\n" +
				"  CASE t.status\n" +
				"    WHEN 'a' THEN\n" +
				"      1\n" +
				"    WHEN 'b' THEN\n" +
				"      2\n" +
				"    ELSE\n" +
				"      8\n" +
				"    END,\n" +
				"  t.id;\n",
		},
		{
			name: "CASE long branch value uses wrapped expression formatting",
			in:   "select case when $2 <> '' then ts_headline('english', name || ' ' || coalesce(description, '') || ' ' || coalesce(status, '') || ' ' || coalesce(domain, ''), to_tsquery('english', $2), 'MaxWords=10, MinWords=5, MaxFragments=1') else null end as search_headline from doc_search;",
			want: "" +
				"SELECT\n" +
				"  CASE\n" +
				"    WHEN $2 <> '' THEN\n" +
				"      ts_headline(\n" +
				"        'english',\n" +
				"        name || ' ' || coalesce(description, '') || ' ' || coalesce(status, '') || ' ' || coalesce(\n" +
				"          domain,\n" +
				"          ''\n" +
				"        ),\n" +
				"        to_tsquery('english', $2),\n" +
				"        'MaxWords=10, MinWords=5, MaxFragments=1'\n" +
				"      )\n" +
				"    ELSE\n" +
				"      NULL\n" +
				"    END AS search_headline\n" +
				"FROM\n" +
				"  doc_search;\n",
		},
		{
			name: "nested CASE in a branch value keeps both END tokens",
			in:   "select case when x = 1 then case y when 'a' then 1 else 2 end else 3 end as v from t;",
			want: "" +
				"SELECT\n" +
				"  CASE\n" +
				"    WHEN x = 1 THEN\n" +
				"      CASE y WHEN 'a' THEN 1 ELSE 2 END\n" +
				"    ELSE\n" +
				"      3\n" +
				"    END AS v\n" +
				"FROM\n" +
				"  t;\n",
		},
		{
			name: "nested CASE that wraps indents inner branches and both ENDs",
			in: "select 1 from t order by case when $1 = 'status' and $2 = 'asc' then " +
				"case s.grade when 'A' then 1 when 'B' then 2 " +
				"when 'C' then 3 when 'D' then 4 else 5 end end asc nulls last;",
			want: "" +
				"SELECT\n" +
				"  1\n" +
				"FROM\n" +
				"  t\n" +
				"ORDER BY\n" +
				"  CASE\n" +
				"    WHEN $1 = 'status' AND $2 = 'asc' THEN\n" +
				"      CASE s.grade\n" +
				"        WHEN 'A' THEN\n" +
				"          1\n" +
				"        WHEN 'B' THEN\n" +
				"          2\n" +
				"        WHEN 'C' THEN\n" +
				"          3\n" +
				"        WHEN 'D' THEN\n" +
				"          4\n" +
				"        ELSE\n" +
				"          5\n" +
				"        END\n" +
				"    END ASC NULLS LAST;\n",
		},
		{
			name: "named-arg => operator keeps its spacing",
			in:   "select * from f(days => $1);",
			want: "" +
				"SELECT\n" +
				"  *\n" +
				"FROM\n" +
				"  f(days => $1);\n",
		},
		{
			name: "INSERT ... VALUES ($1) single-arg uses indented VALUES",
			in:   "insert into t (a) values ($1) on conflict do nothing;",
			want: "" +
				"INSERT INTO t (a)\n" +
				"  VALUES ($1)\n" +
				"ON CONFLICT\n" +
				"DO NOTHING;\n",
		},
		{
			name: "ON CONFLICT target WHERE formats separately from DO UPDATE",
			in: "insert into pages (user_id, title, item_id, summary) " +
				"values ($1, $2, $3, $4) " +
				"on conflict (item_id, title) " +
				"where title = 'summary' " +
				"do update set summary = excluded.summary;",
			want: "" +
				"INSERT INTO pages (\n" +
				"  user_id,\n" +
				"  title,\n" +
				"  item_id,\n" +
				"  summary\n" +
				")\n" +
				"VALUES (\n" +
				"  $1,\n" +
				"  $2,\n" +
				"  $3,\n" +
				"  $4\n" +
				")\n" +
				"ON CONFLICT (\n" +
				"  item_id,\n" +
				"  title\n" +
				")\n" +
				"WHERE\n" +
				"  title = 'summary'\n" +
				"DO UPDATE\n" +
				"SET\n" +
				"  summary = EXCLUDED.summary;\n",
		},
		{
			name: "paired jsonb_build_object wraps two args per line",
			in:   "select jsonb_build_object('user_id', $1::bigint, 'win_end', $2::text, 'phase', $3::text, 'next_url', $4::text) from t;",
			want: "" +
				"SELECT\n" +
				"  jsonb_build_object(\n" +
				"    'user_id', $1::bigint,\n" +
				"    'win_end', $2::text,\n" +
				"    'phase', $3::text,\n" +
				"    'next_url', $4::text\n" +
				"  )\n" +
				"FROM\n" +
				"  t;\n",
		},
		{
			name: "NOT EXISTS subquery always wraps",
			in:   "select 1 from t where not exists (select 1 from u where u.t_id = t.id);",
			want: "" +
				"SELECT\n" +
				"  1\n" +
				"FROM\n" +
				"  t\n" +
				"WHERE\n" +
				"  NOT EXISTS (\n" +
				"    SELECT\n" +
				"      1\n" +
				"    FROM\n" +
				"      u\n" +
				"    WHERE\n" +
				"      u.t_id = t.id\n" +
				"  );\n",
		},
		{
			name: "WITH RECURSIVE keeps CTE list structure",
			in:   "with recursive t as (select 1), u as (select 2) select * from t;",
			want: "" +
				"WITH RECURSIVE t AS (\n" +
				"  SELECT\n" +
				"    1\n" +
				"),\n" +
				"u AS (\n" +
				"  SELECT\n" +
				"    2\n" +
				")\n" +
				"SELECT\n" +
				"  *\n" +
				"FROM\n" +
				"  t;\n",
		},
		{
			name: "WITH CTE column list formats subquery body",
			in:   "with recent(item_id) as (select item_id from notes where occurred_on >= now() - '1 year'::interval union select item_id from tracking where created_at >= now() - '1 year'::interval union select item_id from sessions where stopped_at >= now() - '1 year'::interval or stopped_at is null) insert into items (item_id, list_id) select records.id, $1 from records left join recent on recent.item_id = records.id where records.region is not null and not (records.region = any ($2::text[])) and recent.item_id is null on conflict do nothing;",
			want: "" +
				"WITH recent(item_id) AS (\n" +
				"  SELECT\n" +
				"    item_id\n" +
				"  FROM\n" +
				"    notes\n" +
				"  WHERE\n" +
				"    occurred_on >= now() - '1 year'::interval\n" +
				"  UNION\n" +
				"  SELECT\n" +
				"    item_id\n" +
				"  FROM\n" +
				"    tracking\n" +
				"  WHERE\n" +
				"    created_at >= now() - '1 year'::interval\n" +
				"  UNION\n" +
				"  SELECT\n" +
				"    item_id\n" +
				"  FROM\n" +
				"    sessions\n" +
				"  WHERE\n" +
				"    stopped_at >= now() - '1 year'::interval\n" +
				"    OR stopped_at IS NULL\n" +
				")\n" +
				"INSERT INTO items (\n" +
				"  item_id,\n" +
				"  list_id\n" +
				")\n" +
				"SELECT\n" +
				"  records.id,\n" +
				"  $1\n" +
				"FROM\n" +
				"  records\n" +
				"  LEFT JOIN recent\n" +
				"    ON recent.item_id = records.id\n" +
				"WHERE\n" +
				"  records.region IS NOT NULL\n" +
				"  AND NOT (records.region = ANY ($2::text[]))\n" +
				"  AND recent.item_id IS NULL\n" +
				"ON CONFLICT\n" +
				"DO NOTHING;\n",
		},
		{
			name: "WITH ORDINALITY stays in FROM expression",
			in:   "select * from jsonb_array_elements($1) WITH ORDINALITY as p(value, index);",
			want: "" +
				"SELECT\n" +
				"  *\n" +
				"FROM\n" +
				"  jsonb_array_elements($1) WITH ordinality AS p(value, index);\n",
		},
		{
			name: "blank line between statements preserved",
			in:   "select 1;\n\nselect 2;",
			want: "" +
				"SELECT\n" +
				"  1;\n" +
				"\n" +
				"SELECT\n" +
				"  2;\n",
		},
		{
			name: "transaction wrapper omits extra interior blank lines",
			in: "BEGIN;\n\n" +
				"update jobs set queue = 'email_v2' where queue = 'email';\n\n" +
				"COMMIT;",
			want: "" +
				"BEGIN;\n" +
				"UPDATE\n" +
				"  jobs\n" +
				"SET\n" +
				"  queue = 'email_v2'\n" +
				"WHERE\n" +
				"  queue = 'email';\n" +
				"COMMIT;\n",
		},
		{
			name: "transaction wrapper with multiple statements omits interior blanks",
			in: "BEGIN;\n\n" +
				"update jobs set queue = 'email_v2' where queue = 'email';\n\n" +
				"delete from jobs where queue = 'email';\n\n" +
				"COMMIT;",
			want: "" +
				"BEGIN;\n" +
				"UPDATE\n" +
				"  jobs\n" +
				"SET\n" +
				"  queue = 'email_v2'\n" +
				"WHERE\n" +
				"  queue = 'email';\n" +
				"DELETE FROM\n" +
				"  jobs\n" +
				"WHERE\n" +
				"  queue = 'email';\n" +
				"COMMIT;\n",
		},
		{
			name: "rollback wrapper omits interior blank lines",
			in: "BEGIN;\n\n" +
				"update jobs set queue = 'email_v2' where queue = 'email';\n\n" +
				"ROLLBACK;",
			want: "" +
				"BEGIN;\n" +
				"UPDATE\n" +
				"  jobs\n" +
				"SET\n" +
				"  queue = 'email_v2'\n" +
				"WHERE\n" +
				"  queue = 'email';\n" +
				"ROLLBACK;\n",
		},
		{
			name: "IS NOT DISTINCT FROM keeps FROM in the predicate",
			in:   "select id from drafts where user_id = $1 and item_id is not distinct from $2 and person_id is not distinct from $3;",
			want: "" +
				"SELECT\n" +
				"  id\n" +
				"FROM\n" +
				"  drafts\n" +
				"WHERE\n" +
				"  user_id = $1\n" +
				"  AND item_id IS NOT DISTINCT FROM $2\n" +
				"  AND person_id IS NOT DISTINCT FROM $3;\n",
		},
		{
			name: "jsonb existence operator is recognized",
			in:   "select args from jobs where args ? 'owner_id';",
			want: "" +
				"SELECT\n" +
				"  args\n" +
				"FROM\n" +
				"  jobs\n" +
				"WHERE\n" +
				"  args ? 'owner_id';\n",
		},
		{
			name: "window and array keywords are uppercased",
			in: "select row_number() over(partition by jobs.args ->> 'owner_id' order by jobs.created_at asc, jobs.id asc) as row_num " +
				"from jobs where jobs.status = any (array['pending', 'started']);",
			want: "" +
				"SELECT\n" +
				"  row_number() OVER (\n" +
				"    PARTITION BY jobs.args ->> 'owner_id'\n" +
				"    ORDER BY\n" +
				"      jobs.created_at ASC,\n" +
				"      jobs.id ASC\n" +
				"  ) AS row_num\n" +
				"FROM\n" +
				"  jobs\n" +
				"WHERE\n" +
				"  jobs.status = ANY (ARRAY['pending', 'started']);\n",
		},
		{
			name: "escape string literal keeps E prefix attached",
			in:   "select concat_ws(e'\\n', a, b) from t;",
			want: "" +
				"SELECT\n" +
				"  concat_ws(E'\\n', a, b)\n" +
				"FROM\n" +
				"  t;\n",
		},
		{
			name: "delete using clause is vertical",
			in:   "delete from jobs using ranked where jobs.id = ranked.id;",
			want: "" +
				"DELETE FROM\n" +
				"  jobs\n" +
				"USING\n" +
				"  ranked\n" +
				"WHERE\n" +
				"  jobs.id = ranked.id;\n",
		},
		{
			name: "truncate statement uses uppercase keyword and no leading whitespace",
			in:   "truncate t;\n\ninsert into t (a) values ($1);",
			want: "" +
				"TRUNCATE t;\n" +
				"\n" +
				"INSERT INTO t (a)\n" +
				"  VALUES ($1);\n",
		},
		{
			name: "DROP/CREATE/ALTER TABLE DDL keywords are uppercased",
			in: "drop table if exists doc_search_temp;\n" +
				"create table doc_search_temp(like doc_search including all);\n" +
				"alter table doc_search rename to doc_search_old;",
			want: "" +
				"DROP TABLE IF EXISTS doc_search_temp;\n" +
				"CREATE TABLE doc_search_temp(LIKE doc_search INCLUDING ALL);\n" +
				"ALTER TABLE doc_search RENAME TO doc_search_old;\n",
		},
		{
			name: "CREATE SEQUENCE uppercases the SEQUENCE keyword",
			in:   "create sequence if not exists test_fixture_seq;",
			want: "CREATE SEQUENCE IF NOT EXISTS test_fixture_seq;\n",
		},
		{
			name: "search-table swap DDL: EXCLUDING INDEXES, post-load index build, renames",
			in: "create table tag_search_temp(like tag_search including all excluding indexes);\n" +
				"alter table tag_search_temp add constraint bld_tag_search_pkey primary key (id);\n" +
				"create index bld_tag_search_search_vector on tag_search_temp using gin(search_vector) with (fastupdate = off);\n" +
				"create index bld_note_search_featured on note_search_temp(featured) where (featured = true);\n" +
				"analyze tag_search_temp;\n" +
				"alter table tag_search rename constraint bld_tag_search_pkey to tag_search_pkey;\n" +
				"alter index bld_tag_search_search_vector rename to index_tag_search_on_search_vector;",
			want: "" +
				"CREATE TABLE tag_search_temp(LIKE tag_search INCLUDING ALL EXCLUDING INDEXES);\n" +
				"ALTER TABLE tag_search_temp ADD CONSTRAINT bld_tag_search_pkey PRIMARY KEY (id);\n" +
				"CREATE INDEX bld_tag_search_search_vector ON tag_search_temp USING gin(search_vector) WITH (fastupdate = off);\n" +
				"CREATE INDEX bld_note_search_featured ON note_search_temp(featured)\n" +
				"WHERE\n" +
				"  (featured = TRUE);\n" +
				"ANALYZE tag_search_temp;\n" +
				"ALTER TABLE tag_search RENAME CONSTRAINT bld_tag_search_pkey TO tag_search_pkey;\n" +
				"ALTER INDEX bld_tag_search_search_vector RENAME TO index_tag_search_on_search_vector;\n",
		},
		{
			// The body is another language, so it survives byte for byte.
			name: "dollar-quoted function body passes through",
			in: "" +
				"create function f() returns trigger as $$\n" +
				"begin\n" +
				"  return new;\n" +
				"end;\n" +
				"$$ language plpgsql;",
			want: "" +
				"CREATE function f() returns trigger AS $$\n" +
				"begin\n" +
				"  return new;\n" +
				"end;\n" +
				"$$ language plpgsql;\n",
		},
		{
			name: "tagged dollar quote holds a nested $$",
			in:   "select $body$ a $$ b $body$;",
			want: "SELECT\n  $body$ a $$ b $body$;\n",
		},
		{
			name: "unterminated dollar quote is an error",
			in:   "select $$ oops;",
			want: "",
		},
		{
			name: "unterminated string is an error",
			in:   "select 'oops from t;",
			want: "",
		},
		{
			name: "block comment preserved at top level",
			in:   "/* hi */\nselect 1;",
			want: "/* hi */\nSELECT\n  1;\n",
		},
		{
			name: "!~* and !~ are recognized operators",
			in:   "select x from t where t.a !~* 'foo' and t.b !~ 'bar';",
			want: "" +
				"SELECT\n" +
				"  x\n" +
				"FROM\n" +
				"  t\n" +
				"WHERE\n" +
				"  t.a !~* 'foo'\n" +
				"  AND t.b !~ 'bar';\n",
		},
		{
			name: "unterminated block comment is an error",
			in:   "/* oops\nselect 1;",
			want: "",
		},
		{
			name: "long + chain with trailing CASE wraps one operand per line",
			in: "" +
				"select (coalesce(a.s, 0) + coalesce(b.s, 0) + coalesce(c.s, 0) " +
				"+ coalesce(d.s, 0) + case when x.id is null then 0 else 0 end) " +
				"as total from t;",
			want: "" +
				"SELECT\n" +
				"  (\n" +
				"    coalesce(a.s, 0)\n" +
				"    + coalesce(b.s, 0)\n" +
				"    + coalesce(c.s, 0)\n" +
				"    + coalesce(d.s, 0)\n" +
				"    + CASE\n" +
				"      WHEN x.id IS NULL THEN\n" +
				"        0\n" +
				"      ELSE\n" +
				"        0\n" +
				"      END\n" +
				"  ) AS total\n" +
				"FROM\n" +
				"  t;\n",
		},
		{
			// Trailing line comments on the same line as a token are
			// dropped because the printer joins items with spaces and a
			// `--` would swallow whatever followed on the same rendered
			// line. Comments inside parenthesized groups are dropped for
			// the same reason. Header comments above the statement
			// (`-- name:`, `-- ownership:`) pass through unchanged.
			name: "trailing and intra-paren line comments are dropped",
			in: "" +
				"-- name: FetchListNameForItem :one\n" +
				"-- ownership: system\n" +
				"SELECT lists.name FROM items\n" +
				"  JOIN lists\n" +
				"    ON lists.id = items.list_id\n" +
				"WHERE lists.id IN (\n" +
				"    1, -- a\n" +
				"    2, -- b\n" +
				"    3 -- c\n" +
				"  )\n" +
				"  AND items.item_id = $1;",
			want: "" +
				"-- name: FetchListNameForItem :one\n" +
				"-- ownership: system\n" +
				"SELECT\n" +
				"  lists.name\n" +
				"FROM\n" +
				"  items\n" +
				"  JOIN lists\n" +
				"    ON lists.id = items.list_id\n" +
				"WHERE\n" +
				"  lists.id IN (1, 2, 3)\n" +
				"  AND items.item_id = $1;\n",
		},
		{
			name: "parenthesized set-operation operands",
			in:   "(select 1) union all (select 2);",
			want: "" +
				"(\n" +
				"  SELECT\n" +
				"    1\n" +
				")\n" +
				"UNION ALL\n" +
				"(\n" +
				"  SELECT\n" +
				"    2\n" +
				");\n",
		},
		{
			name: "three-way union with parenthesized operands",
			in:   "(select 1) union all (select 2) union all (select 3);",
			want: "" +
				"(\n" +
				"  SELECT\n" +
				"    1\n" +
				")\n" +
				"UNION ALL\n" +
				"(\n" +
				"  SELECT\n" +
				"    2\n" +
				")\n" +
				"UNION ALL\n" +
				"(\n" +
				"  SELECT\n" +
				"    3\n" +
				");\n",
		},
		{
			name: "nested union inside FROM subquery",
			in:   "select * from ((select 1) union all (select 2)) s;",
			want: "" +
				"SELECT\n" +
				"  *\n" +
				"FROM (\n" +
				"  (\n" +
				"    SELECT\n" +
				"      1\n" +
				"  )\n" +
				"  UNION ALL\n" +
				"  (\n" +
				"    SELECT\n" +
				"      2\n" +
				"  )\n" +
				") s;\n",
		},
		{
			// A leading parenthesized subquery argument must stay a
			// function call, not be misread as a set-operation operand.
			name: "function call with leading subquery arg",
			in:   "select coalesce((select max(id) from u), 0) as m from t;",
			want: "" +
				"SELECT\n" +
				"  coalesce((SELECT max(id) FROM u), 0) AS m\n" +
				"FROM\n" +
				"  t;\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newAssert(t)
			got, err := Format(tt.in)
			if tt.want == "" {
				a.OK(err != nil)
				return
			}
			a.OK(err == nil)
			a.OK(got == tt.want)
		})
	}
}
