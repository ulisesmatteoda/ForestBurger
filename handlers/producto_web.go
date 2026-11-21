package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	sqlc "forestburger/db/sqlc"
	"forestburger/views"
)

// Index: GET /admin (?categoria=<valor> filtra la lista)
// Una categoría que no existe se ignora y se muestran todos, sin error. Con htmx (un chip) devuelve
// solo el panel; sin htmx (F5, un link) la página completa con el filtro ya aplicado.
func (a *API) Index(w http.ResponseWriter, r *http.Request) {
	categoria := r.URL.Query().Get("categoria")
	if !views.CategoriaConocida(categoria) {
		categoria = ""
	}
	if !esHTMX(r) {
		a.renderIndex(w, r, categoria, views.FormProducto{}, http.StatusOK)
		return
	}
	productos, err := a.listarProductos(r.Context(), categoria)
	if err != nil {
		http.Error(w, "No se pudieron cargar los productos", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.ProductosPanel(productos, categoria).Render(r.Context(), w)
}

// CrearProducto: POST /admin/productos
// Recibe el formulario (lo manda HTMX), valida y guarda. Responde con dos fragmentos:
// la lista actualizada y el formulario (vacío si salió bien, con el mensaje si hubo error).
func (a *API) CrearProducto(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	form := formDesdeRequest(r)

	req, msg := armarRequest(form)
	if msg != "" {
		form.Error = msg
		a.renderFragmentos(w, r, form)
		return
	}

	_, err := a.q.CreateProducto(r.Context(), sqlc.CreateProductoParams{
		Nombre:      req.Nombre,
		Categoria:   req.Categoria,
		Precio:      req.precioStr(),
		PrecioDoble: req.precioDobleNull(),
		Descripcion: req.descripcionNull(),
	})
	if err != nil {
		form.Error = "No se pudo guardar el producto"
		a.renderFragmentos(w, r, form)
		return
	}

	// Sin redirect: se devuelve la lista y el formulario vacío (OOB).
	a.renderFragmentos(w, r, views.FormProducto{})
}

// FilaProducto: GET /admin/productos/{id}
// Devuelve la fila del producto en modo lectura (la usa el botón "Cancelar" de la edición).
func (a *API) FilaProducto(w http.ResponseWriter, r *http.Request) {
	p, ok := a.productoDeRuta(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.ProductoFila(p).Render(r.Context(), w)
}

// FormEditarProducto: GET /admin/productos/{id}/editar
// Devuelve la fila del producto en modo edición, con los datos actuales cargados.
func (a *API) FormEditarProducto(w http.ResponseWriter, r *http.Request) {
	p, ok := a.productoDeRuta(w, r)
	if !ok {
		return
	}
	a.renderFilaEditar(w, r, p.ID, formDesdeProducto(p))
}

// ActualizarProducto: PUT /admin/productos/{id}
// Recibe los campos de la fila, valida con las mismas reglas que la creación y actualiza.
// Si salió bien devuelve la fila en modo lectura; si los datos son inválidos devuelve la fila
// en modo edición con el mensaje (con 200, porque HTMX no hace el swap de las respuestas 4xx/5xx).
func (a *API) ActualizarProducto(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, "id inválido", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	form := formDesdeRequest(r)

	req, msg := armarRequest(form)
	if msg != "" {
		form.Error = msg
		a.renderFilaEditar(w, r, id, form)
		return
	}

	// UpdateProducto es :one, así que avisa con sql.ErrNoRows si el producto no existe.
	p, err := a.q.UpdateProducto(r.Context(), sqlc.UpdateProductoParams{
		ID:          id,
		Nombre:      req.Nombre,
		Categoria:   req.Categoria,
		Precio:      req.precioStr(),
		PrecioDoble: req.precioDobleNull(),
		Descripcion: req.descripcionNull(),
	})
	if errors.Is(err, sql.ErrNoRows) {
		productoNoEncontrado(w)
		return
	}
	if err != nil {
		form.Error = "No se pudo actualizar el producto"
		a.renderFilaEditar(w, r, id, form)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.ProductoFila(p).Render(r.Context(), w)
}

// BorrarProducto: DELETE /admin/productos/{id}
func (a *API) BorrarProducto(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, "id inválido", http.StatusBadRequest)
		return
	}
	if err := a.q.DeleteProducto(r.Context(), id); err != nil {
		http.Error(w, "No se pudo borrar el producto", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// ---------- helpers ----------

// formDesdeRequest lee los campos del formulario. Todo llega como texto.
func formDesdeRequest(r *http.Request) views.FormProducto {
	return views.FormProducto{
		Nombre:      strings.TrimSpace(r.FormValue("nombre")),
		Categoria:   r.FormValue("categoria"),
		Precio:      r.FormValue("precio"),
		PrecioDoble: strings.TrimSpace(r.FormValue("precio_doble")),
		Descripcion: strings.TrimSpace(r.FormValue("descripcion")),
	}
}

// armarRequest convierte el formulario (texto) en el mismo productoRequest que usa la API, así
// se reutiliza la validación. Si algo está mal devuelve el mensaje para mostrarle al usuario.
func armarRequest(form views.FormProducto) (productoRequest, string) {
	precio, err := strconv.ParseFloat(form.Precio, 64)
	if err != nil {
		return productoRequest{}, "Ingresá un precio válido"
	}
	req := productoRequest{Nombre: form.Nombre, Categoria: form.Categoria, Precio: precio}

	// El precio doble es opcional: vacío significa que no tiene versión doble.
	if form.PrecioDoble != "" {
		pd, err := strconv.ParseFloat(form.PrecioDoble, 64)
		if err != nil {
			return productoRequest{}, "Ingresá un precio doble válido"
		}
		req.PrecioDoble = &pd
	}
	if form.Descripcion != "" {
		desc := form.Descripcion
		req.Descripcion = &desc
	}
	return req, req.validate()
}

// formDesdeProducto carga el formulario con los datos actuales de un producto.
func formDesdeProducto(p sqlc.Producto) views.FormProducto {
	f := views.FormProducto{Nombre: p.Nombre, Categoria: p.Categoria, Precio: p.Precio}
	if p.PrecioDoble.Valid {
		f.PrecioDoble = p.PrecioDoble.String
	}
	if p.Descripcion.Valid {
		f.Descripcion = p.Descripcion.String
	}
	return f
}

// productoDeRuta lee el {id} de la URL y trae el producto. Si algo falla ya responde al cliente.
func (a *API) productoDeRuta(w http.ResponseWriter, r *http.Request) (sqlc.Producto, bool) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, "id inválido", http.StatusBadRequest)
		return sqlc.Producto{}, false
	}
	p, err := a.q.GetProducto(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		productoNoEncontrado(w)
		return sqlc.Producto{}, false
	}
	if err != nil {
		http.Error(w, "No se pudo cargar el producto", http.StatusInternalServerError)
		return sqlc.Producto{}, false
	}
	return p, true
}

// productoNoEncontrado responde 404. HTMX ignora los 4xx, pero respeta HX-Refresh: la página se
// recarga y la lista deja de mostrar la fila de un producto que ya no existe.
func productoNoEncontrado(w http.ResponseWriter) {
	w.Header().Set("HX-Refresh", "true")
	http.Error(w, "producto no encontrado", http.StatusNotFound)
}

// renderFilaEditar renderiza la fila de un producto en modo edición.
func (a *API) renderFilaEditar(w http.ResponseWriter, r *http.Request, id int32, form views.FormProducto) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.ProductoFilaEditar(id, form).Render(r.Context(), w)
}

// renderIndex trae los productos (filtrados por categoria, "" = todos) y renderiza la página
// completa con el estado del formulario.
func (a *API) renderIndex(w http.ResponseWriter, r *http.Request, categoria string, form views.FormProducto, status int) {
	productos, err := a.listarProductos(r.Context(), categoria)
	if err != nil {
		http.Error(w, "No se pudieron cargar los productos", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	views.IndexPage(productos, categoria, form).Render(r.Context(), w)
}

// renderFragmentos devuelve la lista actualizada y, como segundo fragmento (OOB), el formulario.
func (a *API) renderFragmentos(w http.ResponseWriter, r *http.Request, form views.FormProducto) {
	productos, err := a.q.ListProductos(r.Context())
	if err != nil {
		http.Error(w, "No se pudieron cargar los productos", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.ProductoList(productos, "").Render(r.Context(), w)
	views.ProductoFormOOB(form).Render(r.Context(), w)
}

// listarProductos trae los de una categoría, o todos si categoria es "".
func (a *API) listarProductos(ctx context.Context, categoria string) ([]sqlc.Producto, error) {
	if categoria == "" {
		return a.q.ListProductos(ctx)
	}
	return a.q.ListProductosPorCategoria(ctx, categoria)
}

// esHTMX indica si el pedido lo hizo htmx y espera un fragmento. La excepción es cuando htmx
// reconstruye una página del historial (botón "atrás" sin la página en caché): ahí necesita el
// HTML completo, aunque también mande HX-Request.
func esHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-History-Restore-Request") != "true"
}
