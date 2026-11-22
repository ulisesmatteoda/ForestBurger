package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	sqlc "forestburger/db/sqlc"
)

// pedirExtra manda un form al panel de extras; con htmx=true, como lo manda el modal.
func pedirExtra(mux http.Handler, metodo, ruta string, datos url.Values, htmx bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(metodo, ruta, strings.NewReader(datos.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func crearExtra(t *testing.T, q *sqlc.Queries, nombre, precio string, disponible bool) sqlc.Extra {
	t.Helper()
	e, err := q.CreateExtra(context.Background(), sqlc.CreateExtraParams{Nombre: nombre, Precio: precio, Disponible: disponible})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func idTexto(id int32) string { return strconv.Itoa(int(id)) }

func TestExtras_PaginaVacia(t *testing.T) {
	mux, _ := setup(t)

	rec := pedirPanel(mux, "/admin/extras", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quiero 200", rec.Code)
	}
	body := rec.Body.String()
	for _, esperado := range []string{
		"<h1>EXTRAS</h1>",
		`href="/admin/extras" class="nav-item activo" aria-current="page">Extras`,
		`hx-get="/admin/extras/nuevo"`,
		"Todavía no hay extras.",
		"0 extras",
	} {
		if !strings.Contains(body, esperado) {
			t.Errorf("la página no contiene %q", esperado)
		}
	}
}

func TestExtras_ListaConPrecioYSwitch(t *testing.T) {
	mux, q := setup(t)
	crearExtra(t, q, "Cheddar", "1000.00", true)
	panceta := crearExtra(t, q, "Panceta", "1500.00", false)

	body := pedirPanel(mux, "/admin/extras", false).Body.String()
	for _, esperado := range []string{
		"Cheddar", "+ $1.000", "Panceta", "+ $1.500", "2 extras",
		`role="switch"`, `hx-put="/admin/extras/` + idTexto(panceta.ID) + `/disponible"`,
		`class="apagado"`, // Panceta no está disponible: la fila sale atenuada
	} {
		if !strings.Contains(body, esperado) {
			t.Errorf("la lista no contiene %q", esperado)
		}
	}
}

func TestExtras_PideSesion(t *testing.T) {
	mux, _ := setupSinSesion(t)
	if rec := pedirPanel(mux, "/admin/extras", false); rec.Code != http.StatusSeeOther {
		t.Errorf("sin sesión: status = %d, quiero 303 al login", rec.Code)
	}
}

func TestNuevoExtra_Modal(t *testing.T) {
	mux, _ := setup(t)

	rec := pedirPanel(mux, "/admin/extras/nuevo", true)
	body := rec.Body.String()
	if strings.Contains(body, "<html") {
		t.Error("con htmx tendría que devolver solo el modal")
	}
	for _, esperado := range []string{"NUEVO EXTRA", `hx-post="/admin/extras"`, `name="disponible"`, " checked", "GUARDAR EXTRA"} {
		if !strings.Contains(body, esperado) {
			t.Errorf("el modal no contiene %q", esperado)
		}
	}
	// Sin htmx: la página con el modal abierto.
	if body := pedirPanel(mux, "/admin/extras/nuevo", false).Body.String(); !strings.Contains(body, "<html") || !strings.Contains(body, "NUEVO EXTRA") {
		t.Error("sin htmx tendría que devolver la página con el modal abierto")
	}
}

func TestCrearExtra_Bien(t *testing.T) {
	mux, q := setup(t)

	rec := pedirExtra(mux, http.MethodPost, "/admin/extras", url.Values{"nombre": {"  Cheddar "}, "precio": {"1000"}, "disponible": {"1"}}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quiero 200", rec.Code)
	}
	body := rec.Body.String()
	for _, esperado := range []string{`id="panel-extras"`, "Cheddar", `class="nuevo"`, `<div id="modal" hx-swap-oob="innerHTML"></div>`, "1 extra"} {
		if !strings.Contains(body, esperado) {
			t.Errorf("la respuesta no contiene %q", esperado)
		}
	}
	extras, _ := q.ListExtras(context.Background())
	if len(extras) != 1 || extras[0].Nombre != "Cheddar" || extras[0].Precio != "1000.00" || !extras[0].Disponible {
		t.Errorf("extra guardado incorrecto: %+v", extras)
	}
}

func TestCrearExtra_SinDisponibleQuedaApagado(t *testing.T) {
	mux, q := setup(t)
	pedirExtra(mux, http.MethodPost, "/admin/extras", url.Values{"nombre": {"Panceta"}, "precio": {"1500"}}, true)
	if extras, _ := q.ListExtras(context.Background()); len(extras) != 1 || extras[0].Disponible {
		t.Errorf("sin el switch prendido tendría que quedar no disponible: %+v", extras)
	}
}

func TestCrearExtra_Errores(t *testing.T) {
	casos := map[string]struct {
		datos   url.Values
		mensaje string
	}{
		"nombre vacío":     {url.Values{"nombre": {"  "}, "precio": {"1000"}}, "El nombre es obligatorio"},
		"precio inválido":  {url.Values{"nombre": {"Cheddar"}, "precio": {"mil"}}, "Ingresá un precio válido"},
		"precio negativo":  {url.Values{"nombre": {"Cheddar"}, "precio": {"-5"}}, "El precio no puede ser negativo"},
		"nombre muy largo": {url.Values{"nombre": {strings.Repeat("a", 101)}, "precio": {"1"}}, "El nombre es muy largo"},
	}
	for nombre, c := range casos {
		t.Run(nombre, func(t *testing.T) {
			mux, q := setup(t)
			rec := pedirExtra(mux, http.MethodPost, "/admin/extras", c.datos, true)
			if rec.Code != http.StatusOK || rec.Header().Get("HX-Retarget") != "#modal" {
				t.Errorf("status = %d, HX-Retarget = %q; quiero 200 y #modal", rec.Code, rec.Header().Get("HX-Retarget"))
			}
			if body := rec.Body.String(); !strings.Contains(body, c.mensaje) || strings.Contains(body, `id="panel-extras"`) {
				t.Errorf("tendría que volver el modal con %q: %s", c.mensaje, body)
			}
			if extras, _ := q.ListExtras(context.Background()); len(extras) != 0 {
				t.Errorf("no tendría que guardar nada, hay %d", len(extras))
			}
		})
	}
}

func TestCrearExtra_SinHTMX(t *testing.T) {
	mux, _ := setup(t)

	rec := pedirExtra(mux, http.MethodPost, "/admin/extras", url.Values{"nombre": {"Cheddar"}, "precio": {"1000"}}, false)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/admin/extras" {
		t.Errorf("status = %d, Location = %q; quiero 303 a /admin/extras", rec.Code, rec.Header().Get("Location"))
	}
	rec = pedirExtra(mux, http.MethodPost, "/admin/extras", url.Values{"nombre": {""}, "precio": {"1000"}}, false)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "<html") {
		t.Errorf("con error sin htmx: status = %d; quiero 400 con la página", rec.Code)
	}
}

func TestEditarExtra(t *testing.T) {
	mux, q := setup(t)
	e := crearExtra(t, q, "Cheddar", "1000.00", true)
	id := idTexto(e.ID)

	// El modal sale con los datos cargados.
	body := pedirPanel(mux, "/admin/extras/"+id+"/editar", true).Body.String()
	for _, esperado := range []string{"EDITAR EXTRA", `value="Cheddar"`, `value="1000.00"`, `hx-post="/admin/extras/` + id + `"`} {
		if !strings.Contains(body, esperado) {
			t.Errorf("el modal de edición no contiene %q", esperado)
		}
	}

	rec := pedirExtra(mux, http.MethodPost, "/admin/extras/"+id, url.Values{"nombre": {"Cheddar doble"}, "precio": {"1800"}}, true)
	if !strings.Contains(rec.Body.String(), "Cheddar doble") {
		t.Errorf("el panel no muestra el cambio: %s", rec.Body.String())
	}
	actualizado, _ := q.GetExtra(context.Background(), e.ID)
	if actualizado.Nombre != "Cheddar doble" || actualizado.Precio != "1800.00" || actualizado.Disponible {
		t.Errorf("extra actualizado incorrecto: %+v", actualizado)
	}

	if rec := pedirPanel(mux, "/admin/extras/9999/editar", true); rec.Code != http.StatusNotFound {
		t.Errorf("extra inexistente: status = %d, quiero 404", rec.Code)
	}
}

func TestCambiarDisponible(t *testing.T) {
	mux, q := setup(t)
	e := crearExtra(t, q, "Cheddar", "1000.00", true)
	ruta := "/admin/extras/" + idTexto(e.ID) + "/disponible"

	// Apagar: el checkbox apagado no viaja.
	rec := pedirExtra(mux, http.MethodPut, ruta, url.Values{}, true)
	if body := rec.Body.String(); !strings.Contains(body, `class="apagado"`) || !strings.Contains(body, "<span>No</span>") || strings.Contains(body, "<table") {
		t.Errorf("tendría que devolver solo la fila apagada: %s", body)
	}
	if got, _ := q.GetExtra(context.Background(), e.ID); got.Disponible {
		t.Error("no se guardó el apagado")
	}

	// Prender.
	rec = pedirExtra(mux, http.MethodPut, ruta, url.Values{"disponible": {"1"}}, true)
	if !strings.Contains(rec.Body.String(), "<span>Sí</span>") {
		t.Errorf("tendría que volver prendido: %s", rec.Body.String())
	}
	if got, _ := q.GetExtra(context.Background(), e.ID); !got.Disponible {
		t.Error("no se guardó el prendido")
	}
}

func TestBorrarExtra(t *testing.T) {
	mux, q := setup(t)
	e := crearExtra(t, q, "Cheddar", "1000.00", true)
	id := idTexto(e.ID)

	body := pedirPanel(mux, "/admin/extras/"+id+"/borrar", true).Body.String()
	for _, esperado := range []string{`role="alertdialog"`, "<strong>Cheddar</strong>", `hx-delete="/admin/extras/` + id + `"`} {
		if !strings.Contains(body, esperado) {
			t.Errorf("la confirmación no contiene %q", esperado)
		}
	}

	rec := pedirExtra(mux, http.MethodDelete, "/admin/extras/"+id, nil, true)
	if body := rec.Body.String(); !strings.Contains(body, `id="panel-extras"`) || !strings.Contains(body, `hx-swap-oob="innerHTML"`) || !strings.Contains(body, "Todavía no hay extras.") {
		t.Errorf("tendría que devolver el panel vacío y cerrar el modal: %s", body)
	}
	if extras, _ := q.ListExtras(context.Background()); len(extras) != 0 {
		t.Errorf("no se borró, quedan %d", len(extras))
	}

	if rec := pedirExtra(mux, http.MethodDelete, "/admin/extras/1", nil, false); rec.Code != http.StatusSeeOther {
		t.Errorf("sin htmx: status = %d, quiero 303", rec.Code)
	}
}
