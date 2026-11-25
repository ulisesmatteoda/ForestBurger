# ForestBurger para Railway (staging). Dos etapas: en la primera se genera el código (sqlc, templ)
# y se compila; la final lleva solo los binarios, static/ y el schema.

# ---------- Build ----------
FROM golang:1.26 AS build

# Las mismas versiones que en local: sqlc como `sqlc version`, templ como go.mod. sqlc necesita CGO
# (su parser de Postgres), por eso esta imagen es la Debian completa, que trae gcc.
RUN go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 \
 && go install github.com/a-h/templ/cmd/templ@v0.3.1100

WORKDIR /src
# Primero las dependencias: si no cambian, Docker reusa esta capa.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN sqlc generate && templ generate
RUN CGO_ENABLED=0 go build -o /out/forestburger . \
 && CGO_ENABLED=0 go build -o /out/crear-admin ./cmd/crear-admin

# ---------- Final ----------
FROM debian:bookworm-slim
# ca-certificates: para hablar con R2 por HTTPS. La zona horaria ya viaja en el binario.
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=build /out/forestburger /out/crear-admin ./
COPY static ./static
COPY db/schema.sql ./db/schema.sql

EXPOSE 8080
CMD ["./forestburger"]
