package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"log"
	"net/http"
	"os"

	"forestburger/auth"
	sqlc "forestburger/db/sqlc"
	handlers "forestburger/handlers"
	"forestburger/views"

	_ "github.com/lib/pq"
)

func main() {
	// Sin SESSION_SECRET no se pueden firmar las sesiones: mejor no arrancar que arrancar inseguro.
	if err := auth.CargarConfig(); err != nil {
		log.Fatal(err)
	}

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
	api := handlers.NewAPI(conn)

	// Solo en desarrollo (DEV_ADMIN_EMAIL y DEV_ADMIN_PASSWORD en el .env): deja un admin listo para
	// entrar, porque make run arranca con la base vacía. Si falla, se avisa pero la app arranca igual.
	if email, err := auth.AdminDesarrollo(context.Background(), queries); err != nil {
		log.Printf("Admin de desarrollo: %v", err)
	} else if email != "" {
		log.Printf("Admin de desarrollo listo: %s", email)
	}

	views.VersionEstilos = versionDe("static/styles.css")

	// Router: acá se registran los handlers de la API, que reciben queries.
	mux := http.NewServeMux()
	mux.Handle("GET /static/", sinCache(http.StripPrefix("/static/", http.FileServer(http.Dir("static")))))

	// Por ahora la raíz manda al panel. Más adelante acá va a ir el menú para los clientes.
	mux.Handle("GET /{$}", http.RedirectHandler("/admin", http.StatusSeeOther))

	// /login y /logout son públicos; todo /admin pasa por el middleware de sesión.
	api.Rutas(mux)

	log.Println("Servidor escuchando en http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

// sinCache hace que el navegador pregunte cada vez si un archivo de static/ cambió (si no cambió
// recibe un 304 sin descargarlo). Sin esto puede seguir usando un styles.css viejo después de
// editarlo, y parece que el cambio no anduvo.
func sinCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		next.ServeHTTP(w, r)
	})
}

// versionDe devuelve un pedazo del hash del archivo: cambia solo cuando cambia su contenido. Si no
// lo puede leer devuelve "", y la página funciona igual (sin versión en la URL).
func versionDe(ruta string) string {
	contenido, err := os.ReadFile(ruta)
	if err != nil {
		return ""
	}
	suma := sha256.Sum256(contenido)
	return hex.EncodeToString(suma[:])[:10]
}
