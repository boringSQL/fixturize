ROOT        := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))..
NETWORK     := fixturize-net
PG_NAME     := fixturize-pg
PG_IMAGE    := fixturize-demo-db
FIX_IMAGE   := fixturize:local
DATABASE_URL := postgres://demo:demo@$(PG_NAME):5432/fixturize_demo?sslmode=disable

.PHONY: all large status shell down build build-db build-fix

all: build-db network pg
	@echo ""
	@echo "  Postgres is ready:  psql postgres://demo:demo@localhost:15432/fixturize_demo"
	@echo "  Run 'make try-shell' to get an interactive fixturize container."
	@echo "  Run 'make try-down' to clean up."
	@echo ""

large: build-fix network pg-large
	@echo ""
	@echo "  Postgres is seeding ~59M rows (10-15 min)."
	@echo "  Run 'make try-status' to check progress."
	@echo "  Run 'make try-shell' once seeding is done."
	@echo "  Run 'make try-down' to clean up."
	@echo ""

network:
	@docker network inspect $(NETWORK) >/dev/null 2>&1 || docker network create $(NETWORK)

build: build-db build-fix

build-db:
	@if docker image inspect $(PG_IMAGE) >/dev/null 2>&1; then \
		echo "DB image $(PG_IMAGE) already built."; \
	else \
		echo "Building baked DB image (first time takes ~2 min)..."; \
		docker build -t $(PG_IMAGE) -f $(ROOT)/try/Dockerfile.db $(ROOT); \
	fi

build-fix:
	@if docker image inspect $(FIX_IMAGE) >/dev/null 2>&1; then \
		echo "Fixturize image $(FIX_IMAGE) already built."; \
	else \
		echo "Building fixturize image (first time takes ~3 min)..."; \
		docker build -t $(FIX_IMAGE) -f $(ROOT)/demo/Dockerfile $(ROOT); \
	fi

pg:
	@if docker ps -q -f name=^$(PG_NAME)$$ | grep -q .; then \
		echo "$(PG_NAME) is already running."; \
	else \
		docker rm -f $(PG_NAME) 2>/dev/null || true; \
		docker run -d \
			--name $(PG_NAME) \
			--network $(NETWORK) \
			-p 15432:5432 \
			$(PG_IMAGE); \
		echo "Started $(PG_NAME) (baked DB, instant)."; \
	fi

pg-large:
	@if docker ps -q -f name=^$(PG_NAME)$$ | grep -q .; then \
		echo "$(PG_NAME) is already running."; \
	else \
		docker rm -f $(PG_NAME) 2>/dev/null || true; \
		docker run -d \
			--name $(PG_NAME) \
			--network $(NETWORK) \
			-p 15432:5432 \
			-e POSTGRES_USER=demo \
			-e POSTGRES_PASSWORD=demo \
			-e POSTGRES_DB=fixturize_demo \
			-v $(ROOT)/lighting/schema.sql:/docker-entrypoint-initdb.d/01-schema.sql:ro \
			-v $(ROOT)/lighting/seed.sql:/docker-entrypoint-initdb.d/02-seed.sql:ro \
			postgres:17 \
			-c shared_buffers=512MB \
			-c work_mem=256MB \
			-c maintenance_work_mem=1GB \
			-c wal_level=minimal \
			-c max_wal_senders=0 \
			-c fsync=off \
			-c synchronous_commit=off \
			-c full_page_writes=off; \
		echo "Started $(PG_NAME) (large mode, seeding ~59M rows)."; \
	fi

status:
	@docker exec $(PG_NAME) psql -U demo -d fixturize_demo -c " \
		SELECT schemaname, relname AS table, n_live_tup AS rows \
		FROM pg_stat_user_tables \
		WHERE schemaname = 'public' \
		ORDER BY n_live_tup DESC;" 2>/dev/null || \
	(echo "--- Container logs (last 20 lines) ---" && docker logs --tail 20 $(PG_NAME))

shell: build-fix network
	@docker run --rm -it \
		--network $(NETWORK) \
		-e DATABASE_URL="$(DATABASE_URL)" \
		$(FIX_IMAGE) \
		bash -c ' \
		echo ""; \
		echo "fixturize demo - connected to fixturize_demo"; \
		echo ""; \
		echo "  fixturize inspect  --root organizations --depth 3"; \
		echo "  fixturize analyze"; \
		echo "  fixturize analyze  --yaml   # output as .fixturize.yaml config"; \
		echo "  fixturize extract  --root \"organizations WHERE id = 1\" -o fixture.json"; \
		echo ""; \
		echo "  psql \$$DATABASE_URL     # direct SQL access"; \
		echo "  cat fixture.json | head  # inspect the extracted fixture"; \
		echo ""; \
		exec bash'

down:
	@docker rm -f $(PG_NAME) 2>/dev/null || true
	@docker network rm $(NETWORK) 2>/dev/null || true
	@echo "Cleaned up."
