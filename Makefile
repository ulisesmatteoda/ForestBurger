APP_NAME := forestburger
DB_URL   := postgres://usuario:password@localhost:5432/mydb?sslmode=disable

.PHONY: all generate build up down wait-db test clean

all: test

# Tareas previas -------------------------------------------------

generate:
	@sqlc generate

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
	@until docker compose exec -T database sh -c 'pg_isready -U "$$POSTGRES_USER"'; do sleep 1; done

# Ejecución de tests -----------------------------------------------

test: build wait-db
	@go test ./... ; \
	status=$$? ; \
	$(MAKE) clean ; \
	exit $$status

# Tareas posteriores -------------------------------------------------

clean:
	@docker compose down -v

