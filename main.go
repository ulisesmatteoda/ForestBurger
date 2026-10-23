package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"

	sqlc "forestburger/db/sqlc"
	"forestburger/handlers"

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
	mux.Handle("GET /", http.FileServer(http.Dir("static")))
	_ = queries // se usa en los handlers del paso 2

	// Probamos los endpoints
	// VER PARA HACER EN UN METODO APARTE PARA ASI TENER MAS LIMPIA ESTA PARTE DE CODIGO

	mux.HandleFunc("POST /api/productos", api.CreateProducto)
	mux.HandleFunc("GET /api/productos", api.ListProductos)
	mux.HandleFunc("GET /api/productos/{id}", api.GetProducto)
	mux.HandleFunc("PUT /api/productos/{id}", api.UpdateProducto)
	mux.HandleFunc("DELETE /api/productos/{id}", api.DeleteProducto)

	mux.HandleFunc("POST /api/extras", api.CreateExtra)
	mux.HandleFunc("GET /api/extras", api.ListExtras)
	mux.HandleFunc("GET /api/extras/{id}", api.GetExtra)
	mux.HandleFunc("PUT /api/extras/{id}", api.UpdateExtra)
	mux.HandleFunc("DELETE /api/extras/{id}", api.DeleteExtra)

	mux.HandleFunc("POST /api/pedidos", api.CreatePedido)
	mux.HandleFunc("GET /api/pedidos", api.ListPedidos)
	mux.HandleFunc("GET /api/pedidos/{id}", api.GetPedido)
	mux.HandleFunc("PUT /api/pedidos/{id}/estado", api.UpdateEstadoPedido)

	log.Println("Servidor escuchando en http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
