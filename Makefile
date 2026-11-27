APP_NAME    := forestburger
DB_URL      := postgres://usuario:password@localhost:5432/forestburger?sslmode=disable
# Los tests usan su propia base (en el mismo contenedor), así nunca tocan los datos de desarrollo.
DB_TEST     := forestburger_test
DB_URL_TEST := postgres://usuario:password@localhost:5432/$(DB_TEST)?sslmode=disable

# Estas variables salen del .env y las leen crear-admin y la app (go no lee el .env solo).
-include .env
export ADMIN_EMAILS SESSION_SECRET COOKIE_SECURE DEV_ADMIN_EMAIL DEV_ADMIN_PASSWORD DEV_DATOS_PRUEBA

ESPERAR_DB = until docker compose exec -T database sh -c 'pg_isready -h 127.0.0.1 -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"'; do sleep 1; done
# psql dentro del contenedor, sin los avisos de "ya existe" del schema (todo es IF NOT EXISTS).
# $(call PSQL) usa la base de desarrollo; $(call PSQL,otra) usa esa otra.
PSQL = docker compose exec -T -e BASE=$(1) database sh -c 'PGOPTIONS="-c client_min_messages=warning" psql -q -v ON_ERROR_STOP=1 -U "$$POSTGRES_USER" -d "$${BASE:-$$POSTGRES_DB}"'

.PHONY: all generate build up up-limpio down wait-db wait-db-limpio schema test test-api run run-app crear-admin vapid clean

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

# Levanta la base sin borrarla: los datos quedan entre corridas.
up: .env
	@docker compose up -d

# Levanta la base desde cero (lo que pide el TP para run-app y test-api).
up-limpio: down up

wait-db: up
	@echo "Esperando a que la base esté lista..."
	@$(ESPERAR_DB)

wait-db-limpio: up-limpio
	@echo "Esperando a que la base esté lista..."
	@$(ESPERAR_DB)

# Aplica db/schema.sql a la base de desarrollo. Es seguro correrlo siempre (todo es IF NOT
# EXISTS): así una base que ya tiene datos recibe las tablas nuevas sin borrar nada.
schema: wait-db
	@$(call PSQL) < db/schema.sql

# Ejecución de tests -----------------------------------------------

# Recrea la base de test desde cero, le aplica el schema y corre los tests contra ella. La base de
# desarrollo (y sus datos) no se toca.
test: build wait-db
	@docker compose exec -T database sh -c 'dropdb --if-exists --force -U "$$POSTGRES_USER" $(DB_TEST) && createdb -U "$$POSTGRES_USER" $(DB_TEST)'
	@$(call PSQL,$(DB_TEST)) < db/schema.sql
	@DATABASE_URL="$(DB_URL_TEST)" go test -p 1 -count=1 -v ./...

test-api: build wait-db-limpio
	@./test-api.sh ; \
	status=$$? ; \
	$(MAKE) clean ; \
	exit $$status

# Previas + ejecución + posteriores (el flujo del TP: arranca y termina con la base vacía)
run-app: build wait-db-limpio
	@trap '$(MAKE) --no-print-directory clean' EXIT; \
	go run .

# Ejecución del servidor -------------------------------------------

# Arranca con la base que haya (la levanta si hace falta) y le aplica el schema.
run: build schema
	@DATABASE_URL="$(DB_URL)" go run .

# Admins -------------------------------------------------------------

# Crea un admin o le cambia la contraseña, en la base de desarrollo (la que usa make run).
crear-admin: generate schema
	@DATABASE_URL="$(DB_URL)" go run ./cmd/crear-admin

# Genera las claves VAPID de las notificaciones push (una vez por ambiente: van al .env o a Railway).
vapid:
	@go run ./cmd/vapid

# Tareas posteriores -------------------------------------------------

# Borra todo (la base y las fotos): la forma de empezar de cero.
clean: down
