package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"forestburger/pedidos"
)

// pedirBoost pide como un link con hx-boost: htmx manda HX-Request y HX-Boosted.
func pedirBoost(h http.Handler, ruta string) string {
	req := httptest.NewRequest(http.MethodGet, ruta, nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Boosted", "true")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Body.String()
}

// Un link boosteado (el sidebar, las pestañas, VER) espera la página entera: htmx reemplaza el
// body. Un pedido de htmx común (un filtro) sigue recibiendo solo el pedazo.
func TestBoost_DevuelveLaPaginaEntera(t *testing.T) {
	mux, _, db, prod, ext := conPedidos(t)
	p := nuevoPedido(t, db, prod, ext, "Martina", pedidos.EntregaRetiro, time.Now())
	for _, ruta := range []string{"/admin", "/admin/pedidos", "/admin/pedidos/" + idTexto(p.ID), "/admin/promos"} {
		if body := pedirBoost(mux, ruta); !strings.Contains(body, "<html") || !strings.Contains(body, `class="sidebar-nav"`) {
			t.Errorf("%s con boost tendría que devolver la página entera", ruta)
		}
	}
	// El VER del aviso con boost: la página de Pedidos con el detalle abierto, y ya visto.
	if body := pedirBoost(mux, "/admin/pedidos/"+idTexto(p.ID)); !strings.Contains(body, "PEDIDO #"+idTexto(p.ID)) || strings.Contains(body, "Nuevo pedido #") {
		t.Error("VER con boost tendría que abrir el detalle y sacar el aviso")
	}
	if body := pedirPanel(mux, "/admin/pedidos?rango=ayer", true).Body.String(); strings.Contains(body, "<html") {
		t.Error("un filtro con htmx (sin boost) sigue devolviendo solo el panel")
	}
	// La web del cliente también.
	if body := pedirBoost(mux, "/menu"); !strings.Contains(body, "<html") || !strings.Contains(body, `class="cli" hx-boost="true"`) {
		t.Error("el menú con boost tendría que devolver la página entera")
	}
	_ = context.Background()
}

// Dónde va el boost: la navegación sí; los forms que mandan algo, no; panel.js en el <head>
// (si estuviera en el body, cada navegación lo volvería a cargar).
func TestBoost_DondeVa(t *testing.T) {
	tt := setupCliente(t)
	guardarAdminTest(t, tt.q)
	mux := conSesion(tt.nav.mux, adminTest)
	panel := pedirPanel(mux, "/admin", false).Body.String()
	head := panel[:strings.Index(panel, "</head>")]
	for _, esperado := range []string{`class="sidebar-nav" aria-label="Secciones del panel" hx-boost="true"`, `class="tabbar" aria-label="Secciones del panel" hx-boost="true"`, `class="topbar-marca" hx-boost="true"`} {
		if !strings.Contains(panel, esperado) {
			t.Errorf("panel: falta %q", esperado)
		}
	}
	if !strings.Contains(head, "/static/panel.js") || !strings.Contains(head, `name="htmx-config"`) || strings.Count(panel, "/static/panel.js") != 1 {
		t.Error("panel.js y htmx-config tienen que ir una sola vez, en el <head>")
	}
	burger := tt.nav.pedir(http.MethodGet, "/menu/"+idTexto(tt.classic.ID), nil, false).Body.String()
	if !strings.Contains(burger, `<form id="form-burger" class="cli-burger" method="post" action="/carrito" hx-boost="false">`) || !strings.Contains(burger, `aria-label="Volver" hx-boost="true"`) {
		t.Error("el form de la burger no se boostea; el volver sí")
	}
	carrito := tt.nav.pedir(http.MethodGet, "/carrito", nil, false).Body.String()
	_ = carrito // vacío: el form igual tiene que tener hx-boost="false"
	if !strings.Contains(carrito, `action="/carrito/revisar" hx-boost="false"`) {
		t.Error("el form del carrito no se boostea")
	}
}
