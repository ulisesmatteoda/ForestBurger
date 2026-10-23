package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	sqlc "forestburger/db/sqlc"
)

// ---------- DTOs ----------

// extraRequest es lo que manda el admin para crear o editar un extra (ej: cheddar, bacon).
type extraRequest struct {
	Nombre string  `json:"nombre"`
	Precio float64 `json:"precio"`
}

// extraResponse es lo que devuelve la API.
type extraResponse struct {
	ID     int32   `json:"id"`
	Nombre string  `json:"nombre"`
	Precio float64 `json:"precio"`
}

func (r extraRequest) validate() error {
	switch {
	case strings.TrimSpace(r.Nombre) == "":
		return errors.New("el nombre es obligatorio")
	case r.Precio < 0:
		return errors.New("el precio no puede ser negativo")
	}
	return nil
}

// extraToResponse convierte el modelo de sqlc al JSON de la API.
// sqlc genera DECIMAL como string, por eso se pasa el precio a float64.
func extraToResponse(e sqlc.Extra) extraResponse {
	precio, _ := strconv.ParseFloat(e.Precio, 64)
	return extraResponse{
		ID:     e.ID,
		Nombre: e.Nombre,
		Precio: precio,
	}
}

// decodeExtra lee y valida el body; si hay error ya responde al cliente.
func decodeExtra(w http.ResponseWriter, r *http.Request) (extraRequest, bool) {
	var req extraRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "JSON inválido")
		return req, false
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return req, false
	}
	return req, true
}

// ---------- handlers ----------

// CreateExtra: POST /api/extras
func (a *API) CreateExtra(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeExtra(w, r)
	if !ok {
		return
	}
	e, err := a.q.CreateExtra(r.Context(), sqlc.CreateExtraParams{
		Nombre: req.Nombre,
		Precio: floatToStr(req.Precio), // sqlc espera el DECIMAL como string
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, extraToResponse(e))
}

// ListExtras: GET /api/extras
func (a *API) ListExtras(w http.ResponseWriter, r *http.Request) {
	es, err := a.q.ListExtras(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]extraResponse, 0, len(es)) // slice vacío => "[]" y no "null"
	for _, e := range es {
		out = append(out, extraToResponse(e))
	}
	writeJSON(w, http.StatusOK, out)
}

// GetExtra: GET /api/extras/{id}
func (a *API) GetExtra(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id inválido")
		return
	}
	e, err := a.q.GetExtra(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "extra no encontrado")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, extraToResponse(e))
}

// UpdateExtra: PUT /api/extras/{id}
// Cambiar el precio no afecta a los pedidos viejos: guardan su propio precio_unitario.
func (a *API) UpdateExtra(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id inválido")
		return
	}
	req, ok := decodeExtra(w, r)
	if !ok {
		return
	}
	e, err := a.q.UpdateExtra(r.Context(), sqlc.UpdateExtraParams{
		ID:     id,
		Nombre: req.Nombre,
		Precio: floatToStr(req.Precio),
	})
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "extra no encontrado")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, extraToResponse(e))
}

// DeleteExtra: DELETE /api/extras/{id}
// Si el extra ya se usó en algún pedido, Postgres rechaza el borrado por la FK.
func (a *API) DeleteExtra(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id inválido")
		return
	}
	// DeleteExtra es :exec y no avisa si no existía, así que se verifica antes.
	if _, err := a.q.GetExtra(r.Context(), id); errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "extra no encontrado")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := a.q.DeleteExtra(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
