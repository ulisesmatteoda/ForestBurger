package handlers

import (
	"context"
	"html"
	"net/http"
	"net/url"
	"strings"
	"testing"

	sqlc "forestburger/db/sqlc"
	"forestburger/pedidos"
)

// armar manda el form del modal con una acción, como un botón del carrito.
func armar(mux http.Handler, form url.Values) string {
	return pedirExtra(mux, http.MethodPost, "/admin/pedidos/cargar/armar", form, true).Body.String()
}

// formCarga es el form con lo básico del cliente (lo que ya está en el modal).
func formCarga(extra url.Values) url.Values {
	f := url.Values{"categoria": {"hamburguesa"}, "cliente": {"Martina"}, "origen": {"whatsapp"}, "entrega": {"retiro"}, "pago": {"efectivo"}}
	for k, v := range extra {
		f[k] = v
	}
	return f
}

func TestCargarPedido_ModalVacio(t *testing.T) {
	mux, _, _, prod, _ := conPedidos(t)

	body := pedirPanel(mux, "/admin/pedidos/cargar", true).Body.String()
	for _, esperado := range []string{
		"CARGAR PEDIDO", "Hamburguesas", "Cheese Bacon",
		html.EscapeString(`{"accion":"agregar","producto":"` + idTexto(prod.ID) + `","tamano":"doble"}`),
		"Todavía no hay productos", "disabled", // GUARDAR sin productos
		`value="whatsapp" checked`, `value="retiro" checked`, `value="efectivo" checked`,
	} {
		if !strings.Contains(body, esperado) {
			t.Errorf("el modal vacío no contiene %q", esperado)
		}
	}
	// La página de Pedidos tiene el botón para abrirlo.
	if !strings.Contains(pedirPanel(mux, "/admin/pedidos", false).Body.String(), `hx-get="/admin/pedidos/cargar"`) {
		t.Error("falta el botón + CARGAR PEDIDO")
	}
}

func TestArmarPedido_AgregarYCantidades(t *testing.T) {
	mux, _, _, prod, _ := conPedidos(t)
	id := idTexto(prod.ID)

	body := armar(mux, formCarga(url.Values{"accion": {"agregar"}, "producto": {id}, "tamano": {"doble"}}))
	for _, esperado := range []string{`name="item_0_producto" value="` + id + `"`, `name="item_0_cantidad" value="1"`, "Cheese Bacon (doble)", "$17.000 c/u", `value="Martina"`} {
		if !strings.Contains(body, esperado) {
			t.Errorf("después de agregar falta %q", esperado)
		}
	}

	// El mismo tamaño otra vez suma uno al renglón; otro tamaño es un renglón nuevo.
	con1 := formCarga(url.Values{"item_0_producto": {id}, "item_0_tamano": {"doble"}, "item_0_cantidad": {"1"}})
	con1.Set("accion", "agregar")
	con1.Set("producto", id)
	con1.Set("tamano", "doble")
	if body := armar(mux, con1); !strings.Contains(body, `name="item_0_cantidad" value="2"`) || strings.Contains(body, "item_1_producto") {
		t.Errorf("el mismo tamaño tendría que sumar uno: %s", body)
	}
	con1.Set("tamano", "simple")
	if body := armar(mux, con1); !strings.Contains(body, `name="item_1_tamano" value="simple"`) {
		t.Error("otro tamaño tendría que ser un renglón nuevo")
	}

	// "−" con 1 saca el renglón.
	menos := formCarga(url.Values{"item_0_producto": {id}, "item_0_tamano": {"doble"}, "item_0_cantidad": {"1"}, "accion": {"menos"}, "i": {"0"}})
	if body := armar(mux, menos); strings.Contains(body, "item_0_producto") || !strings.Contains(body, "Todavía no hay productos") {
		t.Error("el − en 1 tendría que sacar el renglón")
	}
	mas := formCarga(url.Values{"item_0_producto": {id}, "item_0_tamano": {"doble"}, "item_0_cantidad": {"1"}, "accion": {"mas"}, "i": {"0"}})
	if body := armar(mux, mas); !strings.Contains(body, `name="item_0_cantidad" value="2"`) || !strings.Contains(body, "$34.000") {
		t.Error("el + tendría que subir la cantidad y el total")
	}
}

func TestArmarPedido_ExtrasEnvioYDescuento(t *testing.T) {
	mux, _, _, prod, ext := conPedidos(t)
	id, eid := idTexto(prod.ID), idTexto(ext.ID)
	base := url.Values{"item_0_producto": {id}, "item_0_tamano": {"doble"}, "item_0_cantidad": {"1"}}

	// Prender el cheddar: $17.000 + $1.000 por unidad.
	f := formCarga(base)
	f.Set("accion", "extra")
	f.Set("i", "0")
	f.Set("extra", eid)
	body := armar(mux, f)
	if !strings.Contains(body, `name="item_0_extras" value="`+eid+`"`) || !strings.Contains(body, "+ cheddar · $18.000 c/u") {
		t.Errorf("el extra tendría que sumarse por unidad: %s", body)
	}
	// Apagarlo.
	f.Set("item_0_extras", eid)
	if body := armar(mux, f); !strings.Contains(body, `name="item_0_extras" value=""`) {
		t.Error("tocar el extra prendido lo tendría que apagar")
	}

	// Delivery: aparece la dirección y suma el envío.
	d := formCarga(base)
	d.Set("entrega", "delivery")
	body = armar(mux, d)
	if !strings.Contains(body, `name="direccion"`) || !strings.Contains(body, "Envío") || !strings.Contains(body, "$18.500") {
		t.Errorf("delivery: falta la dirección o el envío (17.000 + 1.500): %s", body)
	}

	// Descuento 20% y $2.000.
	p := formCarga(base)
	p.Set("desc_tipo", "pct")
	p.Set("desc_valor", "20")
	p.Set("motivo", "amigos")
	if body := armar(mux, p); !strings.Contains(body, "Descuento 20% (amigos)") || !strings.Contains(body, "$13.600") {
		t.Errorf("descuento 20%%: 17.000 - 3.400 = 13.600: %s", body)
	}
	p.Set("desc_tipo", "fijo")
	p.Set("desc_valor", "2000")
	if body := armar(mux, p); !strings.Contains(body, "Descuento (amigos)") || !strings.Contains(body, "$15.000") {
		t.Errorf("descuento de $2.000: 17.000 - 2.000 = 15.000: %s", body)
	}
}

func TestGuardarPedido(t *testing.T) {
	mux, q, _, prod, ext := conPedidos(t)
	gaseosa, err := q.CreateProducto(context.Background(), sqlc.CreateProductoParams{Nombre: "Gaseosa lata", Categoria: "bebida", Precio: "3500"})
	if err != nil {
		t.Fatal(err)
	}
	// Cheese Bacon doble + cheddar y 2 gaseosas, delivery con 20% "amigos":
	// (17.000 + 1.000) + 2 × 3.500 = 25.000; -5.000; +1.500 de envío = 21.500.
	f := formCarga(url.Values{
		"telefono": {"223 555 1234"}, "entrega": {"delivery"}, "direccion": {"Av. Colón 1234"},
		"desc_tipo": {"pct"}, "desc_valor": {"20"}, "motivo": {"amigos"},
		"item_0_producto": {idTexto(prod.ID)}, "item_0_tamano": {"doble"}, "item_0_cantidad": {"1"}, "item_0_extras": {idTexto(ext.ID)},
		"item_1_producto": {idTexto(gaseosa.ID)}, "item_1_tamano": {"simple"}, "item_1_cantidad": {"2"},
	})
	rec := pedirExtra(mux, http.MethodPost, "/admin/pedidos/cargar", f, true)
	body := rec.Body.String()
	for _, esperado := range []string{
		"PEDIDO #1 CARGADO", "/pedido/", "data-copiar=", `href="https://wa.me/5492235551234?text=`,
		`id="panel-pedidos" class="panel-pedidos" hx-swap-oob="true"`, `id="metricas" class="metricas-y-grafico" hx-swap-oob="true"`,
	} {
		if !strings.Contains(body, esperado) {
			t.Errorf("después de guardar falta %q", esperado)
		}
	}
	p, err := q.GetPedido(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if p.Estado != pedidos.EstadoConfirmado || p.Total != "21500.00" || p.Origen != pedidos.OrigenWhatsapp || p.Telefono != "2235551234" || p.DescuentoMotivo != "amigos" {
		t.Errorf("pedido guardado incorrecto: %+v", p)
	}
	if h := historialDe(t, q, p.ID); len(h) != 2 || h[0] != "recibido" || h[1] != "confirmado" {
		t.Errorf("historial = %v, quiero recibido y confirmado", h)
	}
}

func TestGuardarPedido_Errores(t *testing.T) {
	casos := map[string]struct {
		cambiar func(f url.Values, prodID string)
		mensaje string
	}{
		"sin productos":          {func(f url.Values, _ string) {}, "El pedido no tiene productos"},
		"sin nombre":             {func(f url.Values, id string) { f.Set("cliente", " "); conItem(f, id) }, "Falta el nombre del cliente"},
		"delivery sin dirección": {func(f url.Values, id string) { f.Set("entrega", "delivery"); conItem(f, id) }, "Falta la dirección para el delivery"},
		"descuento raro":         {func(f url.Values, id string) { f.Set("desc_tipo", "pct"); f.Set("desc_valor", "150"); conItem(f, id) }, "El descuento tiene que ser de 0 a 100%"},
	}
	for nombre, c := range casos {
		t.Run(nombre, func(t *testing.T) {
			mux, q, _, prod, _ := conPedidos(t)
			f := formCarga(nil)
			c.cambiar(f, idTexto(prod.ID))
			body := pedirExtra(mux, http.MethodPost, "/admin/pedidos/cargar", f, true).Body.String()
			if !strings.Contains(body, c.mensaje) || !strings.Contains(body, "CARGAR PEDIDO") {
				t.Errorf("tendría que volver el modal con %q: %s", c.mensaje, body)
			}
			if _, err := q.GetPedido(context.Background(), 1); err == nil {
				t.Error("con un error no tendría que guardar nada")
			}
		})
	}
}

func TestCargarPedido_PideSesion(t *testing.T) {
	mux, _ := setupSinSesion(t)
	if rec := pedirExtra(mux, http.MethodPost, "/admin/pedidos/cargar", formCarga(nil), true); rec.Header().Get("HX-Redirect") != "/login" {
		t.Error("guardar sin sesión tendría que mandar al login")
	}
}

func conItem(f url.Values, id string) {
	f.Set("item_0_producto", id)
	f.Set("item_0_tamano", "simple")
	f.Set("item_0_cantidad", "1")
}

func historialDe(t *testing.T, q *sqlc.Queries, id int32) []string {
	t.Helper()
	eventos, err := q.ListEventosPedido(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	var h []string
	for _, e := range eventos {
		h = append(h, e.Estado)
	}
	return h
}

func TestCargarPedido_AgotadoNoSePuedeAgregar(t *testing.T) {
	mux, q, _, prod, _ := conPedidos(t)
	if _, err := q.UpdateDisponibleProducto(context.Background(), sqlc.UpdateDisponibleProductoParams{ID: prod.ID, Disponible: false}); err != nil {
		t.Fatal(err)
	}
	body := pedirPanel(mux, "/admin/pedidos/cargar", true).Body.String()
	if !strings.Contains(body, `class="carga-producto apagado"`) || !strings.Contains(body, `<span class="tag">Agotado</span>`) || strings.Contains(body, html.EscapeString(`"accion":"agregar"`)) {
		t.Errorf("un agotado tendría que verse sin botones para agregar: %s", body)
	}
	// Y si igual llega (un form viejo), guardar lo rechaza.
	f := formCarga(nil)
	conItem(f, idTexto(prod.ID))
	if body := pedirExtra(mux, http.MethodPost, "/admin/pedidos/cargar", f, true).Body.String(); !strings.Contains(body, "Cheese Bacon está agotado") {
		t.Errorf("guardar con un agotado tendría que avisar: %s", body)
	}
}
