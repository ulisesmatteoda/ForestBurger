package views

import (
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func render(t *testing.T, c templ.Component) string {
	t.Helper()
	var b strings.Builder
	if err := c.Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestSwitch(t *testing.T) {
	prendido := render(t, Switch("disponible", true, "Disponible", templ.Attributes{"hx-put": "/x"}))
	for _, esperado := range []string{`type="checkbox"`, `role="switch"`, `name="disponible"`, `value="1"`, ` checked`, `aria-label="Disponible"`, `hx-put="/x"`} {
		if !strings.Contains(prendido, esperado) {
			t.Errorf("el switch prendido no tiene %q: %s", esperado, prendido)
		}
	}
	if apagado := render(t, Switch("disponible", false, "Disponible", nil)); strings.Contains(apagado, "checked") {
		t.Errorf("el switch apagado no tiene que venir marcado: %s", apagado)
	}
}

func TestBarraLateral_SeccionesYActiva(t *testing.T) {
	html := render(t, Layout("Productos", SeccionProductos))
	if !strings.Contains(html, `href="/admin" class="nav-item activo" aria-current="page">Productos`) {
		t.Errorf("Productos tendría que estar activa: %s", html)
	}
	for _, ruta := range []string{"/admin/extras", "/admin/pedidos", "/admin/ajustes"} {
		if !strings.Contains(html, `href="`+ruta+`" class="nav-item">`) {
			t.Errorf("falta el link a %s, sin resaltar", ruta)
		}
	}
	if strings.Contains(html, "Pronto") {
		t.Error("ya no queda ninguna sección por venir")
	}
	if !strings.Contains(html, `src="/static/panel.js?v=`) {
		t.Error("el panel tiene que cargar panel.js (COPIAR)")
	}
}

func TestMetricas_AltoDeLasBarras(t *testing.T) {
	html := render(t, Metricas(MetricasPedidos{
		HoyTotal: "412500.00", SemanaTotal: "1774000.00", Ticket: "22917.00",
		Barras: []BarraVenta{{Dia: "Lun", Valor: "198000.00", Alto: 48}, {Dia: "Sáb (hoy)", Valor: "412500.00", Alto: 100, EsHoy: true}, {Dia: "Dom"}},
	}, false))
	for _, esperado := range []string{`style="--alto: 48;"`, `style="--alto: 100;"`, `class="barra-dia hoy"`, "$412.500", "$22.917", "Sáb (hoy)"} {
		if !strings.Contains(html, esperado) {
			t.Errorf("el gráfico no contiene %q: %s", esperado, html)
		}
	}
}
