package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	sqlc "forestburger/db/sqlc"
	"forestburger/pedidos"
	"forestburger/promos"
	"forestburger/views"

	"github.com/a-h/templ"
)

// Sección Promos del panel. Mismo esquema que Extras: con htmx se devuelven pedazos (el panel, la
// tarjeta, el modal); sin htmx, la página entera o un redirect. Las cuentas y los textos los hace
// el paquete promos.

// Lo que entra en una promo: las hamburguesas y las veggies.
var gruposPromo = []struct{ categoria, grupo string }{{"hamburguesa", "HAMBURGUESAS"}, {"veggie", "VEGGIE"}}

// Promos: GET /admin/promos
func (a *API) Promos(w http.ResponseWriter, r *http.Request) {
	a.renderPromos(w, r, nil, http.StatusOK)
}

// NuevaPromo: GET /admin/promos/nueva
// El modal vacío: 2, simples, todos los días. Sin htmx, la página con el modal abierto.
func (a *API) NuevaPromo(w http.ResponseWriter, r *http.Request) {
	f, err := a.armarFormPromo(r.Context(), formPromoDatos{cantidad: 2, tamano: promos.TamanoSimple, dias: todosLosDias()})
	if err != nil {
		http.Error(w, "No se pudieron cargar los productos", http.StatusInternalServerError)
		return
	}
	a.abrirModalPromo(w, r, f)
}

// EditarPromo: GET /admin/promos/{id}/editar
// El mismo modal, con los datos guardados.
func (a *API) EditarPromo(w http.ResponseWriter, r *http.Request) {
	p, ok := a.promoDeRuta(w, r)
	if !ok {
		return
	}
	ids, err := a.q.ListProductosDePromo(r.Context(), p.ID)
	if err != nil {
		http.Error(w, "No se pudo cargar la promo", http.StatusInternalServerError)
		return
	}
	dias := make([]int, len(p.Dias))
	for i, d := range p.Dias {
		dias[i] = int(d)
	}
	f, err := a.armarFormPromo(r.Context(), formPromoDatos{
		id: p.ID, nombre: p.Nombre, precio: precioSinCentavos(p.Precio), cantidad: int(p.Cantidad),
		tamano: p.Tamano, dias: dias, productos: ids, foto: p.ImagenUrl.String,
	})
	if err != nil {
		http.Error(w, "No se pudieron cargar los productos", http.StatusInternalServerError)
		return
	}
	a.abrirModalPromo(w, r, f)
}

// VistaPromo: POST /admin/promos/vista
// Recalcula el modal mientras se arma la promo. Devuelve el modal entero; el que lo pidió se queda
// con la parte que le sirve (hx-select): la vista previa y, según el caso, el stepper, la lista de
// productos o el contador. No guarda nada.
func (a *API) VistaPromo(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	datos := datosDeFormPromo(r, 0)
	if paso, err := strconv.Atoi(r.FormValue("paso")); err == nil {
		datos.cantidad = min(max(datos.cantidad+paso, promos.CantidadMin), promos.CantidadMax)
	}
	datos.marcarTodas = r.FormValue("marcar") == "todas"
	f, err := a.armarFormPromo(r.Context(), datos)
	if err != nil {
		http.Error(w, "No se pudieron cargar los productos", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.ModalPromo(f).Render(r.Context(), w)
}

// CrearPromo: POST /admin/promos
func (a *API) CrearPromo(w http.ResponseWriter, r *http.Request) {
	a.guardarPromo(w, r, 0)
}

// ActualizarPromo: POST /admin/promos/{id}
// Va por POST (y no PUT) para que el form funcione también sin JS.
func (a *API) ActualizarPromo(w http.ResponseWriter, r *http.Request) {
	p, ok := a.promoDeRuta(w, r)
	if !ok {
		return
	}
	a.guardarPromo(w, r, p.ID)
}

// CambiarActivaPromo: PUT /admin/promos/{id}/activa
// El switch de la tarjeta: guarda apenas se toca y devuelve la tarjeta.
func (a *API) CambiarActivaPromo(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, "id inválido", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	p, err := a.q.UpdateActivaPromo(r.Context(), sqlc.UpdateActivaPromoParams{ID: id, Activa: r.FormValue("activa") == "1"})
	if errors.Is(err, sql.ErrNoRows) {
		noEncontrado(w, "promo")
		return
	}
	if err != nil {
		http.Error(w, "No se pudo guardar", http.StatusInternalServerError)
		return
	}
	tarjetas, err := a.tarjetasPromos(r.Context(), []sqlc.Promo{p})
	if err != nil {
		http.Error(w, "No se pudo cargar la promo", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.TarjetaPromo(tarjetas[0], false).Render(r.Context(), w)
}

// ConfirmarBorrarPromo: GET /admin/promos/{id}/borrar
func (a *API) ConfirmarBorrarPromo(w http.ResponseWriter, r *http.Request) {
	p, ok := a.promoDeRuta(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.ModalConfirmarBorradoPromo(p.ID, p.Nombre).Render(r.Context(), w)
}

// BorrarPromo: DELETE /admin/promos/{id}
// Borra la promo (sus productos se van por el CASCADE) y su foto. Con htmx devuelve el panel y
// cierra el modal; sin htmx, a la lista.
func (a *API) BorrarPromo(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, "id inválido", http.StatusBadRequest)
		return
	}
	anterior, err := a.q.GetPromo(r.Context(), id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "No se pudo cargar la promo", http.StatusInternalServerError)
		return
	}
	if err := a.q.DeletePromo(r.Context(), id); err != nil {
		http.Error(w, "No se pudo borrar la promo", http.StatusInternalServerError)
		return
	}
	a.borrarFoto(anterior.ImagenUrl.String)
	a.promoGuardada(w, r, 0)
}

// ---------- guardar ----------

// guardarPromo valida y guarda el alta (id 0) o la edición. La promo y sus productos van en una
// transacción. La foto va a disco: si después algo falla, se borra.
func (a *API) guardarPromo(w http.ResponseWriter, r *http.Request, id int32) {
	if err := parsearFormulario(w, r); err != nil {
		a.errorModalPromo(w, r, formPromoDatos{id: id}, errorFormulario(err))
		return
	}
	datos := datosDeFormPromo(r, id)
	f, err := a.armarFormPromo(r.Context(), datos)
	if err != nil {
		http.Error(w, "No se pudieron cargar los productos", http.StatusInternalServerError)
		return
	}
	p, err := promoDeForm(f)
	if err == nil {
		err = promos.Validar(p)
	}
	if err != nil {
		a.errorModalPromo(w, r, datos, conAvisoFoto(promos.Mensaje(err), r))
		return
	}
	foto, msg := leerFoto(r)
	if msg != "" {
		a.errorModalPromo(w, r, datos, msg)
		return
	}
	nuevaFoto := ""
	if foto != nil {
		if nuevaFoto, err = a.guardarFoto(foto); err != nil {
			a.errorModalPromo(w, r, datos, "No se pudo guardar la foto")
			return
		}
	}

	guardada, anterior, err := a.guardarPromoEnBase(r.Context(), id, p, nuevaFoto, datos.quitarFoto)
	if errors.Is(err, sql.ErrNoRows) {
		a.borrarFoto(nuevaFoto)
		noEncontrado(w, "promo")
		return
	}
	if err != nil {
		a.borrarFoto(nuevaFoto)
		a.errorModalPromo(w, r, datos, "No se pudo guardar la promo")
		return
	}
	if anterior != "" && (nuevaFoto != "" || datos.quitarFoto) {
		a.borrarFoto(anterior)
	}
	a.promoGuardada(w, r, guardada)
}

// guardarPromoEnBase hace el alta o la edición en una transacción. Devuelve el id y la foto que
// tenía antes (para borrarla del disco si se cambió o se quitó).
func (a *API) guardarPromoEnBase(ctx context.Context, id int32, p promos.Promo, nuevaFoto string, quitarFoto bool) (int32, string, error) {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, "", err
	}
	defer tx.Rollback()
	q := a.q.WithTx(tx)

	dias := make([]int32, len(p.Dias))
	for i, d := range p.Dias {
		dias[i] = int32(d)
	}
	anterior := ""
	if id == 0 {
		creada, err := q.CreatePromo(ctx, sqlc.CreatePromoParams{
			Nombre: p.Nombre, Precio: p.Precio.Texto(), Cantidad: int16(p.Cantidad), Tamano: p.Tamano, Dias: dias,
			ImagenUrl: sql.NullString{String: nuevaFoto, Valid: nuevaFoto != ""}, Activa: true,
		})
		if err != nil {
			return 0, "", err
		}
		id = creada.ID
	} else {
		previa, err := q.GetPromo(ctx, id)
		if err != nil {
			return 0, "", err
		}
		anterior = previa.ImagenUrl.String
		if _, err := q.UpdatePromo(ctx, sqlc.UpdatePromoParams{
			ID: id, Nombre: p.Nombre, Precio: p.Precio.Texto(), Cantidad: int16(p.Cantidad), Tamano: p.Tamano, Dias: dias,
		}); err != nil {
			return 0, "", err
		}
		if nuevaFoto != "" || quitarFoto {
			if _, err := q.UpdateImagenPromo(ctx, sqlc.UpdateImagenPromoParams{ID: id, ImagenUrl: sql.NullString{String: nuevaFoto, Valid: nuevaFoto != ""}}); err != nil {
				return 0, "", err
			}
		}
		if err := q.DeleteProductosDePromo(ctx, id); err != nil {
			return 0, "", err
		}
	}
	for _, prod := range p.Productos {
		if err := q.AddProductoAPromo(ctx, sqlc.AddProductoAPromoParams{PromoID: id, ProductoID: prod.ID}); err != nil {
			return 0, "", err
		}
	}
	return id, anterior, tx.Commit()
}

// promoGuardada responde a un alta, edición o borrado que salió bien: con htmx, el panel (con la
// promo resaltada) y el modal cerrado; sin htmx, a la lista.
func (a *API) promoGuardada(w http.ResponseWriter, r *http.Request, id int32) {
	if !esHTMX(r) {
		http.Redirect(w, r, "/admin/promos", http.StatusSeeOther)
		return
	}
	tarjetas, err := a.todasLasTarjetas(r.Context())
	if err != nil {
		http.Error(w, "No se pudieron cargar las promos", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.PromosPanel(tarjetas, id).Render(r.Context(), w)
	views.ModalCerradoOOB().Render(r.Context(), w)
}

// errorModalPromo vuelve a mostrar el modal con el mensaje y lo que se escribió (con htmx, 200 y
// el destino cambiado al modal; sin htmx, la página y 400).
func (a *API) errorModalPromo(w http.ResponseWriter, r *http.Request, datos formPromoDatos, msg string) {
	f, err := a.armarFormPromo(r.Context(), datos)
	if err != nil {
		http.Error(w, "No se pudieron cargar los productos", http.StatusInternalServerError)
		return
	}
	f.Error = msg
	if !esHTMX(r) {
		a.renderPromos(w, r, views.ModalPromo(f), http.StatusBadRequest)
		return
	}
	w.Header().Set("HX-Retarget", "#modal")
	w.Header().Set("HX-Reswap", "innerHTML")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.ModalPromo(f).Render(r.Context(), w)
}

// ---------- el formulario ----------

// formPromoDatos es lo que viene del form (o de la base, al editar), antes de armar la vista.
type formPromoDatos struct {
	id          int32
	nombre      string
	precio      string
	cantidad    int
	tamano      string
	dias        []int
	productos   []int32
	foto        string // la guardada, solo para mostrarla
	quitarFoto  bool
	marcarTodas bool
}

func datosDeFormPromo(r *http.Request, id int32) formPromoDatos {
	d := formPromoDatos{
		id:         id,
		nombre:     strings.TrimSpace(r.FormValue("nombre")),
		precio:     strings.TrimSpace(r.FormValue("precio")),
		tamano:     r.FormValue("tamano"),
		foto:       r.FormValue("foto_actual"),
		quitarFoto: r.FormValue("quitar_foto") == "1",
	}
	d.cantidad, _ = strconv.Atoi(r.FormValue("cantidad"))
	for _, v := range r.Form["dia"] {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 6 {
			d.dias = append(d.dias, n)
		}
	}
	for _, v := range r.Form["producto"] {
		if n, err := strconv.Atoi(v); err == nil {
			d.productos = append(d.productos, int32(n))
		}
	}
	return d
}

// armarFormPromo arma el modal: la lista de productos que pueden entrar (con los marcados) y la
// vista previa con las cuentas, con los precios actuales de la base.
func (a *API) armarFormPromo(ctx context.Context, d formPromoDatos) (views.FormPromo, error) {
	f := views.FormPromo{ID: d.id, Nombre: d.nombre, Precio: d.precio, Cantidad: d.cantidad, Tamano: d.tamano, Foto: d.foto, QuitarFoto: d.quitarFoto}
	for _, dia := range d.dias {
		f.Dias[dia] = true
	}
	productos, err := a.q.ListProductos(ctx)
	if err != nil {
		return f, err
	}
	for _, g := range gruposPromo {
		for _, p := range productos {
			if p.Categoria != g.categoria {
				continue
			}
			e := views.ElegiblePromo{ID: p.ID, Nombre: p.Nombre, Grupo: g.grupo, PorSeparado: p.Precio,
				Marcado: d.marcarTodas || slices.Contains(d.productos, p.ID)}
			if f.Tamano == promos.TamanoDoble {
				e.SinDoble = !p.PrecioDoble.Valid
				e.PorSeparado = p.PrecioDoble.String
			}
			f.Elegibles = append(f.Elegibles, e)
		}
	}
	f.Vista = vistaDePromo(f, productos)
	return f, nil
}

// vistaDePromo arma "Así lo ve el cliente" y las cuentas con lo que hay hasta ahora.
func vistaDePromo(f views.FormPromo, productos []sqlc.Producto) views.VistaPromo {
	v := views.VistaPromo{Nombre: f.Nombre}
	if !f.QuitarFoto {
		v.Foto = f.Foto
	}
	p, err := promoDeFormCon(f, productos)
	if p.Precio > 0 {
		v.Precio = p.Precio.Texto()
	}
	v.Descripcion = promos.Descripcion(p)
	if err != nil || len(p.Productos) == 0 || p.Precio <= 0 {
		return v
	}
	c := promos.Calcular(p)
	v.HayCuentas = true
	v.Min, v.Max, v.MasCara = c.Min.Texto(), c.Max.Texto(), c.MasCara
	if c.Ahorro > 0 {
		v.Ahorro = c.Ahorro.Texto()
	}
	return v
}

// promoDeForm pasa el modal a una promos.Promo (con los productos marcados y sus precios).
func promoDeForm(f views.FormPromo) (promos.Promo, error) {
	return promoDeFormCon(f, nil)
}

func promoDeFormCon(f views.FormPromo, productos []sqlc.Producto) (promos.Promo, error) {
	p := promos.Promo{Nombre: f.Nombre, Cantidad: f.Cantidad, Tamano: f.Tamano}
	for n, hay := range f.Dias {
		if hay {
			p.Dias = append(p.Dias, n)
		}
	}
	if f.Precio != "" {
		precio, err := pedidos.CentavosDe(f.Precio)
		if err != nil {
			return p, errors.New("precio inválido")
		}
		p.Precio = precio
	}
	for _, e := range f.Elegibles {
		if !e.Marcado || e.SinDoble {
			continue
		}
		prod, err := productoDePromo(e, f.Tamano, productos)
		if err != nil {
			return p, err
		}
		p.Productos = append(p.Productos, prod)
	}
	return p, nil
}

// productoDePromo arma el producto con sus dos precios. Los precios vienen de la lista de productos
// si se pasó; si no, del precio "por separado" que ya está en la fila (en el tamaño de la promo).
func productoDePromo(e views.ElegiblePromo, tamano string, productos []sqlc.Producto) (promos.Producto, error) {
	prod := promos.Producto{ID: e.ID, Nombre: e.Nombre}
	for _, p := range productos {
		if p.ID == e.ID {
			simple, err := pedidos.CentavosDe(p.Precio)
			if err != nil {
				return prod, err
			}
			prod.Simple = simple
			if p.PrecioDoble.Valid {
				if prod.Doble, err = pedidos.CentavosDe(p.PrecioDoble.String); err != nil {
					return prod, err
				}
			}
			return prod, nil
		}
	}
	precio, err := pedidos.CentavosDe(e.PorSeparado)
	if err != nil {
		return prod, err
	}
	prod.Simple, prod.Doble = precio, precio
	if tamano != promos.TamanoDoble {
		prod.Doble = 0
	}
	return prod, nil
}

// ---------- la lista ----------

func (a *API) renderPromos(w http.ResponseWriter, r *http.Request, modal templ.Component, status int) {
	tarjetas, err := a.todasLasTarjetas(r.Context())
	if err != nil {
		http.Error(w, "No se pudieron cargar las promos", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	views.PromosPage(tarjetas, modal).Render(r.Context(), w)
}

func (a *API) abrirModalPromo(w http.ResponseWriter, r *http.Request, f views.FormPromo) {
	if !esHTMX(r) {
		a.renderPromos(w, r, views.ModalPromo(f), http.StatusOK)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.ModalPromo(f).Render(r.Context(), w)
}

func (a *API) todasLasTarjetas(ctx context.Context) ([]views.PromoTarjeta, error) {
	lista, err := a.q.ListPromos(ctx)
	if err != nil {
		return nil, err
	}
	return a.tarjetasPromos(ctx, lista)
}

// tarjetasPromos arma las tarjetas con los productos de cada promo y sus cuentas (con los precios
// de hoy: si cambia un precio del menú, cambia el "ahorran hasta").
func (a *API) tarjetasPromos(ctx context.Context, lista []sqlc.Promo) ([]views.PromoTarjeta, error) {
	filas, err := a.q.ListProductosEnPromos(ctx)
	if err != nil {
		return nil, err
	}
	tarjetas := make([]views.PromoTarjeta, 0, len(lista))
	for _, pr := range lista {
		p := promos.Promo{Nombre: pr.Nombre, Cantidad: int(pr.Cantidad), Tamano: pr.Tamano}
		if p.Precio, err = pedidos.CentavosDe(pr.Precio); err != nil {
			return nil, err
		}
		for _, d := range pr.Dias {
			p.Dias = append(p.Dias, int(d))
		}
		t := views.PromoTarjeta{ID: pr.ID, Nombre: pr.Nombre, Precio: pr.Precio, Foto: pr.ImagenUrl.String, Activa: pr.Activa}
		for _, f := range filas {
			if f.PromoID != pr.ID {
				continue
			}
			prod := promos.Producto{ID: f.ID, Nombre: f.Nombre}
			if prod.Simple, err = pedidos.CentavosDe(f.Precio); err != nil {
				return nil, err
			}
			if f.PrecioDoble.Valid {
				if prod.Doble, err = pedidos.CentavosDe(f.PrecioDoble.String); err != nil {
					return nil, err
				}
			}
			p.Productos = append(p.Productos, prod)
			t.Productos = append(t.Productos, f.Nombre)
		}
		t.Encabezado = promos.Encabezado(p)
		t.Dias = promos.TextoDias(p.Dias)
		if c := promos.Calcular(p); c.Ahorro > 0 && len(p.Productos) > 0 {
			t.Ahorro = c.Ahorro.Texto()
		}
		tarjetas = append(tarjetas, t)
	}
	return tarjetas, nil
}

// promoDeRuta lee el {id} de la URL y trae la promo. Si algo falla ya respondió.
func (a *API) promoDeRuta(w http.ResponseWriter, r *http.Request) (sqlc.Promo, bool) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, "id inválido", http.StatusBadRequest)
		return sqlc.Promo{}, false
	}
	p, err := a.q.GetPromo(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		noEncontrado(w, "promo")
		return sqlc.Promo{}, false
	}
	if err != nil {
		http.Error(w, "No se pudo cargar la promo", http.StatusInternalServerError)
		return sqlc.Promo{}, false
	}
	return p, true
}

func todosLosDias() []int { return []int{0, 1, 2, 3, 4, 5, 6} }

// precioSinCentavos muestra "26000.00" como "26000" en el input (si no tiene centavos).
func precioSinCentavos(s string) string {
	return strings.TrimSuffix(s, ".00")
}
