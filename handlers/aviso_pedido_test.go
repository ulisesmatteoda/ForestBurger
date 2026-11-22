package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"forestburger/pedidos"
)

func TestAviso_SinPendientesQuedaVacioYSigueConsultando(t *testing.T) {
	mux, _, _, _, _ := conPedidos(t)
	body := pedirPanel(mux, "/admin", false).Body.String()
	for _, esperado := range []string{`id="aviso-pedido"`, `hx-trigger="every 10s"`, `actual=&amp;titulo=Productos`, "<title>Productos · ForestBurger</title>"} {
		if !strings.Contains(body, esperado) {
			t.Errorf("la página no tiene %q", esperado)
		}
	}
	if strings.Contains(body, "Nuevo pedido") || strings.Contains(body, "<audio") {
		t.Error("sin pendientes no tendría que avisar")
	}
}

func TestAviso_PendienteSinVer(t *testing.T) {
	mux, _, db, prod, ext := conPedidos(t)
	ahora := time.Now()
	p1 := nuevoPedido(t, db, prod, ext, "Martina", pedidos.EntregaRetiro, ahora.Add(-3*time.Minute))
	nuevoPedido(t, db, prod, ext, "Tomás", pedidos.EntregaRetiro, ahora.Add(-2*time.Minute))
	nuevoPedido(t, db, prod, ext, "Lucía", pedidos.EntregaRetiro, ahora.Add(-1*time.Minute))

	// En cualquier pantalla del panel (acá Productos), ya en la primera carga.
	body := pedirPanel(mux, "/admin", false).Body.String()
	for _, esperado := range []string{
		fmt.Sprintf("Nuevo pedido #%d · Martina · $36.000", p1.ID), "y 2 más",
		fmt.Sprintf(`href="/admin/pedidos/%d"`, p1.ID), `<audio src="/static/alarma.wav" autoplay loop>`,
		fmt.Sprintf("actual=%d-3", p1.ID), "<title>(3) Pedido nuevo · ForestBurger</title>",
	} {
		if !strings.Contains(body, esperado) {
			t.Errorf("el aviso no tiene %q", esperado)
		}
	}

	// La consulta con lo mismo que se ve: 204, htmx no toca nada (la alarma no se corta).
	rec := pedirPanel(mux, fmt.Sprintf("/admin/pedidos/aviso?actual=%d-3&titulo=Productos", p1.ID), true)
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Errorf("sin cambios: status %d, cuerpo %q; quiero 204 vacío", rec.Code, rec.Body.String())
	}
	// Desde un panel que todavía no lo mostraba: el aviso con el título de la pestaña.
	rec = pedirPanel(mux, "/admin/pedidos/aviso?actual=&titulo=Productos", true)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Nuevo pedido") || !strings.Contains(rec.Body.String(), "<title>(3) Pedido nuevo · ForestBurger</title>") {
		t.Errorf("con un pedido nuevo tendría que devolver el aviso: %d %s", rec.Code, rec.Body.String())
	}
}

func TestAviso_AlAbrirloSeVa(t *testing.T) {
	mux, _, db, prod, ext := conPedidos(t)
	p := nuevoPedido(t, db, prod, ext, "Martina", pedidos.EntregaRetiro, time.Now())
	actual := fmt.Sprintf("%d-1", p.ID)

	// Abrir el detalle (con htmx): queda visto y el aviso se vacía por OOB en esa misma pantalla.
	body := pedirPanel(mux, fmt.Sprintf("/admin/pedidos/%d", p.ID), true).Body.String()
	if !strings.Contains(body, `id="aviso-pedido" hx-get="/admin/pedidos/aviso?actual=&amp;titulo=Pedidos" hx-trigger="every 10s" hx-swap="outerHTML" hx-swap-oob="true"`) || strings.Contains(body, "<audio") {
		t.Errorf("abrir el detalle tendría que vaciar el aviso por OOB: %s", body)
	}
	// Las otras pantallas abiertas se enteran en la próxima consulta: el aviso vacío y el título normal.
	rec := pedirPanel(mux, "/admin/pedidos/aviso?actual="+actual+"&titulo=Extras", true)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "Nuevo pedido") || !strings.Contains(rec.Body.String(), "<title>Extras · ForestBurger</title>") {
		t.Errorf("después de abrirlo el aviso tendría que irse: %d %s", rec.Code, rec.Body.String())
	}
}

func TestAviso_SinHTMXLaPaginaDelDetalleYaNoAvisa(t *testing.T) {
	mux, _, db, prod, ext := conPedidos(t)
	p := nuevoPedido(t, db, prod, ext, "Martina", pedidos.EntregaRetiro, time.Now())
	// VER es un link común: la página de Pedidos con el detalle, ya sin el aviso.
	body := pedirPanel(mux, fmt.Sprintf("/admin/pedidos/%d", p.ID), false).Body.String()
	if strings.Contains(body, "Nuevo pedido") || !strings.Contains(body, "<title>Pedidos · ForestBurger</title>") {
		t.Error("la página que abre VER no tendría que seguir avisando")
	}
}

func TestAviso_SoloPendientes(t *testing.T) {
	mux, _, db, prod, ext := conPedidos(t)
	confirmado := nuevoPedido(t, db, prod, ext, "Martina", pedidos.EntregaRetiro, time.Now())
	cancelado := nuevoPedido(t, db, prod, ext, "Tomás", pedidos.EntregaRetiro, time.Now())
	ctx := context.Background()
	if _, err := pedidos.CambiarEstado(ctx, db, confirmado.ID, pedidos.AccionConfirmar, 30); err != nil {
		t.Fatal(err)
	}
	if _, err := pedidos.CambiarEstado(ctx, db, cancelado.ID, pedidos.AccionRechazar, 0); err != nil {
		t.Fatal(err)
	}
	if body := pedirPanel(mux, "/admin/pedidos", false).Body.String(); strings.Contains(body, "Nuevo pedido") {
		t.Error("los confirmados y los rechazados sin abrir no tendrían que avisar")
	}
}

func TestAviso_PideSesion(t *testing.T) {
	mux, _ := setupSinSesion(t)
	if rec := pedirPanel(mux, "/admin/pedidos/aviso?actual=", true); rec.Header().Get("HX-Redirect") != "/login" {
		t.Error("el aviso sin sesión tendría que mandar al login")
	}
}
