package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	sqlc "forestburger/db/sqlc"
	"forestburger/pedidos"
)

// conPedidos arma el panel con sesión, un producto y un extra, y devuelve con qué crear pedidos.
func conPedidos(t *testing.T) (http.Handler, *sqlc.Queries, *sql.DB, sqlc.Producto, sqlc.Extra) {
	t.Helper()
	mux, q, api := setupAPI(t)
	guardarAdminTest(t, q)
	prod, err := q.CreateProducto(context.Background(), sqlc.CreateProductoParams{
		Nombre: "Cheese Bacon", Categoria: "hamburguesa", Precio: "15500", PrecioDoble: sql.NullString{String: "17000", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	ext := crearExtra(t, q, "Cheddar", "1000.00", true)
	if err := q.UpdateAjustes(context.Background(), sqlc.UpdateAjustesParams{DeliveryActivo: true, CostoEnvio: "1500"}); err != nil {
		t.Fatal(err)
	}
	return conSesion(mux, adminTest), q, api.db, prod, ext
}

// nuevoPedido crea un pedido de ese cliente en ese momento (2x Cheese Bacon doble + cheddar).
func nuevoPedido(t *testing.T, db *sql.DB, prod sqlc.Producto, ext sqlc.Extra, cliente, entrega string, creado time.Time) sqlc.Pedido {
	t.Helper()
	n := pedidos.NuevoPedido{
		Cliente: cliente, Telefono: "2236001122", Origen: pedidos.OrigenWeb, Entrega: entrega, Pago: pedidos.PagoEfectivo, CreadoEn: creado,
		Items: []pedidos.NuevoItem{{ProductoID: prod.ID, Tamano: pedidos.TamanoDoble, Cantidad: 2, Extras: []pedidos.NuevoExtra{{ExtraID: ext.ID, Cantidad: 1}}}},
	}
	if entrega == pedidos.EntregaDelivery {
		n.Direccion = "Güemes 2850"
	}
	p, err := pedidos.Crear(context.Background(), db, n)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// mediodia es hoy (u otro día, con dias) a las 12, en la hora del local.
func mediodia(dias int) time.Time {
	ahora := time.Now().In(pedidos.Zona)
	return time.Date(ahora.Year(), ahora.Month(), ahora.Day()+dias, 12, 30, 0, 0, pedidos.Zona)
}

func TestPedidos_HoyPorDefecto(t *testing.T) {
	mux, _, db, prod, ext := conPedidos(t)
	nuevoPedido(t, db, prod, ext, "Martina", pedidos.EntregaRetiro, mediodia(0))
	nuevoPedido(t, db, prod, ext, "Juan Pablo", pedidos.EntregaDelivery, mediodia(-1))

	rec := pedirPanel(mux, "/admin/pedidos", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quiero 200", rec.Code)
	}
	body := rec.Body.String()
	for _, esperado := range []string{
		"<h1>PEDIDOS</h1>", "PEDIDOS DE HOY",
		`href="/admin/pedidos" class="nav-item activo"`,
		"Martina", "2x Cheese Bacon (doble) + cheddar", ">12:30<", "Retiro", "Efectivo",
		`class="estado estado-pendiente"`, `hx-get="/admin/pedidos/1"`,
		`class="chip activo" href="/admin/pedidos?rango=hoy"`,
	} {
		if !strings.Contains(body, esperado) {
			t.Errorf("la lista no contiene %q", esperado)
		}
	}
	// Solo la lista: el pedido de ayer sigue pendiente sin ver, así que sí está en el aviso de arriba.
	if lista := body[strings.Index(body, `id="panel-pedidos"`):]; strings.Contains(lista, "Juan Pablo") {
		t.Error("el pedido de ayer no va en la lista de hoy")
	}
}

func TestPedidos_FiltrosDeFechaYEstado(t *testing.T) {
	mux, _, db, prod, ext := conPedidos(t)
	hoy := nuevoPedido(t, db, prod, ext, "Martina", pedidos.EntregaDelivery, mediodia(0))
	nuevoPedido(t, db, prod, ext, "Juan Pablo", pedidos.EntregaDelivery, mediodia(-1))
	ctx := context.Background()
	pedidos.CambiarEstado(ctx, db, hoy.ID, pedidos.AccionConfirmar, 30)
	pedidos.CambiarEstado(ctx, db, hoy.ID, pedidos.AccionEnCamino, 0)

	// Ayer.
	body := pedirPanel(mux, "/admin/pedidos?rango=ayer", true).Body.String()
	if !strings.Contains(body, "PEDIDOS DE AYER") || !strings.Contains(body, "Juan Pablo") || strings.Contains(body, "Martina") || strings.Contains(body, "<html") {
		t.Errorf("filtro ayer (con htmx, solo el panel): %s", body)
	}

	// Un rango de dos días: los dos, y la hora con el día.
	desde, hasta := mediodia(-1).Format("2006-01-02"), mediodia(0).Format("2006-01-02")
	body = pedirPanel(mux, "/admin/pedidos?desde="+desde+"&hasta="+hasta, false).Body.String()
	titulo := "PEDIDOS DEL " + mediodia(-1).Format("02/01") + " AL " + mediodia(0).Format("02/01")
	dia := diasCortos[mediodia(0).Weekday()]
	if !strings.Contains(body, titulo) || !strings.Contains(body, "Martina") || !strings.Contains(body, "Juan Pablo") || !strings.Contains(body, ">"+dia+" 12:30<") {
		t.Errorf("rango de fechas: falta el título %q, algún pedido o la hora con el día", titulo)
	}

	// "Confirmados" incluye los en camino; el chip conserva el rango.
	body = pedirPanel(mux, "/admin/pedidos?rango=hoy&estado=confirmado", true).Body.String()
	if !strings.Contains(body, "Martina") || !strings.Contains(body, `class="estado estado-en_camino"`) || !strings.Contains(body, "En camino") {
		t.Errorf("confirmados tendría que traer el en camino: %s", body)
	}
	if body := pedirPanel(mux, "/admin/pedidos?rango=hoy&estado=pendiente", true).Body.String(); !strings.Contains(body, "No hay pedidos en estas fechas.") {
		t.Errorf("hoy no hay pendientes: %s", body)
	}
}

func TestDetallePedido(t *testing.T) {
	mux, q, db, prod, ext := conPedidos(t)
	p := nuevoPedido(t, db, prod, ext, "Juan Pablo", pedidos.EntregaDelivery, mediodia(0))

	rec := pedirPanel(mux, "/admin/pedidos/"+idTexto(p.ID), true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quiero 200", rec.Code)
	}
	body := rec.Body.String()
	for _, esperado := range []string{
		"PEDIDO #1", "Hoy 12:30 · Vino por la web", "Pendiente",
		"Juan Pablo", "2236001122", "Delivery · Güemes 2850", "Efectivo",
		"2x Cheese Bacon (doble) + cheddar", "$36.000", // 2 × (17.000 + 1.000)
		"Subtotal", "Envío", "$1.500", "$37.500", "Recibido",
		`/pedido/` + p.Token, `data-copiar="http://example.com/pedido/` + p.Token + `"`,
		`href="https://wa.me/5492236001122?text=`,
		"Demora estimada", `name="demora" value="30" checked`, "CONFIRMAR PEDIDO", "RECHAZAR",
	} {
		if !strings.Contains(body, esperado) {
			t.Errorf("el detalle no contiene %q", esperado)
		}
	}
	if visto, _ := q.GetPedido(context.Background(), p.ID); !visto.VistoEn.Valid {
		t.Error("abrir el detalle tendría que marcarlo visto")
	}
	if rec := pedirPanel(mux, "/admin/pedidos/9999", true); rec.Code != http.StatusNotFound {
		t.Errorf("pedido inexistente: status = %d, quiero 404", rec.Code)
	}
	// Sin htmx: la página con el detalle abierto.
	if body := pedirPanel(mux, "/admin/pedidos/"+idTexto(p.ID), false).Body.String(); !strings.Contains(body, "<html") || !strings.Contains(body, "PEDIDO #1") {
		t.Error("sin htmx tendría que devolver la página con el detalle abierto")
	}
}

func TestAccionPedido(t *testing.T) {
	mux, q, db, prod, ext := conPedidos(t)
	p := nuevoPedido(t, db, prod, ext, "Juan Pablo", pedidos.EntregaDelivery, mediodia(0))
	ruta := "/admin/pedidos/" + idTexto(p.ID)

	// Confirmar con 45 minutos: vuelve el detalle confirmado, con la etiqueta de la lista por OOB.
	body := pedirExtra(mux, http.MethodPost, ruta+"/confirmar", url.Values{"demora": {"45"}}, true).Body.String()
	for _, esperado := range []string{"Confirmado · 45 min", "MARCAR EN CAMINO", "CANCELAR PEDIDO",
		`id="estado-pedido-1" class="estado estado-confirmado" hx-swap-oob="true"`} {
		if !strings.Contains(body, esperado) {
			t.Errorf("después de confirmar falta %q", esperado)
		}
	}

	// Confirmar otra vez (como si otra persona ya lo hubiera hecho): aviso y estado real.
	body = pedirExtra(mux, http.MethodPost, ruta+"/confirmar", url.Values{"demora": {"30"}}, true).Body.String()
	if !strings.Contains(body, "Este pedido ya cambió de estado") || !strings.Contains(body, "MARCAR EN CAMINO") {
		t.Errorf("un estado viejo tendría que avisar: %s", body)
	}

	// Cancelar pide confirmación antes.
	body = pedirPanel(mux, ruta+"/baja?accion=cancelar", true).Body.String()
	if !strings.Contains(body, "¿CANCELAR EL PEDIDO #1?") || !strings.Contains(body, `hx-post="`+ruta+`/cancelar"`) || !strings.Contains(body, "VOLVER") {
		t.Errorf("confirmación de cancelar: %s", body)
	}
	pedirExtra(mux, http.MethodPost, ruta+"/cancelar", nil, true)
	if got, _ := q.GetPedido(context.Background(), p.ID); got.Estado != pedidos.EstadoCancelado {
		t.Errorf("estado = %s, quiero cancelado", got.Estado)
	}

	// Sin htmx: vuelve al detalle.
	otro := nuevoPedido(t, db, prod, ext, "Sofía", pedidos.EntregaRetiro, mediodia(0))
	rec := pedirExtra(mux, http.MethodPost, "/admin/pedidos/"+idTexto(otro.ID)+"/confirmar", url.Values{"demora": {"30"}}, false)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/admin/pedidos/"+idTexto(otro.ID) {
		t.Errorf("sin htmx: status = %d, Location = %q", rec.Code, rec.Header().Get("Location"))
	}
	// Un retiro confirmado queda "listo para retirar" (no sale en camino).
	if body := pedirPanel(mux, "/admin/pedidos/"+idTexto(otro.ID), true).Body.String(); !strings.Contains(body, "LISTO PARA RETIRAR") || strings.Contains(body, "MARCAR EN CAMINO") {
		t.Errorf("un retiro confirmado tendría que ofrecer LISTO PARA RETIRAR: %s", body)
	}
	if rec := pedirExtra(mux, http.MethodPost, "/admin/pedidos/"+idTexto(otro.ID)+"/en-camino", nil, true); rec.Code != http.StatusBadRequest {
		t.Errorf("en camino para un retiro: status = %d, quiero 400", rec.Code)
	}
}

func TestPedidos_PideSesion(t *testing.T) {
	mux, _ := setupSinSesion(t)
	if rec := pedirPanel(mux, "/admin/pedidos", false); rec.Code != http.StatusSeeOther {
		t.Errorf("sin sesión: status = %d, quiero 303", rec.Code)
	}
	if rec := pedirExtra(mux, http.MethodPost, "/admin/pedidos/1/confirmar", url.Values{"demora": {"30"}}, true); rec.Header().Get("HX-Redirect") != "/login" {
		t.Error("una acción sin sesión tendría que mandar al login")
	}
}

// Las abreviaturas de los días son texto válido (antes "Sábado"[:3] cortaba la "á" al medio).
func TestMomento_DiasConTilde(t *testing.T) {
	// Un sábado y un miércoles de hace más de una semana, para que no sean "Hoy" ni "Ayer".
	sabado := time.Date(2026, time.November, 7, 21, 5, 0, 0, pedidos.Zona)
	miercoles := time.Date(2026, time.November, 4, 20, 30, 0, 0, pedidos.Zona)
	ahora := time.Date(2026, time.November, 22, 12, 0, 0, 0, pedidos.Zona)
	if got := momento(sabado, ahora); got != "Sáb 07/11 21:05" || !utf8.ValidString(got) {
		t.Errorf("momento del sábado = %q", got)
	}
	if got := momento(miercoles, ahora); got != "Mié 04/11 20:30" {
		t.Errorf("momento del miércoles = %q", got)
	}
	if got := momento(ahora.Add(-time.Hour), ahora); got != "Hoy 11:00" {
		t.Errorf("momento de hoy = %q", got)
	}
}

func TestPedidos_MetricasYGrafico(t *testing.T) {
	mux, _, db, prod, ext := conPedidos(t)
	ctx := context.Background()
	// Cada pedido: 2x Cheese Bacon doble + cheddar, retiro (sin envío) = $36.000.
	crear := func(pago string, creado time.Time) sqlc.Pedido {
		p, err := pedidos.Crear(ctx, db, pedidos.NuevoPedido{
			Cliente: "Martina", Origen: pedidos.OrigenWeb, Entrega: pedidos.EntregaRetiro, Pago: pago, CreadoEn: creado,
			Items: []pedidos.NuevoItem{{ProductoID: prod.ID, Tamano: pedidos.TamanoDoble, Cantidad: 2, Extras: []pedidos.NuevoExtra{{ExtraID: ext.ID, Cantidad: 1}}}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	crear(pedidos.PagoEfectivo, mediodia(0))
	crear(pedidos.PagoTransferencia, mediodia(0))
	cancelado := crear(pedidos.PagoEfectivo, mediodia(0))
	pedidos.CambiarEstado(ctx, db, cancelado.ID, pedidos.AccionRechazar, 0) // no cuenta
	crear(pedidos.PagoEfectivo, mediodia(-1))                               // ayer

	body := pedirPanel(mux, "/admin/pedidos", false).Body.String()
	semana := "$72.000"
	if ayer := mediodia(-1); !ayer.Before(pedidos.LunesDe(mediodia(0))) {
		semana = "$108.000" // ayer es de esta semana (hoy no es lunes)
	}
	for _, esperado := range []string{
		"VENTAS DE HOY", "$72.000", "2 pedidos · +100,0% vs. ayer",
		"Efectivo $36.000 · Transferencia $36.000",
		`<span class="metrica-titulo">PEDIDOS</span> <span class="metrica-valor">2</span> <span class="metrica-detalle">1 cancelado</span>`,
		"TICKET PROMEDIO", "$36.000", "Por pedido, hoy",
		"VENTAS POR DÍA", "no incluye pedidos cancelados", "(hoy)", `style="--alto: 100;"`,
		"Total de la semana", semana, `style="--columnas: 7;"`,
	} {
		if !strings.Contains(body, esperado) {
			t.Errorf("las métricas no contienen %q", esperado)
		}
	}
	if n := strings.Count(body, `<span class="barra-grafico" style=`); n != 7 {
		t.Errorf("el gráfico tiene %d barras, quiero 7", n)
	}

	// Un filtro con htmx trae la lista y, por OOB, las métricas de ese período.
	body = pedirPanel(mux, "/admin/pedidos?rango=ayer", true).Body.String()
	for _, esperado := range []string{
		`id="metricas" class="metricas-y-grafico" hx-swap-oob="true"`, "VENTAS DE AYER", "Por pedido, ayer",
		"1 pedido", "Ninguno cancelado", "$36.000",
	} {
		if !strings.Contains(body, esperado) {
			t.Errorf("con ?rango=ayer las métricas no contienen %q", esperado)
		}
	}

	// Este mes: todos los del mes y una barra por día.
	hoy := mediodia(0)
	mes, cantidad := "$72.000", "2"
	if mediodia(-1).Month() == hoy.Month() {
		mes, cantidad = "$108.000", "3"
	}
	diasDelMes := time.Date(hoy.Year(), hoy.Month()+1, 0, 0, 0, 0, 0, pedidos.Zona).Day()
	body = pedirPanel(mux, "/admin/pedidos?rango=mes", false).Body.String()
	for _, esperado := range []string{
		"VENTAS DE ESTE MES", mes, `<span class="metrica-valor">` + cantidad + `</span>`, "Por pedido, este mes", "Total del mes",
		fmt.Sprintf(`class="barras muchas" style="--columnas: %d;"`, diasDelMes),
	} {
		if !strings.Contains(body, esperado) {
			t.Errorf("con ?rango=mes las métricas no contienen %q", esperado)
		}
	}
}
