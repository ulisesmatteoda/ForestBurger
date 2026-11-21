package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
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
	api.RutasAdmin(mux)
	return mux, q
}

func postForm(mux http.Handler, datos url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/admin/productos", strings.NewReader(datos.Encode()))
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
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quiero 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, quiero text/html", ct)
	}
	body := rec.Body.String()
	for _, esperado := range []string{"<form", `hx-post="/admin/productos"`, `hx-target="#lista-productos"`, `id="lista-productos"`, `id="product-form-container"`, "Forest Clásica"} {
		if !strings.Contains(body, esperado) {
			t.Errorf("el HTML no contiene %q", esperado)
		}
	}
}

func TestCrearProducto_DevuelveFragmentosYGuarda(t *testing.T) {
	mux, q := setup(t)

	rec := postForm(mux, url.Values{
		"nombre":       {"Doble Bacon"},
		"categoria":    {"burger"},
		"precio":       {"12000"},
		"precio_doble": {"15000"},
		"descripcion":  {"con panceta"},
	})

	// Sin redirect: 200 y HTML parcial.
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quiero 200", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Errorf("no debería haber redirect, Location = %q", loc)
	}
	body := rec.Body.String()
	if strings.Contains(body, "<html") {
		t.Error("la respuesta debe ser un fragmento, no la página completa")
	}
	// Fragmento 1: lista actualizada. Fragmento 2: formulario vacío por OOB.
	for _, esperado := range []string{`id="lista-productos"`, "Doble Bacon", `id="product-form-container" hx-swap-oob="true"`} {
		if !strings.Contains(body, esperado) {
			t.Errorf("la respuesta no contiene %q", esperado)
		}
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
			http.StatusOK, "Ingresá un precio válido"},
		{"precio doble no numérico",
			url.Values{"nombre": {"x"}, "categoria": {"c"}, "precio": {"10"}, "precio_doble": {"zzz"}},
			http.StatusOK, "Ingresá un precio doble válido"},
		{"nombre vacío",
			url.Values{"nombre": {"  "}, "categoria": {"c"}, "precio": {"10"}},
			http.StatusOK, "el nombre es obligatorio"},
		{"precio negativo",
			url.Values{"nombre": {"x"}, "categoria": {"c"}, "precio": {"-3"}},
			http.StatusOK, "el precio no puede ser negativo"},
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

func TestBorrarProducto(t *testing.T) {
	mux, q := setup(t)
	p, err := q.CreateProducto(context.Background(), sqlc.CreateProductoParams{
		Nombre: "Para borrar", Categoria: "burger", Precio: "100.00",
	})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/admin/productos/"+strconv.Itoa(int(p.ID)), nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	// 200 y cuerpo vacío: HTMX reemplaza la fila por nada.
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quiero 200", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("el cuerpo debería estar vacío, vino %q", rec.Body.String())
	}
	ps, _ := q.ListProductos(context.Background())
	if len(ps) != 0 {
		t.Errorf("el producto no se borró, quedan %d", len(ps))
	}
}

func TestBorrarProducto_IDInvalido(t *testing.T) {
	mux, _ := setup(t)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/admin/productos/abc", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, quiero 400", rec.Code)
	}
}

// ---------- actualizar productos (HTMX: fila en modo edición / lectura) ----------

// crearParaEditar inserta un producto con precio doble y descripción, y devuelve su id como texto.
func crearParaEditar(t *testing.T, q *sqlc.Queries) (sqlc.Producto, string) {
	t.Helper()
	p, err := q.CreateProducto(context.Background(), sqlc.CreateProductoParams{
		Nombre:      "Forest Clasica",
		Categoria:   "hamburguesa",
		Precio:      "9500.00",
		PrecioDoble: sql.NullString{String: "12000.00", Valid: true},
		Descripcion: sql.NullString{String: "carne y cheddar", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	return p, strconv.Itoa(int(p.ID))
}

func pedir(mux http.Handler, metodo, ruta string, datos url.Values) *httptest.ResponseRecorder {
	var req *http.Request
	if datos == nil {
		req = httptest.NewRequest(metodo, ruta, nil)
	} else {
		req = httptest.NewRequest(metodo, ruta, strings.NewReader(datos.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestFormEditarProducto_MuestraFilaConDatos(t *testing.T) {
	mux, q := setup(t)
	_, id := crearParaEditar(t, q)

	rec := pedir(mux, http.MethodGet, "/admin/productos/"+id+"/editar", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quiero 200", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "<html") {
		t.Error("la respuesta debe ser solo la fila, no la página completa")
	}
	for _, esperado := range []string{
		`hx-put="/admin/productos/` + id + `"`,
		`value="Forest Clasica"`,
		`value="9500.00"`,
		`value="12000.00"`,
		"carne y cheddar",
	} {
		if !strings.Contains(body, esperado) {
			t.Errorf("la fila de edición no contiene %q", esperado)
		}
	}
}

func TestFilaProducto_VuelveAModoLectura(t *testing.T) {
	mux, q := setup(t)
	_, id := crearParaEditar(t, q)

	rec := pedir(mux, http.MethodGet, "/admin/productos/"+id, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quiero 200", rec.Code)
	}
	body := rec.Body.String()
	for _, esperado := range []string{"Forest Clasica", `hx-get="/admin/productos/` + id + `/editar"`} {
		if !strings.Contains(body, esperado) {
			t.Errorf("la fila no contiene %q", esperado)
		}
	}
	if strings.Contains(body, "hx-put=") {
		t.Error("la fila en modo lectura no debería tener el botón Guardar")
	}
}

func TestEditarProducto_NoExisteOIDInvalido(t *testing.T) {
	mux, _ := setup(t)
	casos := []struct {
		nombre string
		metodo string
		ruta   string
		datos  url.Values
		status int
	}{
		{"editar inexistente", http.MethodGet, "/admin/productos/9999/editar", nil, http.StatusNotFound},
		{"fila inexistente", http.MethodGet, "/admin/productos/9999", nil, http.StatusNotFound},
		{"editar id inválido", http.MethodGet, "/admin/productos/abc/editar", nil, http.StatusBadRequest},
		{"actualizar inexistente", http.MethodPut, "/admin/productos/9999",
			url.Values{"nombre": {"x"}, "categoria": {"hamburguesa"}, "precio": {"10"}}, http.StatusNotFound},
		{"actualizar id inválido", http.MethodPut, "/admin/productos/abc",
			url.Values{"nombre": {"x"}, "categoria": {"hamburguesa"}, "precio": {"10"}}, http.StatusBadRequest},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			rec := pedir(mux, c.metodo, c.ruta, c.datos)
			if rec.Code != c.status {
				t.Errorf("status = %d, quiero %d", rec.Code, c.status)
			}
			// Con 404 se pide recargar la página para que la fila vieja desaparezca.
			if c.status == http.StatusNotFound && rec.Header().Get("HX-Refresh") != "true" {
				t.Error("falta el header HX-Refresh: true en la respuesta 404")
			}
		})
	}
}

func TestActualizarProducto_GuardaYDevuelveFila(t *testing.T) {
	mux, q := setup(t)
	p, id := crearParaEditar(t, q)

	rec := pedir(mux, http.MethodPut, "/admin/productos/"+id, url.Values{
		"nombre":       {"Forest Triple"},
		"categoria":    {"veggie"},
		"precio":       {"11500"},
		"precio_doble": {"14000.50"},
		"descripcion":  {"triple carne"},
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quiero 200", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "<html") {
		t.Error("la respuesta debe ser solo la fila, no la página completa")
	}
	for _, esperado := range []string{"Forest Triple", `hx-get="/admin/productos/` + id + `/editar"`} {
		if !strings.Contains(body, esperado) {
			t.Errorf("la fila devuelta no contiene %q", esperado)
		}
	}

	got, err := q.GetProducto(context.Background(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Nombre != "Forest Triple" || got.Categoria != "veggie" || got.Precio != "11500.00" {
		t.Errorf("producto actualizado incorrecto: %+v", got)
	}
	if !got.PrecioDoble.Valid || got.PrecioDoble.String != "14000.50" {
		t.Errorf("precio doble = %+v, quiero 14000.50", got.PrecioDoble)
	}
	if !got.Descripcion.Valid || got.Descripcion.String != "triple carne" {
		t.Errorf("descripción = %+v, quiero 'triple carne'", got.Descripcion)
	}
}

func TestActualizarProducto_PrecioDobleVacioLoBorra(t *testing.T) {
	mux, q := setup(t)
	p, id := crearParaEditar(t, q)

	rec := pedir(mux, http.MethodPut, "/admin/productos/"+id, url.Values{
		"nombre":       {"Forest Clasica"},
		"categoria":    {"hamburguesa"},
		"precio":       {"9500"},
		"precio_doble": {""},
		"descripcion":  {""},
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quiero 200", rec.Code)
	}
	got, err := q.GetProducto(context.Background(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PrecioDoble.Valid {
		t.Errorf("el precio doble debería quedar en NULL, quedó %+v", got.PrecioDoble)
	}
	if got.Descripcion.Valid {
		t.Errorf("la descripción debería quedar en NULL, quedó %+v", got.Descripcion)
	}
}

func TestActualizarProducto_Errores(t *testing.T) {
	casos := []struct {
		nombre  string
		datos   url.Values
		mensaje string
	}{
		{"precio no numérico",
			url.Values{"nombre": {"Nuevo"}, "categoria": {"hamburguesa"}, "precio": {"abc"}},
			"Ingresá un precio válido"},
		{"precio doble no numérico",
			url.Values{"nombre": {"Nuevo"}, "categoria": {"hamburguesa"}, "precio": {"10"}, "precio_doble": {"zzz"}},
			"Ingresá un precio doble válido"},
		{"nombre vacío",
			url.Values{"nombre": {"  "}, "categoria": {"hamburguesa"}, "precio": {"10"}},
			"el nombre es obligatorio"},
		{"categoría vacía",
			url.Values{"nombre": {"Nuevo"}, "categoria": {""}, "precio": {"10"}},
			"la categoría es obligatoria"},
		{"precio negativo",
			url.Values{"nombre": {"Nuevo"}, "categoria": {"hamburguesa"}, "precio": {"-3"}},
			"el precio no puede ser negativo"},
		{"precio doble negativo",
			url.Values{"nombre": {"Nuevo"}, "categoria": {"hamburguesa"}, "precio": {"10"}, "precio_doble": {"-1"}},
			"el precio doble no puede ser negativo"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			mux, q := setup(t)
			p, id := crearParaEditar(t, q)

			rec := pedir(mux, http.MethodPut, "/admin/productos/"+id, c.datos)

			// 200 y la fila sigue en modo edición con el mensaje (HTMX no hace swap de los 4xx).
			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, quiero 200", rec.Code)
			}
			body := rec.Body.String()
			if !strings.Contains(body, c.mensaje) {
				t.Errorf("la fila no muestra el mensaje %q", c.mensaje)
			}
			if !strings.Contains(body, `hx-put="/admin/productos/`+id+`"`) {
				t.Error("la fila debería seguir en modo edición")
			}
			// Lo que se escribió se conserva para no obligar a tipearlo de nuevo.
			if !strings.Contains(body, `name="nombre"`) {
				t.Error("la fila debería conservar los campos")
			}

			got, err := q.GetProducto(context.Background(), p.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Nombre != "Forest Clasica" || got.Precio != "9500.00" {
				t.Errorf("el producto no debería haber cambiado: %+v", got)
			}
		})
	}
}
