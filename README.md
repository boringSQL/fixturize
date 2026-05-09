# fixturize

Subset a PostgreSQL database and anonymize sensitive columns in one step. Extract referentially-intact slices of production data, mask PII, and seed dev, test, or staging environments.

## What it does

1. **`extract`** - Pick a root table and optional filtering clauses. Fixturize follows foreign keys in both directions (parents and children) to collect a referentially-intact subset of the data. Supports composite FKs, self-referencing tables, and circular dependencies.
2. **`apply`** - Load the snapshot into another database. Tables are inserted in FK-dependency order, constraints are deferred.
3. **`inspect`** - Display schema structure with FK relationships without extracting any data.
4. **`analyze`** - Discover PII columns automatically by scanning column names and types. Covers emails, names, phones, addresses, financial data, API keys, and more. Outputs ready-to-use `--mask` expressions.

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

# only completed orders for each org
fixturize extract --connection "$DB" \
  --root "organizations WHERE id = 42" \
  --filter "orders=status='completed'"
```

The `--root` flag accepts any valid SQL fragment after the table name: `WHERE`, `ORDER BY`, `LIMIT`.

`--filter` adds a WHERE condition to a specific child table during traversal (repeatable). Format: `table=condition`.

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

## Data Masking

Replace PII and sensitive columns with SQL expressions during extraction (static data masking — the fixture never contains real values). Expressions run in the SELECT and can reference the same row:

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

Masks are recorded in the fixture metadata (columns masked, expressions used, extraction timestamp) so you have an audit trail of what was anonymized.

Use `fixturize analyze` to auto-detect which columns need masking and get suggested expressions.

## Profiles

Store default flags in a YAML file instead of typing them every time. Fixturize auto-loads `.fixturize.yaml` from the current directory, or you can point to a specific file with `--profile`:

```bash
fixturize extract --profile staging.yaml --root "users LIMIT 5"
```

CLI flags always take precedence over profile values.

### Example profile

```yaml
connection: "postgresql://${DB_USER}:${DB_PASS}@localhost/mydb"
schema: public

masks:
  pii:
    - "users.email='user_' || id || '@test.com'"
    - "users.first_name='First' || id"
    - "users.last_name='Last' || id"
  billing:
    - "billing.cards.number='4111111111111111'"

extract:
  root: "organizations WHERE id = 42"
  output: fixtures/org-42.json
  format: json
  limit: 500
  depth: 0
  statement_timeout: 60
  transaction: false
  on_conflict_do_nothing: false
  include:
    - roles
    - permissions
  exclude:
    - audit_log
    - event_log
  mask_policies:
    - pii
    - billing
  masks:
    - "contacts.phone='+1555' || LPAD((id % 10000000)::text, 7, '0')"

apply:
  force: false
  disable_triggers: false
  sync_sequences: true
```

### How it works

- `.fixturize.yaml` in the working directory is loaded automatically. Pass `--profile path/to/file.yaml` to use a different file.
- Environment variables in the `connection` field are expanded (`$VAR` and `${VAR}` syntax). Unresolved variables cause an error so you don't accidentally connect with a blank password.
- `masks` at the top level defines reusable **mask policies** - named groups of mask expressions. Reference them from `extract.mask_policies` to compose sets of masks without repeating yourself. Inline `extract.masks` are appended after policy masks.
- All fields are optional. Only set what you need - everything else falls back to CLI defaults.

### Shared masking config

Mask expressions live in separate file `data-masking-policy.yml`.

The file is keyed by `database_id` and stores one canonical expression per column, with free-form tags. Policies select columns by tag.

```yaml
# data-masking-policy.yml
version: 1

databases:
  myapp:
    columns:
      users.email:        { expr: "'user_' || id || '@test.com'", tags: [pii] }
      users.first_name:   { expr: "'First' || id",                tags: [pii] }
      billing.cards.number: { expr: "'4111111111111111'",         tags: [pii, financial] }
    policies:
      pii:       { include_tags: [pii] }
      financial: { include_tags: [financial] }
```

Profile fields:

```yaml
database_id: myapp                       # selects the namespace in the shared file
masks_file: ./data-masking-policy.yml    # optional explicit path
extract:
  mask_policies: [pii]                   # same as before
```

Both fields are optional. If `masks_file` is omitted, fixturize walks up from the profile's directory looking for `data-masking-policy.yml`, stopping at the `.git` boundary. If `database_id` is set but missing from the file, the loader errors and lists the available IDs.

Inline `masks:` (from previous versions) can be used, but are now effectively deprecated.

## Precautions

- extraction runs under **REPEATABLE READ** isolation - consistent snapshot, but holds a transaction open. Use `--statement-timeout` (default 30s) to bound query time.
- `--force` on apply **truncates** target tables before insert. Don't point it at production.
- Circular FK dependencies are detected and warned about. The tool handles them via deferred constraints, but review the output.
- generated/identity columns are excluded from extraction and use `OVERRIDING SYSTEM VALUE` on apply.
- Always mask PII before sharing fixtures across environments, staging databases, or CI/CD pipelines.

