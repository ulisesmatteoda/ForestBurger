package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	sqlc "forestburger/db/sqlc"
	"forestburger/views"
)

// Sección Extras del panel. Mismo esquema que Productos: con htmx se devuelven pedazos (el panel,
// la fila, el modal); sin htmx, la página entera o un redirect.

const largoMaxNombreExtra = 100

// Extras: GET /admin/extras
func (a *API) Extras(w http.ResponseWriter, r *http.Request) {
	a.renderExtras(w, r, nil, http.StatusOK)
}

// NuevoExtra: GET /admin/extras/nuevo
// Abre el modal vacío (disponible prendido). Sin htmx, la página con el modal abierto.
func (a *API) NuevoExtra(w http.ResponseWriter, r *http.Request) {
	a.abrirModalExtra(w, r, views.FormExtra{Disponible: true})
}

// EditarExtra: GET /admin/extras/{id}/editar
// El mismo modal, con los datos del extra.
func (a *API) EditarExtra(w http.ResponseWriter, r *http.Request) {
	e, ok := a.extraDeRuta(w, r)
	if !ok {
		return
	}
	a.abrirModalExtra(w, r, views.FormExtra{ID: e.ID, Nombre: e.Nombre, Precio: e.Precio, Disponible: e.Disponible})
}

// CrearExtra: POST /admin/extras
func (a *API) CrearExtra(w http.ResponseWriter, r *http.Request) {
	form, nombre, precio, ok := a.leerFormExtra(w, r, 0)
	if !ok {
		return
	}
	e, err := a.q.CreateExtra(r.Context(), sqlc.CreateExtraParams{Nombre: nombre, Precio: precio, Disponible: form.Disponible})
	if err != nil {
		form.Error = "No se pudo guardar el extra"
		a.errorModalExtra(w, r, form)
		return
	}
	a.extraGuardado(w, r, e.ID)
}

// ActualizarExtra: POST /admin/extras/{id}
// Va por POST (y no PUT) para que el form del modal funcione también sin JS.
func (a *API) ActualizarExtra(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, "id inválido", http.StatusBadRequest)
		return
	}
	form, nombre, precio, ok := a.leerFormExtra(w, r, id)
	if !ok {
		return
	}
	_, err = a.q.UpdateExtra(r.Context(), sqlc.UpdateExtraParams{ID: id, Nombre: nombre, Precio: precio, Disponible: form.Disponible})
	if errors.Is(err, sql.ErrNoRows) {
		noEncontrado(w, "extra")
		return
	}
	if err != nil {
		form.Error = "No se pudo guardar el extra"
		a.errorModalExtra(w, r, form)
		return
	}
	a.extraGuardado(w, r, id)
}

// CambiarDisponibleExtra: PUT /admin/extras/{id}/disponible
// El switch de la tabla: guarda apenas se toca y devuelve la fila. Un checkbox apagado no viaja
// con el pedido, así que "no vino" es "no disponible".
func (a *API) CambiarDisponibleExtra(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, "id inválido", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	e, err := a.q.UpdateDisponibleExtra(r.Context(), sqlc.UpdateDisponibleExtraParams{ID: id, Disponible: r.FormValue("disponible") == "1"})
	if errors.Is(err, sql.ErrNoRows) {
		noEncontrado(w, "extra")
		return
	}
	if err != nil {
		http.Error(w, "No se pudo guardar", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.FilaExtra(e, false).Render(r.Context(), w)
}

// ConfirmarBorrarExtra: GET /admin/extras/{id}/borrar
func (a *API) ConfirmarBorrarExtra(w http.ResponseWriter, r *http.Request) {
	e, ok := a.extraDeRuta(w, r)
	if !ok {
		return
	}
	enCurso, err := a.q.CountPedidosEnCursoConExtra(r.Context(), sql.NullInt32{Int32: e.ID, Valid: true})
	if err != nil {
		http.Error(w, "No se pudo revisar los pedidos", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.ModalConfirmarBorradoExtra(e, enCurso).Render(r.Context(), w)
}

// BorrarExtra: DELETE /admin/extras/{id}
// Con htmx devuelve el panel (con el contador al día) y cierra el modal; sin htmx, a la lista.
func (a *API) BorrarExtra(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, "id inválido", http.StatusBadRequest)
		return
	}
	if err := a.q.DeleteExtra(r.Context(), id); err != nil {
		http.Error(w, "No se pudo borrar el extra", http.StatusInternalServerError)
		return
	}
	if !esHTMX(r) {
		http.Redirect(w, r, "/admin/extras", http.StatusSeeOther)
		return
	}
	a.renderPanelExtras(w, r, 0)
	views.ModalCerradoOOB().Render(r.Context(), w)
}

// ---------- helpers ----------

// leerFormExtra lee y valida el modal. Si algo está mal ya respondió (el modal con el mensaje) y
// devuelve ok en false. El precio vuelve normalizado ("1000.00"), como lo guarda la base.
func (a *API) leerFormExtra(w http.ResponseWriter, r *http.Request, id int32) (views.FormExtra, string, string, bool) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return views.FormExtra{}, "", "", false
	}
	form := views.FormExtra{
		ID:         id,
		Nombre:     strings.TrimSpace(r.FormValue("nombre")),
		Precio:     strings.TrimSpace(r.FormValue("precio")),
		Disponible: r.FormValue("disponible") == "1",
	}
	precio, err := strconv.ParseFloat(form.Precio, 64)
	switch {
	case form.Nombre == "":
		form.Error = "El nombre es obligatorio"
	case len([]rune(form.Nombre)) > largoMaxNombreExtra:
		form.Error = "El nombre es muy largo"
	case err != nil:
		form.Error = "Ingresá un precio válido"
	case precio < 0:
		form.Error = "El precio no puede ser negativo"
	}
	if form.Error != "" {
		a.errorModalExtra(w, r, form)
		return form, "", "", false
	}
	return form, form.Nombre, strconv.FormatFloat(precio, 'f', 2, 64), true
}

// extraGuardado responde a un alta o edición que salió bien: con htmx, el panel con ese extra
// resaltado y el modal cerrado; sin htmx, a la lista.
func (a *API) extraGuardado(w http.ResponseWriter, r *http.Request, id int32) {
	if !esHTMX(r) {
		http.Redirect(w, r, "/admin/extras", http.StatusSeeOther)
		return
	}
	a.renderPanelExtras(w, r, id)
	views.ModalCerradoOOB().Render(r.Context(), w)
}

// errorModalExtra vuelve a mostrar el modal con el mensaje y lo que se escribió. Con htmx responde
// 200 (htmx no hace el swap de los 4xx) y cambia el destino al modal; sin htmx, la página y 400.
func (a *API) errorModalExtra(w http.ResponseWriter, r *http.Request, form views.FormExtra) {
	if !esHTMX(r) {
		a.renderExtras(w, r, &form, http.StatusBadRequest)
		return
	}
	w.Header().Set("HX-Retarget", "#modal")
	w.Header().Set("HX-Reswap", "innerHTML")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.ModalExtra(form).Render(r.Context(), w)
}

// abrirModalExtra muestra el modal: solo el modal con htmx, o la página con el modal abierto.
func (a *API) abrirModalExtra(w http.ResponseWriter, r *http.Request, form views.FormExtra) {
	if !esHTMX(r) {
		a.renderExtras(w, r, &form, http.StatusOK)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.ModalExtra(form).Render(r.Context(), w)
}

func (a *API) renderExtras(w http.ResponseWriter, r *http.Request, modal *views.FormExtra, status int) {
	extras, err := a.q.ListExtras(r.Context())
	if err != nil {
		http.Error(w, "No se pudieron cargar los extras", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	views.ExtrasPage(extras, modal).Render(r.Context(), w)
}

func (a *API) renderPanelExtras(w http.ResponseWriter, r *http.Request, nuevoID int32) {
	extras, err := a.q.ListExtras(r.Context())
	if err != nil {
		http.Error(w, "No se pudieron cargar los extras", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.ExtrasPanel(extras, nuevoID).Render(r.Context(), w)
}

// extraDeRuta lee el {id} de la URL y trae el extra. Si algo falla ya respondió.
func (a *API) extraDeRuta(w http.ResponseWriter, r *http.Request) (sqlc.Extra, bool) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, "id inválido", http.StatusBadRequest)
		return sqlc.Extra{}, false
	}
	e, err := a.q.GetExtra(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		noEncontrado(w, "extra")
		return sqlc.Extra{}, false
	}
	if err != nil {
		http.Error(w, "No se pudo cargar el extra", http.StatusInternalServerError)
		return sqlc.Extra{}, false
	}
	return e, true
}
