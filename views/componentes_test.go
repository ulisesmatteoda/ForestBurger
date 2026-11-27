package views

import (
	"context"
	"fmt"
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
		VentasTitulo: "VENTAS DE HOY", VentasTotal: "412500.00", GraficoTotal: "1774000.00", Ticket: "22917.00",
		Barras: []BarraVenta{{Dia: "Lun", Valor: "198000.00", Alto: 48}, {Dia: "Sáb (hoy)", Valor: "412500.00", Alto: 100, EsHoy: true}, {Dia: "Dom"}},
	}, false))
	for _, esperado := range []string{
		`style="--alto: 48;"`, `style="--alto: 100;"`, `class="barra-dia hoy"`, "$412.500", "$22.917", "Sáb (hoy)",
		`class="barras" style="--columnas: 3;"`, `title="Lun: $198.000"`,
	} {
		if !strings.Contains(html, esperado) {
			t.Errorf("el gráfico no contiene %q: %s", esperado, html)
		}
	}
}

// Con más de 10 barras (un mes) el monto de cada una no entra: la clase muchas lo oculta.
func TestMetricas_MuchasBarras(t *testing.T) {
	barras := make([]BarraVenta, 30)
	for i := range barras {
		barras[i] = BarraVenta{Dia: fmt.Sprint(i + 1), Valor: "1000.00", Alto: 10}
	}
	html := render(t, Metricas(MetricasPedidos{Barras: barras}, false))
	if !strings.Contains(html, `class="barras muchas" style="--columnas: 30;"`) {
		t.Errorf("un mes tendría que ir con 30 columnas y la clase muchas")
	}
}

// El panel se puede instalar como app: el manifest, los íconos y las metas del iPhone van en todas
// las páginas (también en el login, que es donde cae la app si la sesión venció).
func TestDocumento_MetasDeLaPWA(t *testing.T) {
	var b strings.Builder
	if err := Documento("Ingresar").Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	for _, esperado := range []string{
		`<link rel="manifest" href="/static/manifest.json">`, `<meta name="theme-color" content="#023320">`,
		`<link rel="apple-touch-icon" href="/static/icons/apple-touch-icon.png">`, `<meta name="apple-mobile-web-app-capable" content="yes">`,
		"viewport-fit=cover",
	} {
		if !strings.Contains(b.String(), esperado) {
			t.Errorf("falta %q", esperado)
		}
	}
}

// AMBIENTE=staging: la franja y el noindex en el panel y en la web del cliente; sin la variable,
// nada.
func TestStaging_FranjaYNoIndex(t *testing.T) {
	render := func(c templ.Component) string {
		var b strings.Builder
		if err := c.Render(context.Background(), &b); err != nil {
			t.Fatal(err)
		}
		return b.String()
	}
	defer func() { Staging = false }()
	for _, staging := range []bool{true, false} {
		Staging = staging
		for nombre, html := range map[string]string{"panel": render(Documento("Productos")), "cliente": render(DocumentoCliente("Menú"))} {
			franja := strings.Contains(html, "AMBIENTE DE PRUEBA · los pedidos no son reales")
			meta := strings.Contains(html, `<meta name="robots" content="noindex">`)
			if franja != staging || meta != staging {
				t.Errorf("%s con Staging=%v: franja %v, noindex %v", nombre, staging, franja, meta)
			}
		}
	}
}

// En el celu, el gráfico de más de 7 barras (un mes) se esconde: lleva la clase grafico-largo.
func TestMetricas_GraficoLargo(t *testing.T) {
	semana := make([]BarraVenta, 7)
	mes := make([]BarraVenta, 30)
	if html := render(t, Metricas(MetricasPedidos{Barras: semana}, false)); strings.Contains(html, "grafico-largo") {
		t.Error("una semana (7 barras) no es un gráfico largo")
	}
	if html := render(t, Metricas(MetricasPedidos{Barras: mes}, false)); !strings.Contains(html, `class="grafico grafico-largo"`) {
		t.Error("un mes (30 barras) es un gráfico largo")
	}
}
