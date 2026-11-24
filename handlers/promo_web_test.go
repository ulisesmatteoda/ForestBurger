package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	sqlc "forestburger/db/sqlc"
)

// catalogoPromos carga las hamburguesas del Figma (con doble), una veggie sin doble y una bebida
// (que no entra en promos). Devuelve los ids por nombre.
func catalogoPromos(t *testing.T, q *sqlc.Queries) map[string]int32 {
	t.Helper()
	ids := map[string]int32{}
	for _, p := range []struct{ nombre, categoria, simple, doble string }{
		{"Big Forest", "hamburguesa", "14500", "16500"},
		{"Cheese Burger", "hamburguesa", "14000", "16000"},
		{"Classic", "hamburguesa", "14500", "16500"},
		{"Oklahoma", "hamburguesa", "14500", "16500"},
		{"Not Cheese", "veggie", "14500", ""},
		{"Gaseosa lata", "bebida", "3500", ""},
	} {
		creado, err := q.CreateProducto(context.Background(), sqlc.CreateProductoParams{
			Nombre: p.nombre, Categoria: p.categoria, Precio: p.simple, PrecioDoble: sql.NullString{String: p.doble, Valid: p.doble != ""},
		})
		if err != nil {
			t.Fatal(err)
		}
		ids[p.nombre] = creado.ID
	}
	return ids
}

// formPromoFigma es el form de "2 Smash por $26.000": 2 simples entre 4, martes a domingo.
func formPromoFigma(ids map[string]int32) url.Values {
	return url.Values{
		"nombre": {"2 Smash por $26.000"}, "precio": {"26000"}, "cantidad": {"2"}, "tamano": {"simple"},
		"producto": {idTexto(ids["Big Forest"]), idTexto(ids["Cheese Burger"]), idTexto(ids["Classic"]), idTexto(ids["Oklahoma"])},
		"dia":      {"2", "3", "4", "5", "6", "0"},
	}
}

// enviarPromo manda el form como lo manda htmx al guardar: multipart (con foto, si viene).
func enviarPromo(t *testing.T, mux http.Handler, ruta string, campos url.Values, foto []byte) *httptest.ResponseRecorder {
	t.Helper()
	var cuerpo bytes.Buffer
	mw := multipart.NewWriter(&cuerpo)
	for k, vs := range campos {
		for _, v := range vs {
			mw.WriteField(k, v)
		}
	}
	if foto != nil {
		fw, _ := mw.CreateFormFile("foto", "promo.png")
		fw.Write(foto)
	}
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, ruta, &cuerpo)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("HX-Request", "true")
	return servir(mux, req)
}

func TestPromos_PaginaVaciaYSidebar(t *testing.T) {
	mux, _ := setup(t)
	body := pedirPanel(mux, "/admin/promos", false).Body.String()
	for _, esperado := range []string{
		"<h1>PROMOS</h1>", `+ NUEVA <span class="solo-compu">PROMO</span>`, "Todavía no hay promos", "0 promos",
		`href="/admin/promos" class="nav-item activo"`,
	} {
		if !strings.Contains(body, esperado) {
			t.Errorf("la página no tiene %q", esperado)
		}
	}
	// En el sidebar va después de Productos y antes de Extras.
	if i, j, k := strings.Index(body, `>Productos<`), strings.Index(body, `>Promos<`), strings.Index(body, `>Extras<`); !(i < j && j < k) {
		t.Errorf("orden del sidebar: Productos %d, Promos %d, Extras %d", i, j, k)
	}
}

func TestNuevaPromo_Modal(t *testing.T) {
	mux, q := setup(t)
	catalogoPromos(t, q)
	body := pedirPanel(mux, "/admin/promos/nueva", true).Body.String()
	for _, esperado := range []string{
		"NUEVA PROMO", "HAMBURGUESAS", "VEGGIE", "Big Forest", "$14.500 por separado", "Not Cheese",
		"Qué productos entran (0 de 5)", `name="cantidad" value="2"`, `value="simple" checked`,
		"ASÍ LO VE EL CLIENTE", "Acá va la foto", "Marcá los productos", `enctype="multipart/form-data"`,
		// Los chips de días no pueden usar la clase .dia: es la fila de horarios de Ajustes (una
		// grilla) y desarmaba los chips.
		`<label class="chip chip-dia" title="Lunes">`,
	} {
		if !strings.Contains(body, esperado) {
			t.Errorf("el modal no tiene %q", esperado)
		}
	}
	if strings.Contains(body, "Gaseosa lata") {
		t.Error("las bebidas no entran en promos")
	}
	if strings.Count(body, `name="dia"`) != 7 || strings.Count(body, `name="dia" value="`) != strings.Count(body, "checked aria-label") {
		t.Error("los 7 días tendrían que arrancar marcados")
	}
}

func TestVistaPromo_RecalculaComoElFigma(t *testing.T) {
	mux, q := setup(t)
	ids := catalogoPromos(t, q)
	body := pedirExtra(mux, http.MethodPost, "/admin/promos/vista", formPromoFigma(ids), true).Body.String()
	for _, esperado := range []string{
		"Elegí 2 hamburguesas simples entre Big Forest, Cheese Burger, Classic y Oklahoma. Martes a domingo.",
		"$28.000", "$29.000", "hasta $3.000", "Ahorrás hasta $3.000", "Qué productos entran (4 de 5)",
	} {
		if !strings.Contains(body, esperado) {
			t.Errorf("la vista no tiene %q", esperado)
		}
	}
	// El stepper: + sube la cantidad (y las cuentas); no pasa de 10.
	f := formPromoFigma(ids)
	f.Set("paso", "1")
	if body := pedirExtra(mux, http.MethodPost, "/admin/promos/vista", f, true).Body.String(); !strings.Contains(body, `name="cantidad" value="3"`) || !strings.Contains(body, "$42.000") {
		t.Error("el + tendría que subir a 3 (3 × 14.000 = 42.000)")
	}
	// Dobles: la veggie sin doble queda sin poder marcarse.
	f = formPromoFigma(ids)
	f.Set("tamano", "doble")
	if body := pedirExtra(mux, http.MethodPost, "/admin/promos/vista", f, true).Body.String(); !strings.Contains(body, "sin doble") || !strings.Contains(body, "$16.500 por separado") || !strings.Contains(body, "van dobles") {
		t.Error("con dobles los precios son los dobles y la que no tiene, sin doble")
	}
	// Marcar todas: todas las que se pueden.
	f = url.Values{"tamano": {"simple"}, "cantidad": {"2"}, "marcar": {"todas"}}
	if body := pedirExtra(mux, http.MethodPost, "/admin/promos/vista", f, true).Body.String(); !strings.Contains(body, "(5 de 5)") {
		t.Error("marcar todas tendría que marcar las 5")
	}
	// Más cara que por separado: avisa.
	f = formPromoFigma(ids)
	f.Set("precio", "30000")
	if body := pedirExtra(mux, http.MethodPost, "/admin/promos/vista", f, true).Body.String(); !strings.Contains(body, "Ojo: sale más cara que por separado") {
		t.Error("a $30.000 tendría que avisar que sale más cara")
	}
}

func TestCrearPromo(t *testing.T) {
	mux, q, api := setupAPI(t)
	guardarAdminTest(t, q)
	mux2 := conSesion(mux, adminTest)
	ids := catalogoPromos(t, q)

	rec := enviarPromo(t, mux2, "/admin/promos", formPromoFigma(ids), pngDePrueba(t))
	body := rec.Body.String()
	for _, esperado := range []string{
		`id="panel-promos"`, "1 promo", "2 Smash por $26.000", "Elegí 2 hamburguesas simples entre:", "Oklahoma",
		"Martes a domingo", "Ahorran hasta $3.000", "Activa: se ve en el menú", `class="promo-tarjeta nuevo"`, `<div id="modal" hx-swap-oob="innerHTML"></div>`,
	} {
		if !strings.Contains(body, esperado) {
			t.Errorf("después de crear falta %q", esperado)
		}
	}
	lista, _ := q.ListPromos(context.Background())
	if len(lista) != 1 || lista[0].Precio != "26000.00" || lista[0].Cantidad != 2 || len(lista[0].Dias) != 6 || !lista[0].Activa || !strings.HasPrefix(lista[0].ImagenUrl.String, "/uploads/") {
		t.Fatalf("promo guardada: %+v", lista)
	}
	if productos, _ := q.ListProductosDePromo(context.Background(), lista[0].ID); len(productos) != 4 {
		t.Errorf("tendría que tener 4 productos, tiene %d", len(productos))
	}
	if fotos := archivosEn(t, api.dirFotos); len(fotos) != 1 {
		t.Errorf("tendría que haber 1 foto en disco, hay %v", fotos)
	}
}

func TestEditarPromo(t *testing.T) {
	mux, q := setup(t)
	ids := catalogoPromos(t, q)
	enviarPromo(t, mux, "/admin/promos", formPromoFigma(ids), nil)
	lista, _ := q.ListPromos(context.Background())
	id := idTexto(lista[0].ID)

	// El modal de edición viene con lo guardado.
	body := pedirPanel(mux, "/admin/promos/"+id+"/editar", true).Body.String()
	for _, esperado := range []string{"EDITAR PROMO", `value="2 Smash por $26.000"`, `value="26000"`, "(4 de 5)", `action="/admin/promos/` + id + `"`} {
		if !strings.Contains(body, esperado) {
			t.Errorf("el modal de edición no tiene %q", esperado)
		}
	}
	// Cambiar a 1 doble entre dos, solo martes.
	f := url.Values{"nombre": {"Martes doble"}, "precio": {"15000"}, "cantidad": {"1"}, "tamano": {"doble"},
		"producto": {idTexto(ids["Classic"]), idTexto(ids["Oklahoma"])}, "dia": {"2"}}
	body = enviarPromo(t, mux, "/admin/promos/"+id, f, nil).Body.String()
	if !strings.Contains(body, "Martes doble") || !strings.Contains(body, "Solo martes") || !strings.Contains(body, "Ahorran hasta $1.500") {
		t.Errorf("la tarjeta editada: %s", body)
	}
	if productos, _ := q.ListProductosDePromo(context.Background(), lista[0].ID); len(productos) != 2 {
		t.Errorf("tendría que quedar con 2 productos, tiene %d", len(productos))
	}
}

func TestCrearPromo_Errores(t *testing.T) {
	casos := map[string]struct {
		romper  func(f url.Values, ids map[string]int32)
		mensaje string
	}{
		"sin nombre":    {func(f url.Values, _ map[string]int32) { f.Set("nombre", "") }, "Falta el nombre de la promo"},
		"sin precio":    {func(f url.Values, _ map[string]int32) { f.Set("precio", "") }, "Poné el precio de la promo"},
		"sin productos": {func(f url.Values, _ map[string]int32) { f.Del("producto") }, "Marcá al menos un producto"},
		"sin días":      {func(f url.Values, _ map[string]int32) { f.Del("dia") }, "Elegí al menos un día"},
		"doble sin doble": {func(f url.Values, ids map[string]int32) {
			f.Set("tamano", "doble")
			f.Set("producto", idTexto(ids["Not Cheese"]))
		}, "Marcá al menos un producto"},
	}
	for nombre, c := range casos {
		t.Run(nombre, func(t *testing.T) {
			mux, q := setup(t)
			ids := catalogoPromos(t, q)
			f := formPromoFigma(ids)
			c.romper(f, ids)
			rec := enviarPromo(t, mux, "/admin/promos", f, nil)
			if !strings.Contains(rec.Body.String(), c.mensaje) || rec.Header().Get("HX-Retarget") != "#modal" {
				t.Errorf("tendría que volver el modal con %q: %s", c.mensaje, rec.Body.String())
			}
			if lista, _ := q.ListPromos(context.Background()); len(lista) != 0 {
				t.Error("con un error no tendría que guardar nada")
			}
		})
	}
}

func TestCambiarActivaYBorrarPromo(t *testing.T) {
	mux, q := setup(t)
	ids := catalogoPromos(t, q)
	enviarPromo(t, mux, "/admin/promos", formPromoFigma(ids), nil)
	lista, _ := q.ListPromos(context.Background())
	id := idTexto(lista[0].ID)

	// Pausar: vuelve solo la tarjeta.
	body := pedirExtra(mux, http.MethodPut, "/admin/promos/"+id+"/activa", url.Values{}, true).Body.String()
	if !strings.Contains(body, "Pausada") || !strings.Contains(body, `class="promo-tarjeta apagado"`) || strings.Contains(body, "panel-promos") {
		t.Errorf("pausar tendría que devolver la tarjeta pausada: %s", body)
	}
	if p, _ := q.GetPromo(context.Background(), lista[0].ID); p.Activa {
		t.Error("no se guardó la pausa")
	}

	// Borrar: primero confirma, después vuelve el panel vacío.
	if body := pedirPanel(mux, "/admin/promos/"+id+"/borrar", true).Body.String(); !strings.Contains(body, "¿BORRAR PROMO?") || !strings.Contains(body, "mejor") {
		t.Errorf("tendría que pedir confirmación: %s", body)
	}
	body = pedirExtra(mux, http.MethodDelete, "/admin/promos/"+id, nil, true).Body.String()
	if !strings.Contains(body, "Todavía no hay promos") {
		t.Errorf("después de borrar: %s", body)
	}
	if lista, _ := q.ListPromos(context.Background()); len(lista) != 0 {
		t.Error("no se borró")
	}
}

// Borrar un producto que está en una promo activa avisa; al borrarlo sale de la promo.
func TestBorrarProducto_AvisaSiEstaEnUnaPromo(t *testing.T) {
	mux, q := setup(t)
	ids := catalogoPromos(t, q)
	enviarPromo(t, mux, "/admin/promos", formPromoFigma(ids), nil)

	body := pedirPanel(mux, "/admin/productos/"+idTexto(ids["Classic"])+"/borrar", true).Body.String()
	if !strings.Contains(body, "Está en la promo 2 Smash por $26.000") {
		t.Errorf("tendría que avisar que está en la promo: %s", body)
	}
	pedirExtra(mux, http.MethodDelete, "/admin/productos/"+idTexto(ids["Classic"]), nil, true)
	lista, _ := q.ListPromos(context.Background())
	if productos, _ := q.ListProductosDePromo(context.Background(), lista[0].ID); len(productos) != 3 {
		t.Errorf("la promo tendría que quedar con 3 productos, tiene %d", len(productos))
	}
}

func TestPromos_PideSesion(t *testing.T) {
	mux, _ := setupSinSesion(t)
	if rec := pedirPanel(mux, "/admin/promos/nueva", true); rec.Header().Get("HX-Redirect") != "/login" {
		t.Error("sin sesión tendría que mandar al login")
	}
}
