package handlers

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
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
		a.renderIndex(w, r, categoria, nil, http.StatusOK)
		return
	}
	productos, err := a.listarProductos(r.Context(), categoria)
	if err != nil {
		http.Error(w, "No se pudieron cargar los productos", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.ProductosPanel(productos, categoria, 0).Render(r.Context(), w)
}

// NuevoProducto: GET /admin/productos/nuevo
// Abre el modal de alta. Con htmx devuelve solo el modal (va adentro de #modal); sin htmx (link
// directo o F5) la página completa con el modal ya abierto.
func (a *API) NuevoProducto(w http.ResponseWriter, r *http.Request) {
	if !esHTMX(r) {
		a.renderIndex(w, r, "", &views.FormProducto{}, http.StatusOK)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.ModalNuevoProducto(views.FormProducto{}).Render(r.Context(), w)
}

// CerrarModal: GET /admin/modal/cerrar (views.RutaCerrarModal)
// Responde vacío: htmx lo pone adentro de #modal y el modal desaparece.
func (a *API) CerrarModal(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// CrearProducto: POST /admin/productos
// Valida y guarda lo que manda el modal. Con htmx, si sale bien devuelve el panel con todos los
// productos (el nuevo resaltado) y cierra el modal por OOB; si hay error, redirige la respuesta al
// modal (HX-Retarget) para mostrar el mensaje sin tocar la tabla. Sin htmx: redirect o página.
func (a *API) CrearProducto(w http.ResponseWriter, r *http.Request) {
	if err := parsearFormulario(w, r); err != nil {
		a.errorAlta(w, r, views.FormProducto{Error: errorFormulario(err)})
		return
	}
	form := formDesdeRequest(r)

	// Primero se valida todo (datos y foto); recién después se guarda algo.
	req, msg := armarRequest(form)
	foto, msgFoto := leerFoto(r)
	if msg == "" {
		msg = msgFoto
	}
	if msg != "" {
		form.Error = conAvisoFoto(msg, r)
		a.errorAlta(w, r, form)
		return
	}

	imagen := sql.NullString{}
	if foto != nil {
		ruta, err := a.guardarFoto(foto)
		if err != nil {
			form.Error = conAvisoFoto("No se pudo guardar la foto", r)
			a.errorAlta(w, r, form)
			return
		}
		imagen = sql.NullString{String: ruta, Valid: true}
	}

	p, err := a.q.CreateProducto(r.Context(), sqlc.CreateProductoParams{
		Nombre:      req.Nombre,
		Categoria:   req.Categoria,
		Precio:      req.precioStr(),
		PrecioDoble: req.precioDobleNull(),
		Descripcion: req.descripcionNull(),
		ImagenUrl:   imagen,
	})
	if err != nil {
		a.borrarFoto(imagen.String) // el producto no se guardó: la foto quedaría huérfana
		form.Error = conAvisoFoto("No se pudo guardar el producto", r)
		a.errorAlta(w, r, form)
		return
	}

	if !esHTMX(r) {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	productos, err := a.q.ListProductos(r.Context())
	if err != nil {
		http.Error(w, "No se pudieron cargar los productos", http.StatusInternalServerError)
		return
	}
	// El panel vuelve sin filtro, así que la URL también.
	w.Header().Set("HX-Push-Url", "/admin")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.ProductosPanel(productos, "", p.ID).Render(r.Context(), w)
	views.ModalCerradoOOB().Render(r.Context(), w)
}

// errorAlta muestra el modal con el mensaje y lo que se escribió. Con htmx responde 200 (htmx no
// hace el swap de los 4xx) y cambia el destino al modal; sin htmx, la página con el modal y 400.
func (a *API) errorAlta(w http.ResponseWriter, r *http.Request, form views.FormProducto) {
	if !esHTMX(r) {
		a.renderIndex(w, r, "", &form, http.StatusBadRequest)
		return
	}
	w.Header().Set("HX-Retarget", "#modal")
	w.Header().Set("HX-Reswap", "innerHTML")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.ModalNuevoProducto(form).Render(r.Context(), w)
}

// FilaProducto: GET /admin/productos/{id}
// Devuelve la fila del producto en modo lectura (la usa el botón "Cancelar" de la edición).
func (a *API) FilaProducto(w http.ResponseWriter, r *http.Request) {
	p, ok := a.productoDeRuta(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.ProductoFila(p, categoriaActual(r)).Render(r.Context(), w)
}

// FormEditarProducto: GET /admin/productos/{id}/editar
// Devuelve la fila del producto en modo edición, con los datos actuales cargados.
func (a *API) FormEditarProducto(w http.ResponseWriter, r *http.Request) {
	p, ok := a.productoDeRuta(w, r)
	if !ok {
		return
	}
	a.renderFilaEditar(w, r, p.ID, formDesdeProducto(p), p.ImagenUrl.String)
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
	// Se trae antes para saber qué foto tenía (y avisar si ya no existe).
	anterior, err := a.q.GetProducto(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		noEncontrado(w, "producto")
		return
	}
	if err != nil {
		http.Error(w, "No se pudo cargar el producto", http.StatusInternalServerError)
		return
	}
	fotoActual := anterior.ImagenUrl.String

	if err := parsearFormulario(w, r); err != nil {
		form := formDesdeProducto(anterior)
		form.Error = errorFormulario(err)
		a.renderFilaEditar(w, r, id, form, fotoActual)
		return
	}
	form := formDesdeRequest(r)

	req, msg := armarRequest(form)
	foto, msgFoto := leerFoto(r)
	if msg == "" {
		msg = msgFoto
	}
	if msg != "" {
		form.Error = conAvisoFoto(msg, r)
		a.renderFilaEditar(w, r, id, form, fotoActual)
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
		noEncontrado(w, "producto")
		return
	}
	if err != nil {
		form.Error = "No se pudo actualizar el producto"
		a.renderFilaEditar(w, r, id, form, fotoActual)
		return
	}

	// La foto va aparte: una nueva reemplaza a la anterior; "Quitar foto" la saca.
	if foto != nil || r.FormValue("quitar_foto") == "1" {
		nueva := sql.NullString{}
		if foto != nil {
			ruta, err := a.guardarFoto(foto)
			if err != nil {
				form.Error = conAvisoFoto("Se guardaron los datos, pero no la foto", r)
				a.renderFilaEditar(w, r, id, form, fotoActual)
				return
			}
			nueva = sql.NullString{String: ruta, Valid: true}
		}
		if p, err = a.q.UpdateImagenProducto(r.Context(), sqlc.UpdateImagenProductoParams{ID: id, ImagenUrl: nueva}); err != nil {
			a.borrarFoto(nueva.String)
			form.Error = conAvisoFoto("Se guardaron los datos, pero no la foto", r)
			a.renderFilaEditar(w, r, id, form, fotoActual)
			return
		}
		a.borrarFoto(fotoActual)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.ProductoFila(p, categoriaActual(r)).Render(r.Context(), w)
}

// FotoPrevia: POST /admin/productos/foto-previa
// La pide htmx apenas se elige una foto, así se ve que quedó cargada antes de guardar. Valida con
// las mismas reglas que el guardado y devuelve la vista previa o el error; no guarda nada en disco
// (la foto todavía no es de ningún producto), por eso la miniatura va como data URI. Responde
// siempre 200 porque htmx no hace el swap de los 4xx; sin archivo (se canceló el selector), vacío.
func (a *API) FotoPrevia(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := parsearFormulario(w, r); err != nil {
		views.FotoPreviaError(errorFormulario(err)).Render(r.Context(), w)
		return
	}
	foto, msg := leerFoto(r)
	if msg != "" {
		views.FotoPreviaError(msg).Render(r.Context(), w)
		return
	}
	if foto == nil {
		return
	}
	src := "data:" + foto.tipo + ";base64," + base64.StdEncoding.EncodeToString(foto.datos)
	views.FotoPrevia(foto.nombre, len(foto.datos), src).Render(r.Context(), w)
}

// ConfirmarBorrado: GET /admin/productos/{id}/borrar
// Abre el modal que pide confirmar el borrado. ?categoria es el filtro activo, para que después
// del borrado el panel vuelva con el mismo filtro.
func (a *API) ConfirmarBorrado(w http.ResponseWriter, r *http.Request) {
	p, ok := a.productoDeRuta(w, r)
	if !ok {
		return
	}
	enCurso, err := a.q.CountPedidosEnCursoConProducto(r.Context(), sql.NullInt32{Int32: p.ID, Valid: true})
	if err != nil {
		http.Error(w, "No se pudo revisar los pedidos", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.ModalConfirmarBorrado(p, categoriaActual(r), enCurso).Render(r.Context(), w)
}

// BorrarProducto: DELETE /admin/productos/{id}?categoria=<filtro>
// Con htmx devuelve el panel con el filtro que estaba activo (así el contador queda bien) y
// cierra el modal de confirmación por OOB. Sin htmx vuelve al panel.
func (a *API) BorrarProducto(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, "id inválido", http.StatusBadRequest)
		return
	}
	// Se trae antes para borrar también su foto del disco.
	anterior, err := a.q.GetProducto(r.Context(), id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "No se pudo cargar el producto", http.StatusInternalServerError)
		return
	}
	if err := a.q.DeleteProducto(r.Context(), id); err != nil {
		http.Error(w, "No se pudo borrar el producto", http.StatusInternalServerError)
		return
	}
	a.borrarFoto(anterior.ImagenUrl.String)
	if !esHTMX(r) {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	categoria := categoriaActual(r)
	productos, err := a.listarProductos(r.Context(), categoria)
	if err != nil {
		http.Error(w, "No se pudieron cargar los productos", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.ProductosPanel(productos, categoria, 0).Render(r.Context(), w)
	views.ModalCerradoOOB().Render(r.Context(), w)
}

// ---------- helpers ----------

// conAvisoFoto agrega "Volvé a elegir la foto." al error si el pedido traía una: el navegador no
// deja volver a completar un input de archivo, así que al mostrar el error el campo queda vacío.
func conAvisoFoto(msg string, r *http.Request) string {
	if r.MultipartForm != nil {
		for _, f := range r.MultipartForm.File[campoFoto] {
			if f.Size > 0 {
				return msg + ". Volvé a elegir la foto."
			}
		}
	}
	return msg
}

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
		noEncontrado(w, "producto")
		return sqlc.Producto{}, false
	}
	if err != nil {
		http.Error(w, "No se pudo cargar el producto", http.StatusInternalServerError)
		return sqlc.Producto{}, false
	}
	return p, true
}

// noEncontrado responde 404 con el nombre de lo que no está ("producto", "extra"). HTMX ignora los
// 4xx, pero respeta HX-Refresh: la página se recarga y deja de mostrar lo que ya no existe.
func noEncontrado(w http.ResponseWriter, que string) {
	w.Header().Set("HX-Refresh", "true")
	http.Error(w, que+" no encontrado", http.StatusNotFound)
}

// renderFilaEditar renderiza la fila de un producto en modo edición.
func (a *API) renderFilaEditar(w http.ResponseWriter, r *http.Request, id int32, form views.FormProducto, foto string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.ProductoFilaEditar(id, form, foto).Render(r.Context(), w)
}

// renderIndex trae los productos (filtrados por categoria, "" = todos) y renderiza la página
// completa; modal no nil la deja con el modal de alta abierto y esos datos.
func (a *API) renderIndex(w http.ResponseWriter, r *http.Request, categoria string, modal *views.FormProducto, status int) {
	productos, err := a.listarProductos(r.Context(), categoria)
	if err != nil {
		http.Error(w, "No se pudieron cargar los productos", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	views.IndexPage(productos, categoria, modal).Render(r.Context(), w)
}

// categoriaActual es el filtro activo de la lista: el de ?categoria=, o si el pedido no lo trae
// (Guardar y Cancelar de la edición en línea), el de la página desde donde lo mandó htmx
// (header HX-Current-URL). Una categoría desconocida cuenta como "todos" ("").
func categoriaActual(r *http.Request) string {
	categoria := r.URL.Query().Get("categoria")
	if categoria == "" {
		if pagina, err := url.Parse(r.Header.Get("HX-Current-URL")); err == nil {
			categoria = pagina.Query().Get("categoria")
		}
	}
	if !views.CategoriaConocida(categoria) {
		return ""
	}
	return categoria
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
