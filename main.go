package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"log"
	"net/http"
	"os"

	"forestburger/auth"
	sqlc "forestburger/db/sqlc"
	handlers "forestburger/handlers"
	"forestburger/seed"
	"forestburger/views"

	_ "github.com/lib/pq"
)

// schema es db/schema.sql, metido en el binario: se corre en cada arranque (es idempotente), así
// una base nueva (la de Railway) queda lista sin herramientas de migración.
//
//go:embed db/schema.sql
var schema string

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

	// El schema va antes que todo lo que usa las tablas (el admin de desarrollo, el seed).
	if _, err := conn.Exec(schema); err != nil {
		log.Fatalf("No se pudo aplicar db/schema.sql: %v", err)
	}
	log.Println("Schema al día")

	// Instancia del repositorio generado por sqlc.
	queries := sqlc.New(conn)
	// Instancio api para llamar los metodos de endpoints
	api := handlers.NewAPI(conn)
	// Las fotos: al disco (uploads/) o a R2 si están sus variables. A medias, no arranca.
	if err := api.ConfigurarFotos(); err != nil {
		log.Fatal(err)
	}
	// AMBIENTE=staging: franja "ambiente de prueba" y que los buscadores no lo indexen.
	staging := os.Getenv("AMBIENTE") == "staging"
	views.Staging = staging
	if staging {
		log.Println("Ambiente de prueba (AMBIENTE=staging)")
	}

	// Solo en desarrollo (DEV_ADMIN_EMAIL y DEV_ADMIN_PASSWORD en el .env): deja un admin listo para
	// entrar (make run-app arranca con la base vacía). Si falla, se avisa pero la app arranca igual.
	if email, err := auth.AdminDesarrollo(context.Background(), queries); err != nil {
		log.Printf("Admin de desarrollo: %v", err)
	} else if email != "" {
		log.Printf("Admin de desarrollo listo: %s", email)
	}

	// Solo en desarrollo (DEV_DATOS_PRUEBA=1) y con la base sin productos: carga productos, extras,
	// ajustes y pedidos de los últimos días para poder ver el panel. Una sola vez.
	if res, err := seed.DatosPrueba(context.Background(), conn); err != nil {
		log.Printf("Datos de prueba: %v", err)
	} else if res.Productos > 0 {
		log.Printf("Datos de prueba cargados: %d productos, %d extras, %d pedidos", res.Productos, res.Extras, res.Pedidos)
	}

	views.VersionEstilos = versionDe("static/styles.css")
	views.VersionPanelJS = versionDe("static/panel.js")
	views.VersionCliente = versionDe("static/cliente.css")

	// Router: acá se registran los handlers de la API, que reciben queries.
	mux := http.NewServeMux()
	mux.Handle("GET /static/", sinCache(http.StripPrefix("/static/", http.FileServer(http.Dir("static")))))

	// /login y /logout son públicos; todo /admin pasa por el middleware de sesión.
	api.Rutas(mux)

	var handler http.Handler = mux
	if staging {
		handler = noIndexar(mux)
	}

	// Railway (y otros) dicen en qué puerto escuchar con PORT; en local, 8080 como siempre.
	puerto := os.Getenv("PORT")
	if puerto == "" {
		puerto = "8080"
	}
	log.Printf("Servidor escuchando en %s", urlPublica(puerto))
	log.Fatal(http.ListenAndServe(":"+puerto, handler))
}

// urlPublica es la dirección por la que se entra: la que da Railway si está, o localhost.
func urlPublica(puerto string) string {
	if dominio := os.Getenv("RAILWAY_PUBLIC_DOMAIN"); dominio != "" {
		return "https://" + dominio
	}
	return "http://localhost:" + puerto
}

// noIndexar le dice a los buscadores que no indexen nada (el ambiente de prueba): también las
// fotos y los archivos de static/, que no llevan el <meta>.
func noIndexar(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Robots-Tag", "noindex")
		next.ServeHTTP(w, r)
	})
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
