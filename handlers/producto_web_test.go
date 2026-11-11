package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	_ "github.com/lib/pq" // cambiá por el driver que usa tu main.go

	sqlc "forestburger/db/sqlc"
)

// ---------- unitarios (no necesitan base) ----------

func TestProductoRequestValidate(t *testing.T) {
	neg := -1.0
	casos := []struct {
		nombre string
		req    productoRequest
		quiere string
	}{
		{"ok", productoRequest{Nombre: "Clásica", Categoria: "burger", Precio: 10}, ""},
		{"nombre vacío", productoRequest{Nombre: "  ", Categoria: "burger", Precio: 10}, "el nombre es obligatorio"},
		{"categoría vacía", productoRequest{Nombre: "x", Categoria: "", Precio: 10}, "la categoría es obligatoria"},
		{"precio negativo", productoRequest{Nombre: "x", Categoria: "c", Precio: -5}, "el precio no puede ser negativo"},
		{"precio doble negativo", productoRequest{Nombre: "x", Categoria: "c", Precio: 5, PrecioDoble: &neg}, "el precio doble no puede ser negativo"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := c.req.validate(); got != c.quiere {
				t.Errorf("validate() = %q, quiero %q", got, c.quiere)
			}
		})
	}
}

func TestConversionesPrecio(t *testing.T) {
	r := productoRequest{Precio: 308}
	if got := r.precioStr(); got != "308.00" {
		t.Errorf("precioStr() = %q, quiero 308.00", got)
	}
	if r.precioDobleNull().Valid {
		t.Error("precioDobleNull() debería ser NULL cuando no hay precio doble")
	}
	pd := 450.5
	r.PrecioDoble = &pd
	if got := r.precioDobleNull(); !got.Valid || got.String != "450.50" {
		t.Errorf("precioDobleNull() = %+v, quiero 450.50 válido", got)
	}
}

// ---------- integración (necesitan Postgres) ----------

// setup abre la base de TEST_DATABASE_URL, la deja vacía y arma el mux con las rutas web.
func setup(t *testing.T) (*http.ServeMux, *sqlc.Queries) {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://usuario:password@localhost:5432/forestburger?sslmode=disable"
	}
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("no se pudo abrir la base: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := conn.Ping(); err != nil {
		t.Fatalf("no se pudo conectar a la base: %v", err)
	}
	// Ajustá el nombre de la tabla si es distinto.
	if _, err := conn.Exec(`TRUNCATE producto RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("no se pudo limpiar la tabla: %v", err)
	}

	q := sqlc.New(conn)
	api := NewAPI(q)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", api.Index)
	mux.HandleFunc("POST /productos", api.CrearProducto)
	return mux, q
}

func postForm(mux http.Handler, datos url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/productos", strings.NewReader(datos.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestIndex_MuestraFormularioYProductos(t *testing.T) {
	mux, q := setup(t)
	_, err := q.CreateProducto(context.Background(), sqlc.CreateProductoParams{
		Nombre: "Forest Clásica", Categoria: "burger", Precio: "9500.00",
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quiero 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, quiero text/html", ct)
	}
	body := rec.Body.String()
	for _, esperado := range []string{"<form", `action="/productos"`, "Forest Clásica"} {
		if !strings.Contains(body, esperado) {
			t.Errorf("el HTML no contiene %q", esperado)
		}
	}
}

func TestCrearProducto_RedirigeYGuarda(t *testing.T) {
	mux, q := setup(t)

	rec := postForm(mux, url.Values{
		"nombre":       {"Doble Bacon"},
		"categoria":    {"burger"},
		"precio":       {"12000"},
		"precio_doble": {"15000"},
		"descripcion":  {"con panceta"},
	})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, quiero 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Errorf("Location = %q, quiero /", loc)
	}

	ps, err := q.ListProductos(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 {
		t.Fatalf("hay %d productos, quiero 1", len(ps))
	}
	if ps[0].Nombre != "Doble Bacon" || !ps[0].PrecioDoble.Valid {
		t.Errorf("producto guardado incorrecto: %+v", ps[0])
	}
}

func TestCrearProducto_Errores(t *testing.T) {
	casos := []struct {
		nombre  string
		datos   url.Values
		status  int
		mensaje string
	}{
		{"precio no numérico",
			url.Values{"nombre": {"x"}, "categoria": {"c"}, "precio": {"abc"}},
			http.StatusBadRequest, "Ingresá un precio válido"},
		{"precio doble no numérico",
			url.Values{"nombre": {"x"}, "categoria": {"c"}, "precio": {"10"}, "precio_doble": {"zzz"}},
			http.StatusBadRequest, "Ingresá un precio doble válido"},
		{"nombre vacío",
			url.Values{"nombre": {"  "}, "categoria": {"c"}, "precio": {"10"}},
			http.StatusBadRequest, "el nombre es obligatorio"},
		{"precio negativo",
			url.Values{"nombre": {"x"}, "categoria": {"c"}, "precio": {"-3"}},
			http.StatusBadRequest, "el precio no puede ser negativo"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			mux, q := setup(t)
			rec := postForm(mux, c.datos)

			if rec.Code != c.status {
				t.Errorf("status = %d, quiero %d", rec.Code, c.status)
			}
			if !strings.Contains(rec.Body.String(), c.mensaje) {
				t.Errorf("la página no muestra el mensaje %q", c.mensaje)
			}
			ps, _ := q.ListProductos(context.Background())
			if len(ps) != 0 {
				t.Errorf("no debería haberse guardado nada, hay %d productos", len(ps))
			}
		})
	}
}
