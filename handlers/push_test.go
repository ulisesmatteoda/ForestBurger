package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	sqlc "forestburger/db/sqlc"
	"forestburger/pedidos"
	"forestburger/views"
)

// enviadorFalso anota cada envío y responde el status que diga cada endpoint (201 por defecto).
type enviadorFalso struct {
	mu        sync.Mutex
	mensajes  map[string]avisoPush
	respuesta map[string]int
}

func (f *enviadorFalso) enviar(_ context.Context, s sqlc.PushSuscripcion, msg []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var a avisoPush
	json.Unmarshal(msg, &a)
	f.mensajes[s.Endpoint] = a
	if st, ok := f.respuesta[s.Endpoint]; ok {
		return st, nil
	}
	return http.StatusCreated, nil
}

func (f *enviadorFalso) recibidos() map[string]avisoPush {
	f.mu.Lock()
	defer f.mu.Unlock()
	copia := map[string]avisoPush{}
	for k, v := range f.mensajes {
		copia[k] = v
	}
	return copia
}

func setupPush(t *testing.T) (http.Handler, *sqlc.Queries, *API, *enviadorFalso) {
	t.Helper()
	mux, q, api := setupAPI(t)
	guardarAdminTest(t, q)
	falso := &enviadorFalso{mensajes: map[string]avisoPush{}, respuesta: map[string]int{}}
	api.push = falso
	return conSesion(mux, adminTest), q, api, falso
}

func postJSON(h http.Handler, ruta, cuerpo string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, ruta, strings.NewReader(cuerpo))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func suscripcion(endpoint string) string {
	return `{"endpoint":"` + endpoint + `","keys":{"p256dh":"BClave","auth":"secreto"}}`
}

func TestConfigurarPush(t *testing.T) {
	for _, v := range variablesPush {
		t.Setenv(v, "")
	}
	defer func() { views.ClavePush = "" }()
	a := &API{}
	if err := a.ConfigurarPush(); err != nil || a.push != nil || views.ClavePush != "" {
		t.Errorf("sin variables tendría que quedar apagado: %v", err)
	}
	t.Setenv("VAPID_PUBLIC_KEY", "publica")
	if err := a.ConfigurarPush(); err == nil || !strings.Contains(err.Error(), "faltan VAPID_PRIVATE_KEY, VAPID_SUBJECT") {
		t.Errorf("a medias tendría que decir qué falta: %v", err)
	}
	t.Setenv("VAPID_PRIVATE_KEY", "privada")
	t.Setenv("VAPID_SUBJECT", "mailto:local@forestburger.com")
	if err := a.ConfigurarPush(); err != nil || a.push == nil || views.ClavePush != "publica" {
		t.Errorf("con las tres tendría que prender: %v", err)
	}
}

func TestPush_SuscribirYDesuscribir(t *testing.T) {
	mux, q, _, _ := setupPush(t)
	ctx := context.Background()
	if rec := postJSON(mux, "/admin/push/suscribir", suscripcion("https://push.ejemplo/celu")); rec.Code != http.StatusNoContent {
		t.Fatalf("suscribir: %d %s", rec.Code, rec.Body.String())
	}
	postJSON(mux, "/admin/push/suscribir", suscripcion("https://push.ejemplo/compu"))
	postJSON(mux, "/admin/push/suscribir", suscripcion("https://push.ejemplo/celu")) // otra vez: actualiza
	if subs, _ := q.ListSuscripciones(ctx); len(subs) != 2 || subs[0].AdminEmail != adminTest {
		t.Fatalf("tendría que haber 2 dispositivos del admin: %+v", subs)
	}
	// Desactivar el celu: la compu sigue.
	if rec := postJSON(mux, "/admin/push/desuscribir", `{"endpoint":"https://push.ejemplo/celu"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("desuscribir: %d", rec.Code)
	}
	if subs, _ := q.ListSuscripciones(ctx); len(subs) != 1 || subs[0].Endpoint != "https://push.ejemplo/compu" {
		t.Errorf("tendría que quedar solo la compu: %+v", subs)
	}
	for nombre, cuerpo := range map[string]string{"no es JSON": "hola", "sin https": suscripcion("http://push.ejemplo/x"), "sin claves": `{"endpoint":"https://push.ejemplo/x"}`} {
		if rec := postJSON(mux, "/admin/push/suscribir", cuerpo); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, quiero 400", nombre, rec.Code)
		}
	}
}

func TestPush_PideSesionYConfiguracion(t *testing.T) {
	mux, _ := setupSinSesion(t)
	if rec := postJSON(mux, "/admin/push/suscribir", suscripcion("https://push.ejemplo/celu")); rec.Code != http.StatusSeeOther {
		t.Errorf("sin sesión tendría que mandar al login, vino %d", rec.Code)
	}
	conSes, q := setup(t)
	_ = q
	if rec := postJSON(conSes, "/admin/push/suscribir", suscripcion("https://push.ejemplo/celu")); rec.Code != http.StatusNotFound {
		t.Errorf("sin las claves VAPID el push no existe: %d", rec.Code)
	}
}

// Un pedido de la web avisa a todos los dispositivos; uno que el servicio da por muerto (410) se
// borra solo.
func TestPush_PedidoNuevoAvisaYLimpia(t *testing.T) {
	mux, q, api, falso := setupPush(t)
	ctx := context.Background()
	postJSON(mux, "/admin/push/suscribir", suscripcion("https://push.ejemplo/celu"))
	postJSON(mux, "/admin/push/suscribir", suscripcion("https://push.ejemplo/viejo"))
	falso.respuesta["https://push.ejemplo/viejo"] = http.StatusGone

	prod, err := q.CreateProducto(ctx, sqlc.CreateProductoParams{Nombre: "Classic", Categoria: "hamburguesa", Precio: "17000"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := pedidos.Crear(ctx, api.db, pedidos.NuevoPedido{Cliente: "Martina", Origen: pedidos.OrigenWeb, Entrega: pedidos.EntregaDelivery, Direccion: "Colón 1234", Pago: pedidos.PagoEfectivo,
		Items: []pedidos.NuevoItem{{ProductoID: prod.ID, Tamano: pedidos.TamanoSimple, Cantidad: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	api.notificarPedidoNuevo(p)
	// Va en segundo plano: se espera un poco a que llegue.
	var recibidos map[string]avisoPush
	for i := 0; i < 100 && len(recibidos) < 2; i++ {
		time.Sleep(20 * time.Millisecond)
		recibidos = falso.recibidos()
	}
	a := recibidos["https://push.ejemplo/celu"]
	if a.Titulo != "Nuevo pedido #"+idTexto(p.ID) || a.Cuerpo != "Martina · $17.000 · Delivery" || a.URL != "/admin/pedidos/"+idTexto(p.ID) {
		t.Errorf("aviso = %+v", a)
	}
	for i := 0; i < 100; i++ {
		if subs, _ := q.ListSuscripciones(ctx); len(subs) == 1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("la suscripción vencida (410) tendría que borrarse")
}

func TestPush_Probar(t *testing.T) {
	mux, _, _, falso := setupPush(t)
	postJSON(mux, "/admin/push/suscribir", suscripcion("https://push.ejemplo/celu"))
	rec := postJSON(mux, "/admin/push/prueba", "")
	if !strings.Contains(rec.Body.String(), "enviada a 1 dispositivo") || falso.recibidos()["https://push.ejemplo/celu"].Titulo != "Prueba de ForestBurger" {
		t.Errorf("prueba: %d %s", rec.Code, rec.Body.String())
	}
}

// La tarjeta de Ajustes aparece solo con el push configurado.
func TestPush_TarjetaEnAjustes(t *testing.T) {
	mux, _ := setup(t)
	defer func() { views.ClavePush = "" }()
	if body := pedirPanel(mux, "/admin/ajustes", false).Body.String(); strings.Contains(body, "NOTIFICACIONES") {
		t.Error("sin claves VAPID no va la tarjeta")
	}
	views.ClavePush = "BClavePublica"
	body := pedirPanel(mux, "/admin/ajustes", false).Body.String()
	for _, esperado := range []string{"NOTIFICACIONES", `data-clave="BClavePublica"`, `data-push="activar" hidden`, `data-push="desactivar" hidden`, "Cargando…"} {
		if !strings.Contains(body, esperado) {
			t.Errorf("la tarjeta no tiene %q", esperado)
		}
	}
}
