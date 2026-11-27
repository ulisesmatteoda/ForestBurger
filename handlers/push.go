package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"forestburger/auth"
	sqlc "forestburger/db/sqlc"
	"forestburger/pedidos"
	"forestburger/views"

	"github.com/SherClockHolmes/webpush-go"
)

// Notificaciones push del panel: cuando entra un pedido de la web, les llega un aviso a los
// dispositivos donde un admin tocó "Activar notificaciones" (Ajustes), aunque la app esté
// cerrada. Es la excepción a "sin JS propio": static/sw.js recibe el aviso y lo muestra, y
// static/panel.js pide el permiso y suscribe el dispositivo. Sin las claves VAPID, todo esto está
// apagado (como en local y en los tests).

// variablesPush son las que hacen falta para mandar notificaciones (todas o ninguna). Las claves
// salen de `make vapid`.
var variablesPush = []string{"VAPID_PUBLIC_KEY", "VAPID_PRIVATE_KEY", "VAPID_SUBJECT"}

// enviadorPush manda un mensaje a una suscripción y devuelve el status del servicio de push del
// navegador. Es una interfaz para que los tests usen uno falso (el de verdad sale a internet).
type enviadorPush interface {
	enviar(ctx context.Context, s sqlc.PushSuscripcion, mensaje []byte) (int, error)
}

type enviadorWebPush struct {
	publica, privada, sujeto string
}

func (e enviadorWebPush) enviar(ctx context.Context, s sqlc.PushSuscripcion, mensaje []byte) (int, error) {
	resp, err := webpush.SendNotificationWithContext(ctx, mensaje, &webpush.Subscription{
		Endpoint: s.Endpoint,
		Keys:     webpush.Keys{P256dh: s.P256dh, Auth: s.Auth},
	}, &webpush.Options{
		Subscriber:      e.sujeto,
		VAPIDPublicKey:  e.publica,
		VAPIDPrivateKey: e.privada,
		TTL:             60 * 60, // si el celu está apagado, el aviso espera hasta una hora
		Urgency:         webpush.UrgencyHigh,
	})
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

// ConfigurarPush prende las notificaciones si están las 3 variables; sin ninguna quedan apagadas;
// con algunas, la app no arranca y dice cuál falta (como las de R2).
func (a *API) ConfigurarPush() error {
	valores := map[string]string{}
	var faltan []string
	for _, v := range variablesPush {
		valores[v] = strings.TrimSpace(os.Getenv(v))
		if valores[v] == "" {
			faltan = append(faltan, v)
		}
	}
	switch len(faltan) {
	case len(variablesPush):
		return nil
	case 0:
	default:
		return fmt.Errorf("faltan %s para las notificaciones push (o se ponen todas, o ninguna; las claves salen de make vapid)", strings.Join(faltan, ", "))
	}
	a.push = enviadorWebPush{publica: valores["VAPID_PUBLIC_KEY"], privada: valores["VAPID_PRIVATE_KEY"], sujeto: valores["VAPID_SUBJECT"]}
	views.ClavePush = valores["VAPID_PUBLIC_KEY"]
	log.Println("Notificaciones push activadas")
	return nil
}

// suscripcionJSON es lo que manda panel.js: PushSubscription.toJSON() del navegador.
type suscripcionJSON struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

// SuscribirPush: POST /admin/push/suscribir (JSON)
// "Activar notificaciones en este dispositivo": guarda la suscripción para el admin de la sesión.
func (a *API) SuscribirPush(w http.ResponseWriter, r *http.Request) {
	if a.push == nil {
		http.NotFound(w, r)
		return
	}
	var s suscripcionJSON
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&s); err != nil || !strings.HasPrefix(s.Endpoint, "https://") || s.Keys.P256dh == "" || s.Keys.Auth == "" {
		http.Error(w, "Suscripción inválida", http.StatusBadRequest)
		return
	}
	if err := a.q.GuardarSuscripcion(r.Context(), sqlc.GuardarSuscripcionParams{
		AdminEmail: auth.AdminDesdeContexto(r.Context()), Endpoint: s.Endpoint, P256dh: s.Keys.P256dh, Auth: s.Keys.Auth,
	}); err != nil {
		http.Error(w, "No se pudo guardar", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DesuscribirPush: POST /admin/push/desuscribir (JSON con el endpoint)
// "Desactivar en este dispositivo": los demás dispositivos siguen recibiendo.
func (a *API) DesuscribirPush(w http.ResponseWriter, r *http.Request) {
	var s suscripcionJSON
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&s); err != nil || s.Endpoint == "" {
		http.Error(w, "Suscripción inválida", http.StatusBadRequest)
		return
	}
	if err := a.q.BorrarSuscripcion(r.Context(), s.Endpoint); err != nil {
		http.Error(w, "No se pudo borrar", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ProbarPush: POST /admin/push/prueba
// Manda una notificación de prueba a los dispositivos del admin de la sesión. Devuelve el texto
// para mostrar en Ajustes.
func (a *API) ProbarPush(w http.ResponseWriter, r *http.Request) {
	if a.push == nil {
		http.NotFound(w, r)
		return
	}
	subs, err := a.q.ListSuscripcionesDe(r.Context(), auth.AdminDesdeContexto(r.Context()))
	if err != nil {
		http.Error(w, "No se pudo leer las suscripciones", http.StatusInternalServerError)
		return
	}
	msg, _ := json.Marshal(avisoPush{Titulo: "Prueba de ForestBurger", Cuerpo: "Así te van a llegar los pedidos nuevos.", URL: "/admin/pedidos", Tag: "prueba"})
	ctx, cancelar := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancelar()
	enviados := a.enviarPush(ctx, subs, msg)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "Notificación de prueba enviada a %s.", conteoDispositivos(enviados))
}

// avisoPush es lo que recibe static/sw.js para armar la notificación.
type avisoPush struct {
	Titulo string `json:"titulo"`
	Cuerpo string `json:"cuerpo"`
	URL    string `json:"url"`
	Tag    string `json:"tag"`
}

// avisoDePedido: "Nuevo pedido #18" · "Martina · $17.000 · Delivery", y al tocarla abre el pedido.
func avisoDePedido(p sqlc.Pedido) avisoPush {
	entrega := "Retiro"
	if p.Entrega == pedidos.EntregaDelivery {
		entrega = "Delivery"
	}
	return avisoPush{
		Titulo: fmt.Sprintf("Nuevo pedido #%d", p.ID),
		Cuerpo: fmt.Sprintf("%s · %s · %s", p.Cliente, views.Precio(p.Total), entrega),
		URL:    fmt.Sprintf("/admin/pedidos/%d", p.ID),
		Tag:    fmt.Sprintf("pedido-%d", p.ID),
	}
}

// notificarPedidoNuevo avisa a todos los dispositivos suscriptos, en segundo plano: el cliente que
// confirmó el pedido no espera a que salgan los avisos.
func (a *API) notificarPedidoNuevo(p sqlc.Pedido) {
	if a.push == nil {
		return
	}
	go func() {
		ctx, cancelar := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelar()
		subs, err := a.q.ListSuscripciones(ctx)
		if err != nil {
			log.Printf("Push: no se pudieron leer las suscripciones: %v", err)
			return
		}
		msg, _ := json.Marshal(avisoDePedido(p))
		a.enviarPush(ctx, subs, msg)
	}()
}

// enviarPush manda el mensaje a cada suscripción y devuelve a cuántas les llegó. Las que el
// servicio de push da por muertas (404 o 410: se desinstaló la app, se borraron los datos) se
// borran solas.
func (a *API) enviarPush(ctx context.Context, subs []sqlc.PushSuscripcion, msg []byte) int {
	enviados := 0
	for _, s := range subs {
		status, err := a.push.enviar(ctx, s, msg)
		switch {
		case err != nil:
			log.Printf("Push: no se pudo enviar a %s: %v", s.AdminEmail, err)
		case status == http.StatusNotFound || status == http.StatusGone:
			if err := a.q.BorrarSuscripcion(ctx, s.Endpoint); err != nil {
				log.Printf("Push: no se pudo borrar una suscripción vencida: %v", err)
			}
		case status >= 300:
			log.Printf("Push: el servicio respondió %d para %s", status, s.AdminEmail)
		default:
			enviados++
		}
	}
	return enviados
}

func conteoDispositivos(n int) string {
	if n == 1 {
		return "1 dispositivo"
	}
	return fmt.Sprintf("%d dispositivos", n)
}

// ServiceWorker: GET /sw.js
// Desde la raíz (y no desde /static/) para que pueda tomar el scope /admin/. Sin caché, así un
// cambio llega enseguida.
func ServiceWorker(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, "static/sw.js")
}
