package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	sqlc "forestburger/db/sqlc"
	"forestburger/views"
)

// Sección Ajustes del panel: los datos del local (horarios, contacto, transferencia, delivery) y
// el switch "Local abierto" de la barra lateral.

const (
	formatoHora      = "15:04"
	largoMinWhatsapp = 8
	largoMaxWhatsapp = 15
)

// Lo que se acepta en el WhatsApp además de los números; se limpia antes de guardar.
var separadoresTelefono = regexp.MustCompile(`[\s\-+().]`)

// Ajustes: GET /admin/ajustes (?guardado=1 después de guardar sin JS)
func (a *API) Ajustes(w http.ResponseWriter, r *http.Request) {
	form, err := a.formAjustesGuardado(r.Context())
	if err != nil {
		http.Error(w, "No se pudieron cargar los ajustes", http.StatusInternalServerError)
		return
	}
	form.Guardado = r.URL.Query().Get("guardado") == "1"
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.AjustesPage(form).Render(r.Context(), w)
}

// GuardarAjustes: POST /admin/ajustes
// Valida todo y guarda en una transacción (los datos del local y los 7 días juntos: o se guarda
// todo o nada). Con htmx devuelve el form entero (con el aviso o el error); sin htmx, redirect o
// la página con el error.
func (a *API) GuardarAjustes(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	guardado, err := a.formAjustesGuardado(r.Context())
	if err != nil {
		http.Error(w, "No se pudieron cargar los ajustes", http.StatusInternalServerError)
		return
	}
	form := formAjustesDesdeRequest(r, guardado)
	params, msg := validarAjustes(&form)
	if msg != "" {
		form.Error = msg
		a.responderAjustes(w, r, form, http.StatusBadRequest)
		return
	}
	if err := a.guardarAjustes(r.Context(), params, form.Dias); err != nil {
		form.Error = "No se pudieron guardar los ajustes"
		a.responderAjustes(w, r, form, http.StatusInternalServerError)
		return
	}
	if !esHTMX(r) {
		http.Redirect(w, r, "/admin/ajustes?guardado=1", http.StatusSeeOther)
		return
	}
	// Se vuelve a leer de la base, así el form muestra exactamente lo guardado (el WhatsApp limpio,
	// los precios normalizados).
	form, err = a.formAjustesGuardado(r.Context())
	if err != nil {
		http.Error(w, "No se pudieron cargar los ajustes", http.StatusInternalServerError)
		return
	}
	form.Guardado = true
	a.responderAjustes(w, r, form, http.StatusOK)
}

// VistaTransferencia: GET /admin/ajustes/transferencia?alias_cbu=...&titular=...
// La vista previa "Así lo ve el cliente", mientras se escribe.
func (a *API) VistaTransferencia(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	q := r.URL.Query()
	views.VistaTransferencia(strings.TrimSpace(q.Get("alias_cbu")), strings.TrimSpace(q.Get("titular"))).Render(r.Context(), w)
}

// AvisoCambios: GET /admin/ajustes/cambios
// "Tenés cambios sin guardar · DESCARTAR", que htmx pide con el primer cambio del form.
func (a *API) AvisoCambios(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.AvisoCambios().Render(r.Context(), w)
}

// CambiarLocalAbierto: PUT /admin/local/abierto
// El switch de la barra lateral: guarda apenas se toca y devuelve el switch con su texto. Un
// checkbox apagado no viaja, así que "no vino" es "cerrado".
func (a *API) CambiarLocalAbierto(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	abierto, err := a.q.SetLocalAbierto(r.Context(), r.FormValue("abierto") == "1")
	if err != nil {
		http.Error(w, "No se pudo guardar", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.LocalEstado(abierto).Render(r.Context(), w)
}

// ---------- helpers ----------

// formAjustesGuardado arma el form con lo que hay en la base.
func (a *API) formAjustesGuardado(ctx context.Context) (views.FormAjustes, error) {
	aj, err := a.q.GetAjustes(ctx)
	if err != nil {
		return views.FormAjustes{}, err
	}
	horarios, err := a.q.ListHorarios(ctx)
	if err != nil {
		return views.FormAjustes{}, err
	}
	form := views.FormAjustes{
		Whatsapp:        aj.Whatsapp,
		DireccionRetiro: aj.DireccionRetiro,
		AliasCbu:        aj.AliasCbu,
		Titular:         aj.Titular,
		DeliveryActivo:  aj.DeliveryActivo,
		CostoEnvio:      sinCentavosCero(aj.CostoEnvio),
	}
	if aj.PedidoMinimo.Valid {
		form.PedidoMinimo = sinCentavosCero(aj.PedidoMinimo.String)
	}
	for _, h := range horarios {
		form.Dias[h.Dia] = views.FormDia{Abierto: h.Abierto, Desde: h.Desde, Hasta: h.Hasta}
	}
	return form, nil
}

// formAjustesDesdeRequest lee el form tal como vino. Si las horas de un día no vinieron, quedan
// las guardadas (por ejemplo, un día cerrado cuyas horas no se tocaron).
func formAjustesDesdeRequest(r *http.Request, guardado views.FormAjustes) views.FormAjustes {
	form := views.FormAjustes{
		Whatsapp:        strings.TrimSpace(r.PostFormValue("whatsapp")),
		DireccionRetiro: strings.TrimSpace(r.PostFormValue("direccion_retiro")),
		AliasCbu:        strings.TrimSpace(r.PostFormValue("alias_cbu")),
		Titular:         strings.TrimSpace(r.PostFormValue("titular")),
		DeliveryActivo:  r.PostFormValue("delivery_activo") == "1",
		CostoEnvio:      strings.TrimSpace(r.PostFormValue("costo_envio")),
		PedidoMinimo:    strings.TrimSpace(r.PostFormValue("pedido_minimo")),
	}
	for i := range form.Dias {
		d := guardado.Dias[i]
		d.Abierto = r.PostFormValue(fmt.Sprintf("abierto_%d", i)) == "1"
		if v, ok := r.PostForm[fmt.Sprintf("desde_%d", i)]; ok {
			d.Desde = strings.TrimSpace(v[0])
		}
		if v, ok := r.PostForm[fmt.Sprintf("hasta_%d", i)]; ok {
			d.Hasta = strings.TrimSpace(v[0])
		}
		form.Dias[i] = d
	}
	return form
}

// validarAjustes revisa el form y arma lo que se guarda. Deja el WhatsApp limpio (solo números)
// en el form. Si algo está mal devuelve el mensaje para mostrar.
func validarAjustes(form *views.FormAjustes) (sqlc.UpdateAjustesParams, string) {
	for i, d := range form.Dias {
		desde, errDesde := time.Parse(formatoHora, d.Desde)
		hasta, errHasta := time.Parse(formatoHora, d.Hasta)
		if errDesde != nil || errHasta != nil {
			return sqlc.UpdateAjustesParams{}, "Revisá las horas del " + strings.ToLower(views.NombresDias[i]) + ": tienen que ser como 20:00"
		}
		if d.Abierto && desde.Equal(hasta) {
			return sqlc.UpdateAjustesParams{}, "El " + strings.ToLower(views.NombresDias[i]) + " abre y cierra a la misma hora"
		}
	}

	whatsapp := separadoresTelefono.ReplaceAllString(form.Whatsapp, "")
	if whatsapp != "" {
		if _, err := strconv.ParseUint(whatsapp, 10, 64); err != nil || len(whatsapp) < largoMinWhatsapp || len(whatsapp) > largoMaxWhatsapp {
			return sqlc.UpdateAjustesParams{}, "El WhatsApp tiene que ser un número con código de área, por ejemplo 2236001122"
		}
	}
	form.Whatsapp = whatsapp

	costo := form.CostoEnvio
	if costo == "" {
		costo = "0"
	}
	envio, err := strconv.ParseFloat(costo, 64)
	if err != nil || envio < 0 {
		return sqlc.UpdateAjustesParams{}, "El costo de envío tiene que ser un número (0 si es gratis)"
	}
	minimo := sql.NullString{}
	if form.PedidoMinimo != "" {
		m, err := strconv.ParseFloat(form.PedidoMinimo, 64)
		if err != nil || m < 0 {
			return sqlc.UpdateAjustesParams{}, "El pedido mínimo tiene que ser un número, o vacío si no hay mínimo"
		}
		minimo = sql.NullString{String: strconv.FormatFloat(m, 'f', 2, 64), Valid: true}
	}

	return sqlc.UpdateAjustesParams{
		Whatsapp:        whatsapp,
		DireccionRetiro: form.DireccionRetiro,
		AliasCbu:        form.AliasCbu,
		Titular:         form.Titular,
		DeliveryActivo:  form.DeliveryActivo,
		CostoEnvio:      strconv.FormatFloat(envio, 'f', 2, 64),
		PedidoMinimo:    minimo,
	}, ""
}

// guardarAjustes guarda los datos del local y los 7 días en una transacción.
func (a *API) guardarAjustes(ctx context.Context, params sqlc.UpdateAjustesParams, dias [7]views.FormDia) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() // no hace nada si ya se hizo Commit
	q := a.q.WithTx(tx)
	if err := q.UpdateAjustes(ctx, params); err != nil {
		return err
	}
	for i, d := range dias {
		if err := q.UpdateHorario(ctx, sqlc.UpdateHorarioParams{Dia: int16(i), Abierto: d.Abierto, Desde: d.Desde, Hasta: d.Hasta}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// responderAjustes devuelve el form (con htmx) o la página entera (sin htmx). Con htmx siempre es
// 200, porque htmx no hace el swap de los 4xx/5xx.
func (a *API) responderAjustes(w http.ResponseWriter, r *http.Request, form views.FormAjustes, status int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if esHTMX(r) {
		views.AjustesForm(form).Render(r.Context(), w)
		return
	}
	w.WriteHeader(status)
	views.AjustesPage(form).Render(r.Context(), w)
}

// sinCentavosCero muestra "1500.00" como "1500" en los inputs (y deja "1500.50" como está).
func sinCentavosCero(s string) string {
	return strings.TrimSuffix(s, ".00")
}
