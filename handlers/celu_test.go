package handlers

import (
	"strings"
	"testing"
	"time"

	"forestburger/pedidos"
)

// En el celu la barra lateral se oculta (CSS) y quedan la barra de arriba y las pestañas: tienen
// que venir en todas las páginas del panel, con la sección actual marcada.
func TestCelu_BarrasDeArribaYPestanas(t *testing.T) {
	mux, _ := setup(t)
	for ruta, activa := range map[string]string{"/admin": "Productos", "/admin/pedidos": "Pedidos", "/admin/promos": "Promos", "/admin/extras": "Extras", "/admin/ajustes": "Ajustes"} {
		body := pedirPanel(mux, ruta, false).Body.String()
		for _, esperado := range []string{
			`<header class="topbar">`, `src="/static/img/pinos.svg"`, `<nav class="tabbar"`,
			`class="tab activo" aria-current="page"`, "<span>" + activa + "</span>",
		} {
			if !strings.Contains(body, esperado) {
				t.Errorf("%s: falta %q", ruta, esperado)
			}
		}
		// Las 5 pestañas, en el orden del Figma.
		orden := []string{"<span>Pedidos</span>", "<span>Productos</span>", "<span>Promos</span>", "<span>Extras</span>", "<span>Ajustes</span>"}
		ultimo := -1
		for _, o := range orden {
			i := strings.Index(body[strings.Index(body, `<nav class="tabbar"`):], o)
			if i < ultimo {
				t.Errorf("%s: las pestañas no van en el orden del Figma", ruta)
			}
			ultimo = i
		}
	}
	// "Salir" en el celu va al final de Ajustes.
	if body := pedirPanel(mux, "/admin/ajustes", false).Body.String(); !strings.Contains(body, `class="sesion-celu solo-celu"`) {
		t.Error("falta la sesión y Salir al final de Ajustes")
	}
}

func TestPedidos_ChipPendientesConCantidad(t *testing.T) {
	mux, _, db, prod, ext := conPedidos(t)
	nuevoPedido(t, db, prod, ext, "Martina", pedidos.EntregaRetiro, time.Now())
	nuevoPedido(t, db, prod, ext, "Tomás", pedidos.EntregaRetiro, time.Now())
	if body := pedirPanel(mux, "/admin/pedidos", false).Body.String(); !strings.Contains(body, ">Pendientes (2)</a>") {
		t.Error("el chip tendría que decir Pendientes (2)")
	}
	// Con htmx (un filtro), también.
	if body := pedirPanel(mux, "/admin/pedidos?estado=pendiente", true).Body.String(); !strings.Contains(body, ">Pendientes (2)</a>") {
		t.Error("con htmx el chip tendría que seguir diciendo Pendientes (2)")
	}
}
