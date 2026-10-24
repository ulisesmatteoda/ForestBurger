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

// ---------- DTOs (lo que ve el cliente de la API) ----------

type productoRequest struct {
	Nombre      string  `json:"nombre"`
	Categoria   string  `json:"categoria"`
	Precio      float64 `json:"precio"`
	PrecioDoble *float64 `json:"precio_doble"` // null si el producto no tiene versión doble
	Descripcion *string `json:"descripcion"`
}

type productoResponse struct {
	ID          int32   `json:"id"`
	Nombre      string  `json:"nombre"`
	Categoria   string  `json:"categoria"`
	Precio      float64 `json:"precio"`
	PrecioDoble *float64 `json:"precio_doble"`
	Descripcion *string `json:"descripcion"`
}

func (r productoRequest) validate() error {
	switch {
	case strings.TrimSpace(r.Nombre) == "":
		return errors.New("el nombre es obligatorio")
	case strings.TrimSpace(r.Categoria) == "":
		return errors.New("la categoría es obligatoria")
	case r.Precio < 0:
		return errors.New("el precio no puede ser negativo")
	case r.PrecioDoble != nil && *r.PrecioDoble < 0:
		return errors.New("el precio doble no puede ser negativo")
	}
	return nil
}

func toResponse(p sqlc.Producto) productoResponse {
	precio, _ := strconv.ParseFloat(p.Precio, 64)
	return productoResponse{
		ID:          p.ID,
		Nombre:      p.Nombre,
		Categoria:   p.Categoria,
		Precio:      precio,
		PrecioDoble: nullToFloatPtr(p.PrecioDoble),
		Descripcion: nullToPtr(p.Descripcion),
	}
}

// decodeProducto lee y valida el body; si hay error ya responde al cliente.
func decodeProducto(w http.ResponseWriter, r *http.Request) (productoRequest, bool) {
	var req productoRequest
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

func (a *API) CreateProducto(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeProducto(w, r)
	if !ok {
		return
	}
	p, err := a.q.CreateProducto(r.Context(), sqlc.CreateProductoParams{
		Nombre:      req.Nombre,
		Categoria:   req.Categoria,
		Precio:      floatToStr(req.Precio),
		PrecioDoble: floatPtrToNull(req.PrecioDoble),
		Descripcion: toNullString(req.Descripcion),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toResponse(p))
}

func (a *API) ListProductos(w http.ResponseWriter, r *http.Request) {
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

func (a *API) GetProducto(w http.ResponseWriter, r *http.Request) {
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

func (a *API) UpdateProducto(w http.ResponseWriter, r *http.Request) {
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
		Precio:      floatToStr(req.Precio),
		PrecioDoble: floatPtrToNull(req.PrecioDoble),
		Descripcion: toNullString(req.Descripcion),
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

func (a *API) DeleteProducto(w http.ResponseWriter, r *http.Request) {
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
