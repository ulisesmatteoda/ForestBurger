package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"

	sqlc "forestburger/db/sqlc"
	handlers "forestburger/handlers"

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
	// Instancio api para llamar los metodos de endpoints
	api := handlers.NewAPI(queries)

	// Router: acá se registran los handlers de la API, que reciben queries.
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Por ahora la raíz manda al panel. Más adelante acá va a ir el menú para los clientes.
	mux.Handle("GET /{$}", http.RedirectHandler("/admin", http.StatusSeeOther))

	api.RutasAdmin(mux)

	log.Println("Servidor escuchando en http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
