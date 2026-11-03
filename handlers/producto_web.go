package handlers

import (
	"net/http"
	"strconv"
	"strings"

	sqlc "forestburger/db/sqlc"
	"forestburger/views"
)

// Index: GET /
// Trae todos los productos y renderiza la página con el formulario vacío.
func (a *API) Index(w http.ResponseWriter, r *http.Request) {
	a.renderIndex(w, r, views.FormProducto{}, http.StatusOK)
}

// CrearProducto: POST /productos
// Recibe el formulario, valida, guarda y redirige a / (patrón Post/Redirect/Get).
// Si hay un error, vuelve a mostrar la página con el formulario lleno y el mensaje.
func (a *API) CrearProducto(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	form := views.FormProducto{
		Nombre:      strings.TrimSpace(r.FormValue("nombre")),
		Categoria:   r.FormValue("categoria"),
		Precio:      r.FormValue("precio"),
				PrecioDoble: strings.TrimSpace(r.FormValue("precio_doble")),
		Descripcion: strings.TrimSpace(r.FormValue("descripcion")),
	}

	// El formulario manda todo como texto: el precio hay que convertirlo.
	precio, err := strconv.ParseFloat(form.Precio, 64)
	if err != nil {
		form.Error = "Ingresá un precio válido"
		a.renderIndex(w, r, form, http.StatusBadRequest)
		return
	}

	// Se arma el mismo productoRequest que usa la API, así se reutiliza la validación.
	req := productoRequest{Nombre: form.Nombre, Categoria: form.Categoria, Precio: precio}
		// El precio doble es opcional: vacío significa que no tiene versión doble.
	if form.PrecioDoble != "" {
		pd, err := strconv.ParseFloat(form.PrecioDoble, 64)
		if err != nil {
			form.Error = "Ingresá un precio doble válido"
			a.renderIndex(w, r, form, http.StatusBadRequest)
			return
		}
		req.PrecioDoble = &pd
	}
	if form.Descripcion != "" {
		req.Descripcion = &form.Descripcion
	}
	if msg := req.validate(); msg != "" {
		form.Error = msg
		a.renderIndex(w, r, form, http.StatusBadRequest)
		return
	}

	_, err = a.q.CreateProducto(r.Context(), sqlc.CreateProductoParams{
		Nombre:      req.Nombre,
		Categoria:   req.Categoria,
		Precio:      req.precioStr(),
		PrecioDoble: req.precioDobleNull(),
		Descripcion: req.descripcionNull(),
	})
	if err != nil {
		form.Error = "No se pudo guardar el producto"
		a.renderIndex(w, r, form, http.StatusInternalServerError)
		return
	}

	// 303 See Other: el navegador vuelve a pedir / con un GET y ve la lista actualizada.
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// renderIndex trae los productos y renderiza la página completa con el estado del formulario.
func (a *API) renderIndex(w http.ResponseWriter, r *http.Request, form views.FormProducto, status int) {
	productos, err := a.q.ListProductos(r.Context())
	if err != nil {
		http.Error(w, "No se pudieron cargar los productos", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	views.IndexPage(productos, form).Render(r.Context(), w)
}