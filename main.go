package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"

	sqlc "forestburger/db/sqlc"

	_ "github.com/lib/pq"
)

func main() {
	// Cadena de conexión: se toma de DATABASE_URL o se usa la misma que el Makefile y los tests.
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		connStr = "postgres://usuario:password@localhost:5432/forestburger?sslmode=disable"
	}

	// sql.Open no conecta realmente, por eso se verifica con Ping.
	conn, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("Error abriendo conexión: %v", err)
	}
	defer conn.Close()

	if err := conn.Ping(); err != nil {
		log.Fatalf("No se pudo conectar a la base de datos: %v", err)
	}
	log.Println("Conectado a la base de datos")

	// Instancia del repositorio generado por sqlc.
	queries := sqlc.New(conn)

	// Router: acá se registran los handlers de la API, que reciben queries.
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServer(http.Dir("static")))
	_ = queries // se usa en los handlers del paso 2

	log.Println("Servidor escuchando en http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
