package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

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
	ImagenURL   *string `json:"imagen_url"`
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
		ImagenURL:   nullToPtr(p.ImagenUrl),
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

// UpdateImagenProducto: POST /api/productos/{id}/imagen
// Recibe la foto como multipart/form-data (campo "imagen"), la guarda en uploads/
// y actualiza imagen_url del producto. La base guarda solo la ruta, nunca la imagen.
func (a *API) UpdateImagenProducto(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id inválido")
		return
	}

	// El producto tiene que existir (y así sabemos si ya tenía una foto para borrarla).
	p, err := a.q.GetProducto(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "producto no encontrado")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Limitar el tamaño a 5 MB antes de leer nada.
	const maxTam = 5 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxTam)
	if err := r.ParseMultipartForm(maxTam); err != nil {
		writeError(w, http.StatusBadRequest, "La imagen no puede pesar más de 5 MB")
		return
	}
	file, _, err := r.FormFile("imagen")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Falta la imagen")
		return
	}
	defer file.Close()

	// Ver qué es el archivo mirando su contenido, no la extensión (que se puede falsear).
	buf := make([]byte, 512)
	n, _ := file.Read(buf)
	extensiones := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}
	ext, ok := extensiones[http.DetectContentType(buf[:n])]
	if !ok {
		writeError(w, http.StatusBadRequest, "La imagen tiene que ser JPG, PNG o WEBP")
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil { // volver al inicio para copiarlo entero
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Guardar con un nombre propio: id + timestamp. El timestamp hace que, si cambian la foto,
	// el navegador no muestre la vieja guardada en caché.
	if err := os.MkdirAll("uploads", 0755); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	nombre := fmt.Sprintf("%d-%d%s", id, time.Now().Unix(), ext)
	dst, err := os.Create(filepath.Join("uploads", nombre))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := io.Copy(dst, file); err != nil {
		dst.Close()
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dst.Close()

	// Guardar la ruta en la base.
	actualizado, err := a.q.UpdateImagenProducto(r.Context(), sqlc.UpdateImagenProductoParams{
		ID:        id,
		ImagenUrl: sql.NullString{String: "/uploads/" + nombre, Valid: true},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Si tenía una foto anterior (y no es el mismo archivo), borrarla para no acumular basura.
	if p.ImagenUrl.Valid {
		vieja := filepath.Base(p.ImagenUrl.String)
		if vieja != nombre {
			os.Remove(filepath.Join("uploads", vieja))
		}
	}

	writeJSON(w, http.StatusOK, toResponse(actualizado))
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
