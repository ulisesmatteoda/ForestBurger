APP_NAME := forestburger
DB_URL   := postgres://usuario:password@localhost:5432/forestburger?sslmode=disable

# Estas variables salen del .env y las leen crear-admin y la app (go no lee el .env solo).
-include .env
export ADMIN_EMAILS SESSION_SECRET COOKIE_SECURE DEV_ADMIN_EMAIL DEV_ADMIN_PASSWORD

ESPERAR_DB = until docker compose exec -T database sh -c 'pg_isready -h 127.0.0.1 -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"'; do sleep 1; done

.PHONY: all generate build up down wait-db test test-api run run-app crear-admin clean

all: test

# Tareas previas -------------------------------------------------

generate:
	@sqlc generate
	@templ generate

build: generate
	@go build ./...

.env:
	@cp .env.example .env

# Borra la base y, con ella, las fotos subidas: sin la base nadie las usa y quedarían huérfanas.
down:
	@docker compose down -v
	@find uploads -type f ! -name .gitkeep -delete 2>/dev/null || true

up: down .env
	@docker compose up -d

wait-db: up
	@echo "Esperando a que la base esté lista..."
	@$(ESPERAR_DB)

# Ejecución de tests -----------------------------------------------

test: build wait-db
	@DATABASE_URL="$(DB_URL)" go test -p 1 -count=1 -v ./... ; \
	status=$$? ; \
	$(MAKE) --no-print-directory clean ; \
	exit $$status

test-api: build wait-db
	@./test-api.sh ; \
	status=$$? ; \
	$(MAKE) clean ; \
	exit $$status

# Previas + ejecución + posteriores
run-app: build wait-db
	@trap '$(MAKE) --no-print-directory clean' EXIT; \
	go run .

# Ejecución del servidor -------------------------------------------

run: build wait-db
	@go run .

# Admins -------------------------------------------------------------

# Crea un admin o le cambia la contraseña. A diferencia de up, NO borra la base: si está levantada
# (por ejemplo con make run en otra terminal) usa esa. Aplica el schema antes porque es idempotente
# (CREATE TABLE IF NOT EXISTS): una base que se levantó antes de existir la tabla admin la recibe.
crear-admin: generate .env
	@docker compose up -d database
	@echo "Esperando a que la base esté lista..."
	@$(ESPERAR_DB)
	@docker compose exec -T database sh -c 'PGOPTIONS="-c client_min_messages=warning" psql -q -v ON_ERROR_STOP=1 -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"' < db/schema.sql
	@DATABASE_URL="$(DB_URL)" go run ./cmd/crear-admin

# Tareas posteriores -------------------------------------------------

clean: down
