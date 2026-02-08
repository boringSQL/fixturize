# fixturize

Extract consistent data sub-graphs from a PostgreSQL database and apply them elsewhere. Built for seeding test databases with real-world data while keeping referential integrity intact.

## What it does

1. **`extract`** - Pick a root table and (optional) filtering clauses. Fixturize will follow foreign keys in both directions (parents and children) to collect a self-contained snapshot of the data.
2. **`apply`** - Load the snapshot into another database. Tables are inserted in FK-dependency order, constraints are deferred.
3. **`inspect`** - Display schema structure with FK relationships without extracting any data.
4. **`analyze`** - Auto-detect PII columns by scanning column names and types, and suggest mask expressions.

## Install

```bash
go build -o ~/bin/fixturize ./cmd/fixturize
```

## Usage

### Extract

```bash
# one org and everything it touches
fixturize extract --connection "$DB" \
  --root "organizations WHERE id = 42"

# 3 random orgs, cap child tables at 500 rows
fixturize extract --connection "$DB" \
  --root "organizations ORDER BY random() LIMIT 3" \
  --limit 500

# pull in lookup tables that have no FK path, skip audit noise
fixturize extract --connection "$DB" \
  --root "organizations WHERE id = 42" \
  --include "roles,permissions" \
  --exclude "audit_log,event_log"

# preview without writing a file
fixturize extract --connection "$DB" \
  --root "users LIMIT 5" --dry-run
```

The `--root` flag accepts any valid SQL fragment after the table name: `WHERE`, `ORDER BY`, `LIMIT`.

`--include` pulls entire tables (all rows) - useful for enums and lookups that aren't FK-linked.

`--exclude` skips tables completely. You'll get a warning if an excluded table is a FK parent (dangling references).

### Apply

```bash
# load fixture into test DB
fixturize apply --connection "postgresql://..." fixtures/org-42.json

# Wipe target tables first
fixturize apply --connection "postgresql://..." --force fixtures/org-42.json
````

### Inspect

Preview the table structure before extraction.

```bash
# show all tables with columns, PKs, FKs, unique constraints
fixturize inspect --connection "$DB"

# only tables reachable from users (2 FK hops)
fixturize inspect --connection "$DB" --root users --depth 2
```

### Analyze

Scan schema for PII columns and get ready-to-use `--mask` expressions:

```bash
fixturize analyze --connection "$DB"
```

```
public.users
  email       character varying(255)  Email       MED   'user_' || "id" || '@test.com'
  first_name  character varying(100)  First Name  MED   'First' || "id"
  last_name   character varying(100)  Last Name   MED   'Last' || "id"

public.contacts
  phone       character varying(20)   Phone       MED   '+1555' || LPAD(("id" % 10000000)::text, 7, '0')

4 PII column(s) in 2 table(s)
```

Filter by confidence level or scope to a subgraph:

```bash
# only medium+ confidence (skip noisy matches)
fixturize analyze --connection "$DB" --min-confidence medium

# scope to tables reachable from users
fixturize analyze --connection "$DB" --root users --depth 2
```

Detection uses column name patterns (word-level matching, e.g. `user_email` matches but `emailed_at` does not) combined with PostgreSQL type checking. Columns that are boolean, timestamp, integer, PK, or FK are automatically excluded to reduce false positives.

### Connection

Pass `--connection` or set `DATABASE_URL` env variable:

```bash
export DATABASE_URL="postgresql://user:pass@localhost/mydb"

fixturize extract --root "users LIMIT 10"
```

## Masking

Replace sensitive columns with SQL expressions during extraction. The expression runs in the SELECT and can reference the same row:

```bash
fixturize extract --connection "$DB" \
  --root "organizations WHERE id = 42" \
  --mask "auth.users.email='user_' || id || '@test.com'" \
  --mask "auth.users.name='User ' || id" \
  --mask "billing.cards.number='4111111111111111'"
```

Format: `schema.table.column=sql_expression` (or `table.column=expr` for public schema).

Since the expression runs as raw SQL in the SELECT, you can use `CASE` to preserve NULLs:

```bash
--mask "users.email=CASE WHEN email IS NOT NULL THEN 'user_' || id || '@test.com' END"
```

Masks are recorded in the fixture metadata so you know what was scrubbed.

Use `fixturize analyze` to auto-detect which columns need masking and get suggested expressions.

## Precautions

- extraction runs under **REPEATABLE READ** isolation - consistent snapshot, but holds a transaction open. Use `--statement-timeout` (default 30s) to bound query time.
- `--force` on apply **truncates** target tables before insert. Don't point it at production.
- Circular FK dependencies are detected and warned about. The tool handles them via deferred constraints, but review the output.
- generated/identity columns are excluded from extraction and use `OVERRIDING SYSTEM VALUE` on apply.
- Always mask PII before sharing fixtures across environments.

