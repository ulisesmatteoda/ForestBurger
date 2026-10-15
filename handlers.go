package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	sqlc "forestburger/db/sqlc"
)

type API struct {
	q *sqlc.Queries
}

// ---------- DTOs (lo que ve el cliente de la API) ----------

type productoRequest struct {
	Nombre      string  `json:"nombre"`
	Categoria   string  `json:"categoria"`
	Precio      float64 `json:"precio"`
	Descripcion *string `json:"descripcion"`
}

type productoResponse struct {
	ID          int32   `json:"id"`
	Nombre      string  `json:"nombre"`
	Categoria   string  `json:"categoria"`
	Precio      float64 `json:"precio"`
	Descripcion *string `json:"descripcion"`
}

func (r productoRequest) validate() string {
	switch {
	case strings.TrimSpace(r.Nombre) == "":
		return "el nombre es obligatorio"
	case strings.TrimSpace(r.Categoria) == "":
		return "la categoría es obligatoria"
	case r.Precio < 0:
		return "el precio no puede ser negativo"
	}
	return ""
}

func (r productoRequest) precioStr() string {
	return strconv.FormatFloat(r.Precio, 'f', 2, 64)
}

func (r productoRequest) descripcionNull() sql.NullString {
	if r.Descripcion == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *r.Descripcion, Valid: true}
}

func toResponse(p sqlc.Producto) productoResponse {
	precio, _ := strconv.ParseFloat(p.Precio, 64)
	var desc *string
	if p.Descripcion.Valid {
		desc = &p.Descripcion.String
	}
	return productoResponse{
		ID:          p.ID,
		Nombre:      p.Nombre,
		Categoria:   p.Categoria,
		Precio:      precio,
		Descripcion: desc,
	}
}

// ---------- helpers ----------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func pathID(r *http.Request) (int32, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 32)
	return int32(id), err
}

// decodeProducto lee y valida el body; si hay error ya responde al cliente.
func decodeProducto(w http.ResponseWriter, r *http.Request) (productoRequest, bool) {
	var req productoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "JSON inválido")
		return req, false
	}
	if msg := req.validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return req, false
	}
	return req, true
}

// ---------- handlers ----------

func (a *API) createProducto(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeProducto(w, r)
	if !ok {
		return
	}
	p, err := a.q.CreateProducto(r.Context(), sqlc.CreateProductoParams{
		Nombre:      req.Nombre,
		Categoria:   req.Categoria,
		Precio:      req.precioStr(),
		Descripcion: req.descripcionNull(),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toResponse(p))
}

func (a *API) listProductos(w http.ResponseWriter, r *http.Request) {
	ps, err := a.q.ListProductos(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]productoResponse, 0, len(ps)) // slice vacío => "[]" y no "null"
	for _, p := range ps {
		out = append(out, toResponse(p))
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) getProducto(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id inválido")
		return
	}
	p, err := a.q.GetProducto(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "producto no encontrado")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toResponse(p))
}

func (a *API) updateProducto(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id inválido")
		return
	}
	req, ok := decodeProducto(w, r)
	if !ok {
		return
	}
	/*
		Pasamos el valor del precio a string por que la api lee "precio" : "308.00", sirve y funciona de este modo pero para
		quien la usa (cliente), tiene que mandar los precios entre comillas. Podemos sacar ese metodo y cambiar un par de lineas
		en el sqlc.yaml, pero con los DTO's armados al inicio del archivo se arma la conversion de string a float64.
	*/
	p, err := a.q.UpdateProducto(r.Context(), sqlc.UpdateProductoParams{
		ID:          id,
		Nombre:      req.Nombre,
		Categoria:   req.Categoria,
		Precio:      req.precioStr(),
		Descripcion: req.descripcionNull(),
	})
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "producto no encontrado")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toResponse(p))
}

func (a *API) deleteProducto(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id inválido")
		return
	}
	// DeleteProducto es :exec y no avisa si no existía, así que se verifica antes.
	if _, err := a.q.GetProducto(r.Context(), id); errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "producto no encontrado")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := a.q.DeleteProducto(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}