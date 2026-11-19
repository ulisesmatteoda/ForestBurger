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
	mux.HandleFunc("GET /{$}", api.Index)
	mux.HandleFunc("POST /productos", api.CrearProducto)
	mux.HandleFunc("GET /productos/{id}", api.FilaProducto)
	mux.HandleFunc("GET /productos/{id}/editar", api.FormEditarProducto)
	mux.HandleFunc("PUT /productos/{id}", api.ActualizarProducto)
	mux.HandleFunc("DELETE /productos/{id}", api.BorrarProducto)

	// Probamos los endpoints
	// VER PARA HACER EN UN METODO APARTE PARA ASI TENER MAS LIMPIA ESTA PARTE DE CODIGO
	/*
		mux.HandleFunc("POST /api/productos", api.createProducto)
		mux.HandleFunc("GET /api/productos", api.listProductos)
		mux.HandleFunc("GET /api/productos/{id}", api.getProducto)
		mux.HandleFunc("PUT /api/productos/{id}", api.updateProducto)
		mux.HandleFunc("DELETE /api/productos/{id}", api.deleteProducto)
	*/
	log.Println("Servidor escuchando en http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
