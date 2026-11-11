APP_NAME := forestburger
DB_URL   := postgres://usuario:password@localhost:5432/forestburger?sslmode=disable

.PHONY: all generate build up down wait-db test test-api run run-app clean

all: test

# Tareas previas -------------------------------------------------

generate:
	@sqlc generate
	@templ generate

build: generate
	@go build ./...

.env:
	@cp .env.example .env

down:
	@docker compose down -v

up: down .env
	@docker compose up -d

wait-db: up
	@echo "Esperando a que la base esté lista..."
	@until docker compose exec -T database sh -c 'pg_isready -h 127.0.0.1 -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"'; do sleep 1; done

# Ejecución de tests -----------------------------------------------

test: build wait-db
	@DATABASE_URL="$(DB_URL)" go test -count=1 -v ./... ; \
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

# Tareas posteriores -------------------------------------------------

clean:
	@docker compose down -v
