package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	sqlc "forestburger/db/sqlc"
)

// ajustesValidos es un form completo y correcto: lunes cerrado y el resto de 20 a 00 (viernes y
// sábado hasta la 01).
func ajustesValidos() url.Values {
	v := url.Values{
		"whatsapp":         {"223 600-1122"},
		"direccion_retiro": {"Güemes 2850"},
		"alias_cbu":        {"forest.burger.mp"},
		"titular":          {"Forest SRL"},
		"delivery_activo":  {"1"},
		"costo_envio":      {"1500"},
		"pedido_minimo":    {""},
	}
	for i := 0; i < 7; i++ {
		if i != 0 {
			v.Set(fmt.Sprintf("abierto_%d", i), "1")
		}
		v.Set(fmt.Sprintf("desde_%d", i), "20:00")
		hasta := "00:00"
		if i == 4 || i == 5 {
			hasta = "01:00"
		}
		v.Set(fmt.Sprintf("hasta_%d", i), hasta)
	}
	return v
}

func TestAjustes_Pagina(t *testing.T) {
	mux, _ := setup(t)

	rec := pedirPanel(mux, "/admin/ajustes", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quiero 200", rec.Code)
	}
	body := rec.Body.String()
	for _, esperado := range []string{
		"<h1>AJUSTES</h1>", "HORARIOS", "CONTACTO", "PAGO POR TRANSFERENCIA", "DELIVERY",
		`href="/admin/ajustes" class="nav-item activo"`,
		"Lunes", "Domingo", `name="abierto_6"`, `value="20:00"`,
		"[ALIAS DEL LOCAL]", "GUARDAR CAMBIOS",
		"Local abierto", `hx-put="/admin/local/abierto"`, // el switch de la barra lateral
	} {
		if !strings.Contains(body, esperado) {
			t.Errorf("la página no contiene %q", esperado)
		}
	}
}

func TestGuardarAjustes_Bien(t *testing.T) {
	mux, q := setup(t)

	rec := pedirExtra(mux, http.MethodPost, "/admin/ajustes", ajustesValidos(), true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quiero 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Cambios guardados") || strings.Contains(body, "<html") {
		t.Errorf("tendría que devolver solo el form con el aviso de guardado: %s", body)
	}

	aj, _ := q.GetAjustes(context.Background())
	if aj.Whatsapp != "2236001122" || aj.AliasCbu != "forest.burger.mp" || aj.CostoEnvio != "1500.00" || aj.PedidoMinimo.Valid || !aj.DeliveryActivo {
		t.Errorf("ajustes guardados incorrectos: %+v", aj)
	}
	horarios, _ := q.ListHorarios(context.Background())
	if horarios[0].Abierto || !horarios[4].Abierto || horarios[4].Hasta != "01:00" {
		t.Errorf("horarios guardados incorrectos: %+v", horarios)
	}
}

func TestGuardarAjustes_DiaCerradoConservaSusHoras(t *testing.T) {
	mux, q := setup(t)
	if err := q.UpdateHorario(context.Background(), sqlc.UpdateHorarioParams{Dia: 0, Abierto: true, Desde: "19:30", Hasta: "23:00"}); err != nil {
		t.Fatal(err)
	}

	datos := ajustesValidos()
	datos.Del("desde_0") // el lunes cerrado no manda horas
	datos.Del("hasta_0")
	pedirExtra(mux, http.MethodPost, "/admin/ajustes", datos, true)

	h, _ := q.ListHorarios(context.Background())
	if h[0].Abierto || h[0].Desde != "19:30" || h[0].Hasta != "23:00" {
		t.Errorf("el lunes cerrado tendría que conservar 19:30 a 23:00: %+v", h[0])
	}
}

func TestGuardarAjustes_Errores(t *testing.T) {
	casos := map[string]struct {
		campo, valor, mensaje string
	}{
		"hora inválida":       {"desde_2", "25:00", "Revisá las horas del miércoles"},
		"abre y cierra igual": {"hasta_3", "20:00", "El jueves abre y cierra a la misma hora"},
		"whatsapp con letras": {"whatsapp", "llamame", "El WhatsApp tiene que ser un número"},
		"whatsapp corto":      {"whatsapp", "123", "El WhatsApp tiene que ser un número"},
		"envío negativo":      {"costo_envio", "-1", "El costo de envío tiene que ser un número"},
		"pedido mínimo raro":  {"pedido_minimo", "mucho", "El pedido mínimo tiene que ser un número"},
	}
	for nombre, c := range casos {
		t.Run(nombre, func(t *testing.T) {
			mux, q := setup(t)
			datos := ajustesValidos()
			datos.Set(c.campo, c.valor)

			rec := pedirExtra(mux, http.MethodPost, "/admin/ajustes", datos, true)
			body := rec.Body.String()
			if rec.Code != http.StatusOK || !strings.Contains(body, c.mensaje) || strings.Contains(body, "Cambios guardados") {
				t.Errorf("status = %d; quiero el form con %q: %s", rec.Code, c.mensaje, body)
			}
			// Vuelve lo que se escribió, no lo guardado.
			if !strings.Contains(body, `value="Güemes 2850"`) {
				t.Error("tendría que conservar lo que se escribió")
			}
			if aj, _ := q.GetAjustes(context.Background()); aj.AliasCbu != "" {
				t.Errorf("con un error no tendría que guardar nada: %+v", aj)
			}
		})
	}
}

func TestGuardarAjustes_SinHTMX(t *testing.T) {
	mux, _ := setup(t)

	rec := pedirExtra(mux, http.MethodPost, "/admin/ajustes", ajustesValidos(), false)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/admin/ajustes?guardado=1" {
		t.Errorf("status = %d, Location = %q; quiero 303 a /admin/ajustes?guardado=1", rec.Code, rec.Header().Get("Location"))
	}
	if body := pedirPanel(mux, "/admin/ajustes?guardado=1", false).Body.String(); !strings.Contains(body, "Cambios guardados") || !strings.Contains(body, `value="2236001122"`) {
		t.Error("después del redirect tendría que mostrar el aviso y lo guardado")
	}

	datos := ajustesValidos()
	datos.Set("costo_envio", "-5")
	if rec := pedirExtra(mux, http.MethodPost, "/admin/ajustes", datos, false); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "<html") {
		t.Errorf("con error sin htmx: status = %d; quiero 400 con la página", rec.Code)
	}
}

func TestVistaTransferencia(t *testing.T) {
	mux, _ := setup(t)

	body := pedirPanel(mux, "/admin/ajustes/transferencia?alias_cbu=forest.mp&titular=Juan+P%C3%A9rez", true).Body.String()
	if !strings.Contains(body, "forest.mp") || !strings.Contains(body, "Titular: Juan Pérez") || !strings.Contains(body, `id="vista-transferencia"`) {
		t.Errorf("vista previa incorrecta: %s", body)
	}
	if body := pedirPanel(mux, "/admin/ajustes/transferencia", true).Body.String(); !strings.Contains(body, "[ALIAS DEL LOCAL]") || !strings.Contains(body, "[TITULAR]") {
		t.Errorf("sin datos tendría que mostrar los marcadores: %s", body)
	}
}

func TestAvisoCambios(t *testing.T) {
	mux, _ := setup(t)
	if body := pedirPanel(mux, "/admin/ajustes/cambios", true).Body.String(); !strings.Contains(body, "Tenés cambios sin guardar") || !strings.Contains(body, `href="/admin/ajustes"`) {
		t.Errorf("aviso incorrecto: %s", body)
	}
}

func TestLocalAbierto(t *testing.T) {
	mux, q := setup(t)

	// Cerrar: el checkbox apagado no viaja.
	body := pedirExtra(mux, http.MethodPut, "/admin/local/abierto", url.Values{}, true).Body.String()
	if !strings.Contains(body, "Local cerrado") || strings.Contains(body, "<html") {
		t.Errorf("tendría que devolver el switch cerrado: %s", body)
	}
	if aj, _ := q.GetAjustes(context.Background()); aj.LocalAbierto {
		t.Error("no se guardó el cierre")
	}
	// La barra lateral de cualquier pantalla lo refleja.
	if !strings.Contains(pedirPanel(mux, "/admin", false).Body.String(), "Local cerrado") {
		t.Error("la barra lateral tendría que mostrar el local cerrado")
	}

	body = pedirExtra(mux, http.MethodPut, "/admin/local/abierto", url.Values{"abierto": {"1"}}, true).Body.String()
	if !strings.Contains(body, "Local abierto") {
		t.Errorf("tendría que volver abierto: %s", body)
	}
}

func TestAjustes_PideSesion(t *testing.T) {
	mux, _ := setupSinSesion(t)
	if rec := pedirPanel(mux, "/admin/ajustes", false); rec.Code != http.StatusSeeOther {
		t.Errorf("sin sesión: status = %d, quiero 303", rec.Code)
	}
	if rec := pedirExtra(mux, http.MethodPut, "/admin/local/abierto", url.Values{}, true); rec.Header().Get("HX-Redirect") != "/login" {
		t.Error("el switch sin sesión tendría que mandar al login")
	}
}
