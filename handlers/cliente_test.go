package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	sqlc "forestburger/db/sqlc"
	"forestburger/pedidos"
)

// navegador es un cliente que guarda las cookies entre pedidos (el carrito y los datos).
type navegador struct {
	t       *testing.T
	mux     http.Handler
	cookies map[string]*http.Cookie
}

func (n *navegador) pedir(metodo, ruta string, form url.Values, htmx bool) *httptest.ResponseRecorder {
	n.t.Helper()
	var req *http.Request
	if form != nil {
		req = httptest.NewRequest(metodo, ruta, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(metodo, ruta, nil)
	}
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	for _, c := range n.cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	n.mux.ServeHTTP(rec, req)
	for _, c := range rec.Result().Cookies() {
		if c.MaxAge < 0 {
			delete(n.cookies, c.Name)
		} else {
			n.cookies[c.Name] = c
		}
	}
	return rec
}

type tiendaTest struct {
	nav                     *navegador
	q                       *sqlc.Queries
	db                      *sql.DB
	classic, bacon, gaseosa sqlc.Producto
	cheddar                 sqlc.Extra
}

// setupCliente arma la web con un local abierto a toda hora (horario de 00:00 a 00:00), delivery
// a $1.500, alias y WhatsApp; Classic, Cheese Bacon (agotado) y Gaseosa lata; y el cheddar.
func setupCliente(t *testing.T) tiendaTest {
	t.Helper()
	mux, q, api := setupAPI(t)
	ctx := context.Background()
	if _, err := api.db.Exec(`UPDATE horario SET abierto = true, desde = '00:00', hasta = '00:00'`); err != nil {
		t.Fatal(err)
	}
	if err := q.UpdateAjustes(ctx, sqlc.UpdateAjustesParams{Whatsapp: "2236001122", AliasCbu: "forest.burger.mp", Titular: "Forest SRL", DeliveryActivo: true, CostoEnvio: "1500"}); err != nil {
		t.Fatal(err)
	}
	crear := func(nombre, cat, simple, doble string) sqlc.Producto {
		p, err := q.CreateProducto(ctx, sqlc.CreateProductoParams{Nombre: nombre, Categoria: cat, Precio: simple, PrecioDoble: sql.NullString{String: doble, Valid: doble != ""},
			Descripcion: sql.NullString{String: "Rica.", Valid: true}})
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	tt := tiendaTest{nav: &navegador{t: t, mux: mux, cookies: map[string]*http.Cookie{}}, q: q, db: api.db}
	tt.classic = crear("Classic", "hamburguesa", "14500", "16500")
	tt.bacon = crear("Cheese Bacon", "hamburguesa", "15500", "17000")
	if _, err := q.UpdateDisponibleProducto(ctx, sqlc.UpdateDisponibleProductoParams{ID: tt.bacon.ID, Disponible: false}); err != nil {
		t.Fatal(err)
	}
	tt.gaseosa = crear("Gaseosa lata", "bebida", "3500", "")
	tt.cheddar = crearExtra(t, q, "Cheddar", "1000", true)
	return tt
}

func debeContener(t *testing.T, que, body string, esperados ...string) {
	t.Helper()
	for _, e := range esperados {
		if !strings.Contains(body, e) {
			t.Errorf("%s: falta %q", que, e)
		}
	}
}

func TestInicio_AbiertoYCerrado(t *testing.T) {
	tt := setupCliente(t)
	body := tt.nav.pedir(http.MethodGet, "/", nil, false).Body.String()
	debeContener(t, "inicio abierto", body, `href="/menu"`, "VER EL MENÚ")
	if strings.Contains(body, "AHORA ESTAMOS CERRADOS") {
		t.Error("abierto no tendría que decir cerrado")
	}
	if _, err := tt.q.SetLocalAbierto(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	body = tt.nav.pedir(http.MethodGet, "/", nil, false).Body.String()
	debeContener(t, "inicio cerrado", body, "AHORA ESTAMOS CERRADOS", "Lunes a domingo: 00:00 a 00:00", "Podés mirar los precios")
	// El menú se ve, pero sin botones para agregar.
	body = tt.nav.pedir(http.MethodGet, "/menu", nil, false).Body.String()
	debeContener(t, "menú cerrado", body, "Ahora estamos cerrados", "Classic")
	if strings.Contains(body, `class="cli-mas"`) || strings.Contains(body, "/carrito/rapido") {
		t.Error("cerrado no se puede agregar nada")
	}
}

func TestMenu(t *testing.T) {
	tt := setupCliente(t)
	ctx := context.Background()
	// Una promo activa todos los días.
	pr, err := tt.q.CreatePromo(ctx, sqlc.CreatePromoParams{Nombre: "2 Smash", Precio: "26000", Cantidad: 2, Tamano: "simple", Dias: []int32{0, 1, 2, 3, 4, 5, 6}, Activa: true})
	if err != nil {
		t.Fatal(err)
	}
	tt.q.AddProductoAPromo(ctx, sqlc.AddProductoAPromoParams{PromoID: pr.ID, ProductoID: tt.classic.ID})
	body := tt.nav.pedir(http.MethodGet, "/menu", nil, false).Body.String()
	debeContener(t, "menú", body,
		`href="#promos"`, `href="#smash"`, `href="#bebidas"`, "2 Smash", "Elegí 2 hamburguesas simples entre Classic.",
		"SMASH", "BEBIDAS", "14.500", "16.500", idTexto(tt.classic.ID), "AGOTADO", `id="rapida-`+idTexto(tt.gaseosa.ID)+`"`,
	)
	if strings.Contains(body, "/menu/"+idTexto(tt.bacon.ID)+`"`) {
		t.Error("un agotado no tiene el + para agregar")
	}
}

func TestPedidoCompleto(t *testing.T) {
	tt := setupCliente(t)
	n := tt.nav
	classic, ched := idTexto(tt.classic.ID), idTexto(tt.cheddar.ID)

	// La burger: sumar un cheddar recalcula el precio (14.500 + 1.000).
	body := n.pedir(http.MethodPost, "/menu/"+classic+"/armar", url.Values{"producto": {classic}, "tamano": {"simple"}, "cantidad": {"1"}, "extra_" + ched: {"0"}, "extra": {ched}, "paso": {"1"}, "aclaraciones": {"sin cebolla"}}, true).Body.String()
	debeContener(t, "burger", body, `name="extra_`+ched+`" value="1"`, "$15.500", `value="sin cebolla"`)

	// Agregar: vuelve al menú.
	rec := n.pedir(http.MethodPost, "/carrito", url.Values{"producto": {classic}, "tamano": {"simple"}, "cantidad": {"1"}, "extra_" + ched: {"1"}, "aclaraciones": {"sin cebolla"}}, false)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/menu" {
		t.Fatalf("agregar = %d %s", rec.Code, rec.Header().Get("Location"))
	}
	// Dos gaseosas desde el menú: la fila y la barra de abajo por OOB.
	n.pedir(http.MethodPost, "/carrito/rapido", url.Values{"producto": {idTexto(tt.gaseosa.ID)}, "paso": {"1"}}, true)
	body = n.pedir(http.MethodPost, "/carrito/rapido", url.Values{"producto": {idTexto(tt.gaseosa.ID)}, "paso": {"1"}}, true).Body.String()
	debeContener(t, "rápido", body, "<span>2</span>", `id="barra-pedido"`, "3 productos · $22.500", `id="boton-carrito"`)

	body = n.pedir(http.MethodGet, "/menu", nil, false).Body.String()
	debeContener(t, "menú con pedido", body, "1 en el pedido", "HACER PEDIDO")

	// Tu pedido: delivery por defecto (suma el envío).
	body = n.pedir(http.MethodGet, "/carrito", nil, false).Body.String()
	debeContener(t, "carrito", body, "Classic · Simple", "+ Cheddar x1", "“sin cebolla”", "Gaseosa lata", "Envío", "$24.000", `value="delivery" checked`)

	datos := url.Values{"nombre": {"Ulises"}, "telefono": {"223 555 1234"}, "entrega": {"delivery"}, "direccion": {"Av. Colón 1234"}, "pago": {"transferencia"}, "notas": {"Tocar timbre 2B"}}
	sinTel := url.Values{}
	for k, v := range datos {
		sinTel[k] = v
	}
	sinTel.Set("telefono", "")
	debeContener(t, "sin teléfono", n.pedir(http.MethodPost, "/carrito/revisar", sinTel, false).Body.String(), "Poné un teléfono", `value="Ulises"`)

	rec = n.pedir(http.MethodPost, "/carrito/revisar", datos, false)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/carrito/revisar" {
		t.Fatalf("revisar = %d %s: %s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	body = n.pedir(http.MethodGet, "/carrito/revisar", nil, false).Body.String()
	debeContener(t, "revisar", body, "REVISÁ TU PEDIDO", "Delivery · Av. Colón 1234", "Transferencia", "Tocar timbre 2B", "ahí también vas a ver el alias", "CONFIRMAR PEDIDO", "$24.000")

	// Confirmar: el pedido entra pendiente, el carrito se vacía y va al seguimiento.
	rec = n.pedir(http.MethodPost, "/pedido", url.Values{}, false)
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/pedido/") {
		t.Fatalf("confirmar = %d %s", rec.Code, rec.Header().Get("Location"))
	}
	if _, hay := n.cookies[cookieCarrito]; hay {
		t.Error("después de confirmar el carrito tendría que quedar vacío")
	}
	p, err := tt.q.GetPedidoPorToken(context.Background(), strings.TrimPrefix(rec.Header().Get("Location"), "/pedido/"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Estado != pedidos.EstadoPendiente || p.Origen != pedidos.OrigenWeb || p.Total != "24000.00" || p.Telefono != "2235551234" || p.Notas != "Tocar timbre 2B" {
		t.Errorf("pedido guardado: %+v", p)
	}
	items, _ := tt.q.ListItemsPedido(context.Background(), p.ID)
	if len(items) != 2 || items[0].Aclaraciones != "sin cebolla" {
		t.Errorf("ítems: %+v", items)
	}

	body = n.pedir(http.MethodGet, rec.Header().Get("Location"), nil, false).Body.String()
	debeContener(t, "seguimiento", body, "Recibimos tu pedido", "Esta página se actualiza sola", "every 15s",
		"PAGO POR TRANSFERENCIA", "forest.burger.mp", "Comprobante del pedido", "Classic · Simple", "Delivery · Av. Colón 1234 · Tocar timbre 2B")
	// En el panel, el aviso de pedido nuevo.
	mux := conSesion(n.mux, adminTest)
	guardarAdminTest(t, tt.q)
	debeContener(t, "panel", pedirPanel(mux, "/admin/pedidos", false).Body.String(), "Nuevo pedido #", "Ulises", "1x Classic + cheddar · “sin cebolla”")
}

func TestRevisar_Validaciones(t *testing.T) {
	tt := setupCliente(t)
	n := tt.nav
	n.pedir(http.MethodPost, "/carrito/rapido", url.Values{"producto": {idTexto(tt.gaseosa.ID)}, "paso": {"1"}}, true)
	datos := url.Values{"nombre": {"Ulises"}, "telefono": {"2235551234"}, "entrega": {"delivery"}, "pago": {"efectivo"}}
	debeContener(t, "delivery sin dirección", n.pedir(http.MethodPost, "/carrito/revisar", datos, false).Body.String(), "Falta la dirección")

	datos.Set("entrega", "retiro")
	if err := tt.q.UpdateAjustes(context.Background(), sqlc.UpdateAjustesParams{DeliveryActivo: true, CostoEnvio: "1500", PedidoMinimo: sql.NullString{String: "10000", Valid: true}}); err != nil {
		t.Fatal(err)
	}
	debeContener(t, "mínimo", n.pedir(http.MethodPost, "/carrito/revisar", datos, false).Body.String(), "El pedido mínimo es de $10.000")

	tt.q.SetLocalAbierto(context.Background(), false)
	debeContener(t, "cerrado", n.pedir(http.MethodPost, "/carrito/revisar", datos, false).Body.String(), "Ahora estamos cerrados")
	// Y no se puede saltear: confirmar con el local cerrado vuelve al carrito.
	if rec := n.pedir(http.MethodPost, "/pedido", url.Values{}, false); rec.Header().Get("Location") != "/carrito" {
		t.Errorf("confirmar cerrado tendría que volver al carrito, fue a %q", rec.Header().Get("Location"))
	}
}

func TestCarrito_ProductoAgotadoSeSaca(t *testing.T) {
	tt := setupCliente(t)
	tt.nav.pedir(http.MethodPost, "/carrito", url.Values{"producto": {idTexto(tt.classic.ID)}, "tamano": {"doble"}, "cantidad": {"1"}}, false)
	tt.q.UpdateDisponibleProducto(context.Background(), sqlc.UpdateDisponibleProductoParams{ID: tt.classic.ID, Disponible: false})
	body := tt.nav.pedir(http.MethodGet, "/carrito", nil, false).Body.String()
	debeContener(t, "carrito con agotado", body, "Sacamos algo de tu pedido: Classic está agotado.", "Todavía no agregaste nada")
}

func TestSeguimiento_Estados(t *testing.T) {
	tt := setupCliente(t)
	ctx := context.Background()
	p, err := pedidos.Crear(ctx, tt.db, pedidos.NuevoPedido{Cliente: "Ulises", Telefono: "2235551234", Origen: pedidos.OrigenWeb, Entrega: pedidos.EntregaDelivery, Direccion: "Colón 1234", Pago: pedidos.PagoEfectivo,
		Items: []pedidos.NuevoItem{{ProductoID: tt.classic.ID, Tamano: pedidos.TamanoDoble, Cantidad: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	ruta := "/pedido/" + p.Token
	if _, err := pedidos.CambiarEstado(ctx, tt.db, p.ID, pedidos.AccionConfirmar, 30); err != nil {
		t.Fatal(err)
	}
	body := tt.nav.pedir(http.MethodGet, ruta, nil, false).Body.String()
	debeContener(t, "confirmado", body, "¡Confirmado! Lo estamos preparando", "Llega aprox. a las", "Confirmado, en preparación", "En camino", "Classic · Doble")
	if strings.Contains(body, "PAGO POR TRANSFERENCIA") {
		t.Error("en efectivo no va el alias")
	}
	// La parte que se pide sola cada 15 s.
	debeContener(t, "estado", tt.nav.pedir(http.MethodGet, ruta+"/estado", nil, true).Body.String(), `id="seguimiento-estado"`, "¡Confirmado!")

	if _, err := pedidos.CambiarEstado(ctx, tt.db, p.ID, pedidos.AccionCancelar, 0); err != nil {
		t.Fatal(err)
	}
	body = tt.nav.pedir(http.MethodGet, ruta, nil, false).Body.String()
	debeContener(t, "cancelado", body, "Se canceló tu pedido")
	if strings.Contains(body, "every 15s") {
		t.Error("un pedido cancelado ya no se actualiza")
	}

	rec := tt.nav.pedir(http.MethodGet, "/pedido/noexiste", nil, false)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "NO ENCONTRAMOS ESE PEDIDO") {
		t.Errorf("token inexistente: %d", rec.Code)
	}
}

func TestPromo_ArmarYPedir(t *testing.T) {
	tt := setupCliente(t)
	ctx := context.Background()
	n := tt.nav
	pr, err := tt.q.CreatePromo(ctx, sqlc.CreatePromoParams{Nombre: "2 a elección", Precio: "29000", Cantidad: 2, Tamano: "cliente", Dias: []int32{0, 1, 2, 3, 4, 5, 6}, Activa: true})
	if err != nil {
		t.Fatal(err)
	}
	tt.q.AddProductoAPromo(ctx, sqlc.AddProductoAPromoParams{PromoID: pr.ID, ProductoID: tt.classic.ID})
	tt.q.AddProductoAPromo(ctx, sqlc.AddProductoAPromoParams{PromoID: pr.ID, ProductoID: tt.bacon.ID}) // agotado
	id, classic, ched := idTexto(pr.ID), idTexto(tt.classic.ID), idTexto(tt.cheddar.ID)

	debeContener(t, "menú", n.pedir(http.MethodGet, "/menu", nil, false).Body.String(), `href="/promo/`+id+`"`, "ARMAR PROMO")
	body := n.pedir(http.MethodGet, "/promo/"+id, nil, false).Body.String()
	debeContener(t, "promo vacía", body, "2 a elección", "Elegí 2 hamburguesas, simples o dobles: el precio es el mismo. Podés repetir la misma.", "Te falta 2", "0 de 2", "AGOTADO", "Elegí 2 más para agregarla")

	// Sumar dos Classic, la primera doble y con cheddar.
	armar := func(f url.Values) string {
		f.Set("promo", id)
		return n.pedir(http.MethodPost, "/promo/"+id+"/armar", f, true).Body.String()
	}
	armar(url.Values{"accion": {"mas"}, "producto": {classic}})
	body = armar(url.Values{"accion": {"mas"}, "producto": {classic}, "slot_0_producto": {classic}, "slot_0_tamano": {"simple"}})
	debeContener(t, "dos elegidas", body, "¡Listo! Ya elegiste todas", "2 de 2", "1 · Classic (simple)", "2 · Classic (simple)")
	dos := url.Values{"slot_0_producto": {classic}, "slot_0_tamano": {"simple"}, "slot_1_producto": {classic}, "slot_1_tamano": {"simple"}}
	dos.Set("accion", "tamano")
	dos.Set("i", "0")
	dos.Set("t", "doble")
	debeContener(t, "tamaño", armar(dos), "1 · Classic (doble)")
	dos.Set("slot_0_tamano", "doble")
	dos.Set("accion", "extra")
	dos.Set("extra", ched)
	debeContener(t, "extra", armar(dos), `name="slot_0_extras" value="`+ched+`"`, "$30.000", "Promo $29.000 + extras $1.000")

	// Agregar: va al carrito como un renglón.
	f := url.Values{"promo": {id}, "slot_0_producto": {classic}, "slot_0_tamano": {"doble"}, "slot_0_extras": {ched}, "slot_1_producto": {classic}, "slot_1_tamano": {"simple"}}
	if rec := n.pedir(http.MethodPost, "/carrito/promo", f, false); rec.Header().Get("Location") != "/menu" {
		t.Fatalf("agregar promo = %d", rec.Code)
	}
	debeContener(t, "carrito", n.pedir(http.MethodGet, "/carrito", nil, false).Body.String(), "2 a elección · PROMO", "· Classic (doble) + Cheddar", "· Classic (simple)", "$30.000")

	// Pedirla y verla en el panel y en el seguimiento.
	n.pedir(http.MethodPost, "/carrito/revisar", url.Values{"nombre": {"Ulises"}, "telefono": {"2235551234"}, "entrega": {"retiro"}, "pago": {"efectivo"}}, false)
	rec := n.pedir(http.MethodPost, "/pedido", url.Values{}, false)
	debeContener(t, "seguimiento", n.pedir(http.MethodGet, rec.Header().Get("Location"), nil, false).Body.String(), "1x 2 a elección · PROMO", "Classic (doble) + Cheddar · Classic (simple)", "$30.000")
	guardarAdminTest(t, tt.q)
	debeContener(t, "panel", pedirPanel(conSesion(n.mux, adminTest), "/admin/pedidos", false).Body.String(), "1x 2 a elección (promo): Classic (doble) + Cheddar · Classic (simple)")
}
