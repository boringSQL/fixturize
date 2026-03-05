# Try fixturize

Try fixturize against a real PostgreSQL database without installing Go or setting up a local Postgres. Uses plain `docker run` (no docker-compose needed).

## Quick start

```bash
git clone https://github.com/boringsql/fixturize && cd fixturize

make try              # ~130K rows, data baked into image (instant start)
make try-shell        # interactive container with fixturize + psql
```

## Commands

| Command | What it does |
|---------|-------------|
| `make try` | Build images, start postgres with ~130K rows (instant) |
| `make try-large` | Start with full 59M rows (~10-15 min seed) |
| `make try-status` | Show row counts / seeding progress |
| `make try-shell` | Interactive container with fixturize on same network |
| `make try-down` | Remove container and network |

## Host access

Connect from your host machine:

```bash
psql postgres://demo:demo@localhost:15432/fixturize_demo
```

## What's inside

20-table e-commerce marketplace schema with:
- Self-referencing categories, circular FK between departments/employees
- Partitioned tables (payments, audit_logs) with range and hash partitioning
- UUID primary keys, JSONB columns, generated columns, ENUM types
- PII-heavy fields (addresses, bank details, credit cards)

### Medium seed (~130K rows)

| Table | Rows |
|-------|------|
| organizations | 10 |
| org_settings | 10 |
| categories | 50 |
| users | 5,000 |
| user_addresses | 10,000 |
| seller_profiles | 250 |
| departments | 100 |
| employees | 500 |
| warehouses | 50 |
| products | 2,000 |
| product_variants | 5,000 |
| warehouse_inventory | 5,000 |
| inventory_movements | 10,000 |
| orders | 15,000 |
| order_items | 30,000 |
| payments | 20,000 |
| shipments | 5,000 |
| reviews | 10,000 |
| support_tickets | 2,000 |
| ticket_messages | 5,000 |
| audit_logs | 10,000 |
