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
	for _, s := range []string{"Extras", "Pedidos", "Ajustes"} {
		if !strings.Contains(html, s) {
			t.Errorf("falta la sección %s", s)
		}
	}
	if !strings.Contains(html, `href="/admin/extras" class="nav-item">Extras`) {
		t.Error("Extras tendría que ser un link, sin resaltar")
	}
	// Las que no existen todavía no son links (darían 404).
	if strings.Contains(html, `href="/admin/pedidos"`) {
		t.Error("Pedidos todavía no existe: no tendría que ser un link")
	}
}
