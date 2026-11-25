package handlers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	sqlc "forestburger/db/sqlc"
	"forestburger/pedidos"
	"forestburger/promos"
	"forestburger/views"
)

// La web del cliente (pública, sin sesión): inicio, menú, burger con extras, carrito, revisar,
// confirmar y seguimiento. El pedido entra al sistema como pendiente (le salta el aviso al local)
// y el cliente lo sigue con el link /pedido/{token}.

// Las secciones del menú, en orden. Rapida: se suman con − n + en la misma fila.
var seccionesMenu = []struct {
	categoria, ancla, chip, titulo, nota string
	rapida                               bool
}{
	{"hamburguesa", "smash", "Smash", "SMASH", "Todas las hamburguesas incluyen papas.", false},
	{"veggie", "veggie", "Veggie", "VEGGIE", "", false},
	{"acompañamiento", "acompanamientos", "Acomp.", "ACOMPAÑAMIENTOS", "", true},
	{"bebida", "bebidas", "Bebidas", "BEBIDAS", "", true},
}

var soloDigitos = regexp.MustCompile(`\D`)

const maxExtrasPorUnidad = 5

// ---------- 1 · Inicio ----------

// Inicio: GET /
func (a *API) Inicio(w http.ResponseWriter, r *http.Request) {
	abierto, _, horarios, err := a.estadoLocal(r.Context())
	if err != nil {
		http.Error(w, "No se pudo cargar la página", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.InicioPage(views.VistaInicio{Abierto: abierto, Horarios: pedidos.TextoHorarios(horarios)}).Render(r.Context(), w)
}

// ---------- 2 · Menú ----------

// Menu: GET /menu
func (a *API) Menu(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	abierto, _, _, err := a.estadoLocal(ctx)
	if err != nil {
		http.Error(w, "No se pudo cargar el menú", http.StatusInternalServerError)
		return
	}
	productos, err := a.q.ListProductos(ctx)
	if err != nil {
		http.Error(w, "No se pudo cargar el menú", http.StatusInternalServerError)
		return
	}
	c := leerCarrito(r)
	m := views.VistaMenu{Abierto: abierto}
	for _, s := range seccionesMenu {
		sec := views.SeccionMenu{Ancla: s.ancla, Chip: s.chip, Titulo: s.titulo, Nota: s.nota, Rapida: s.rapida}
		for _, p := range productos {
			if p.Categoria == s.categoria {
				sec.Productos = append(sec.Productos, productoMenu(p, c))
			}
		}
		if len(sec.Productos) > 0 {
			m.Secciones = append(m.Secciones, sec)
		}
	}
	if m.Promos, err = a.promosDeHoy(ctx); err != nil {
		http.Error(w, "No se pudo cargar el menú", http.StatusInternalServerError)
		return
	}
	if m.Barra, err = a.barraPedido(ctx, c); err != nil {
		http.Error(w, "No se pudo cargar el menú", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.MenuPage(m).Render(ctx, w)
}

// CambiarRapido: POST /carrito/rapido (producto, paso)
// El − n + de las bebidas y los acompañamientos en el menú. Devuelve la fila y, por OOB, la barra
// de abajo y el botón del carrito.
func (a *API) CambiarRapido(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	id, err := strconv.Atoi(r.FormValue("producto"))
	if err != nil {
		http.Error(w, "producto inválido", http.StatusBadRequest)
		return
	}
	p, err := a.q.GetProducto(ctx, int32(id))
	if err != nil {
		http.Error(w, "producto inválido", http.StatusBadRequest)
		return
	}
	abierto, _, _, err := a.estadoLocal(ctx)
	if err != nil {
		http.Error(w, "No se pudo guardar", http.StatusInternalServerError)
		return
	}
	c := leerCarrito(r)
	if paso, _ := strconv.Atoi(r.FormValue("paso")); abierto && p.Disponible && (paso == 1 || paso == -1) {
		c.cambiarSimple(p.ID, paso)
		guardarCarrito(w, c)
	}
	barra, err := a.barraPedido(ctx, c)
	if err != nil {
		http.Error(w, "No se pudo guardar", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.FilaRapida(productoMenu(p, c), abierto).Render(ctx, w)
	views.BarraPedidoAbajo(barra, true).Render(ctx, w)
	views.BotonCarrito(barra, true).Render(ctx, w)
}

// ---------- 3 · Burger con extras ----------

// Burger: GET /menu/{id}
func (a *API) Burger(w http.ResponseWriter, r *http.Request) {
	a.responderBurger(w, r, lineaCarrito{Tamano: pedidos.TamanoSimple, Cantidad: 1}, false)
}

// ArmarBurger: POST /menu/{id}/armar
// Recalcula la pantalla mientras se elige (tamaño, extras, cantidad). No guarda nada.
func (a *API) ArmarBurger(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	a.responderBurger(w, r, lineaDeForm(r), true)
}

// AgregarAlCarrito: POST /carrito
// Suma la burger (o lo que venga del form) al carrito y vuelve al menú.
func (a *API) AgregarAlCarrito(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	l := lineaDeForm(r)
	abierto, _, _, err := a.estadoLocal(ctx)
	if err != nil {
		http.Error(w, "No se pudo guardar", http.StatusInternalServerError)
		return
	}
	// Se cotiza antes de guardar: un producto agotado o un extra apagado no entra.
	if _, err := pedidos.CalcularPresupuesto(ctx, a.db, pedidos.NuevoPedido{Items: []pedidos.NuevoItem{l.nuevoItem()}}); abierto && err == nil {
		c := leerCarrito(r)
		c.agregar(l)
		guardarCarrito(w, c)
	}
	http.Redirect(w, r, "/menu", http.StatusSeeOther)
}

func (a *API) responderBurger(w http.ResponseWriter, r *http.Request, l lineaCarrito, soloForm bool) {
	ctx := r.Context()
	id, err := pathID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	p, err := a.q.GetProducto(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		http.Redirect(w, r, "/menu", http.StatusSeeOther)
		return
	}
	if err != nil {
		http.Error(w, "No se pudo cargar el producto", http.StatusInternalServerError)
		return
	}
	abierto, _, _, err := a.estadoLocal(ctx)
	if err != nil {
		http.Error(w, "No se pudo cargar el producto", http.StatusInternalServerError)
		return
	}
	extras, err := a.q.ListExtras(ctx)
	if err != nil {
		http.Error(w, "No se pudo cargar el producto", http.StatusInternalServerError)
		return
	}
	l.Producto = p.ID
	if !p.PrecioDoble.Valid {
		l.Tamano = pedidos.TamanoSimple
	}
	b := views.VistaBurger{
		ID: p.ID, Nombre: p.Nombre, Descripcion: p.Descripcion.String, Foto: p.ImagenUrl.String,
		Simple: p.Precio, Doble: p.PrecioDoble.String, Tamano: l.Tamano, Aclaraciones: l.Aclaraciones,
		Cantidad: l.Cantidad, Abierto: abierto && p.Disponible,
	}
	// Los extras van en las hamburguesas y las veggies, y solo los disponibles.
	if p.Categoria == "hamburguesa" || p.Categoria == "veggie" {
		for _, e := range extras {
			if !e.Disponible {
				continue
			}
			eb := views.ExtraBurger{ID: e.ID, Nombre: e.Nombre, Precio: e.Precio}
			for _, le := range l.Extras {
				if le.ID == e.ID {
					eb.Cantidad = le.Cantidad
				}
			}
			b.Extras = append(b.Extras, eb)
		}
	}
	// Los extras que no están en la lista (apagados, de otra categoría) no se cobran.
	l.Extras = slices.DeleteFunc(l.Extras, func(e extraCarrito) bool {
		return !slices.ContainsFunc(b.Extras, func(eb views.ExtraBurger) bool { return eb.ID == e.ID })
	})
	if pres, err := pedidos.CalcularPresupuesto(ctx, a.db, pedidos.NuevoPedido{Items: []pedidos.NuevoItem{l.nuevoItem()}}); err == nil {
		b.Total = pres.Cuentas.Total.Texto()
	} else {
		b.Abierto = false // agotado: se ve, pero no se puede agregar
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if soloForm {
		views.FormBurger(b).Render(ctx, w)
		return
	}
	views.BurgerPage(b).Render(ctx, w)
}

// lineaDeForm lee la burger del form: tamaño, cantidad (con el paso del − +), extras (con el paso
// del extra tocado) y aclaraciones.
func lineaDeForm(r *http.Request) lineaCarrito {
	id, _ := strconv.Atoi(r.FormValue("producto"))
	if id == 0 {
		id, _ = strconv.Atoi(r.PathValue("id"))
	}
	l := lineaCarrito{Producto: int32(id), Tamano: r.FormValue("tamano"), Aclaraciones: strings.TrimSpace(r.FormValue("aclaraciones"))}
	if l.Tamano != pedidos.TamanoDoble {
		l.Tamano = pedidos.TamanoSimple
	}
	if len([]rune(l.Aclaraciones)) > pedidos.LargoMaxAclaraciones {
		l.Aclaraciones = string([]rune(l.Aclaraciones)[:pedidos.LargoMaxAclaraciones])
	}
	l.Cantidad, _ = strconv.Atoi(r.FormValue("cantidad"))
	paso, _ := strconv.Atoi(r.FormValue("cantidad_paso"))
	l.Cantidad = min(max(l.Cantidad+paso, 1), maxPorLinea)

	tocado, _ := strconv.Atoi(r.FormValue("extra"))
	pasoExtra, _ := strconv.Atoi(r.FormValue("paso"))
	for clave, valores := range r.Form {
		eid, err := strconv.Atoi(strings.TrimPrefix(clave, "extra_"))
		if !strings.HasPrefix(clave, "extra_") || err != nil || len(valores) == 0 {
			continue
		}
		n, _ := strconv.Atoi(valores[0])
		if eid == tocado {
			n += pasoExtra
		}
		if n = min(n, maxExtrasPorUnidad); n > 0 {
			l.Extras = append(l.Extras, extraCarrito{ID: int32(eid), Cantidad: n})
		}
	}
	slices.SortFunc(l.Extras, func(x, y extraCarrito) int { return int(x.ID - y.ID) })
	return l
}

// ---------- 4 · Tu pedido ----------

// Carrito: GET /carrito
func (a *API) Carrito(w http.ResponseWriter, r *http.Request) {
	c := leerCarrito(r)
	v, err := a.vistaCarrito(w, r, c, a.datosConDefecto(r.Context(), leerDatos(r)))
	if err != nil {
		http.Error(w, "No se pudo cargar el pedido", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.CarritoPage(v).Render(r.Context(), w)
}

// CambiarLinea: POST /carrito/linea (i, paso)
// El − n + de un renglón del carrito; en 0 lo saca. Devuelve el form con lo escrito.
func (a *API) CambiarLinea(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	c := leerCarrito(r)
	i, _ := strconv.Atoi(r.FormValue("i"))
	if paso, _ := strconv.Atoi(r.FormValue("paso")); paso == 1 || paso == -1 {
		c.cambiar(i, paso)
		guardarCarrito(w, c)
	}
	a.responderFormCarrito(w, r, c, datosDeForm(r), "")
}

// RecalcularCarrito: POST /carrito/recalcular
// Al cambiar la entrega (aparece la dirección y el envío). No guarda nada.
func (a *API) RecalcularCarrito(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	a.responderFormCarrito(w, r, leerCarrito(r), datosDeForm(r), "")
}

// RevisarCarrito: POST /carrito/revisar
// Valida los datos; si está todo bien los guarda (cookie) y pasa a "Revisá tu pedido".
func (a *API) RevisarCarrito(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	c, d := leerCarrito(r), datosDeForm(r)
	if msg, err := a.validarPedido(r.Context(), c, d); err != nil {
		http.Error(w, "No se pudo revisar el pedido", http.StatusInternalServerError)
		return
	} else if msg != "" {
		a.responderFormCarrito(w, r, c, d, msg)
		return
	}
	guardarCookieJSON(w, cookieDatos, d)
	http.Redirect(w, r, "/carrito/revisar", http.StatusSeeOther)
}

// ---------- 5 · Revisá tu pedido ----------

// Revisar: GET /carrito/revisar
func (a *API) Revisar(w http.ResponseWriter, r *http.Request) {
	c, d := leerCarrito(r), leerDatos(r)
	if msg, err := a.validarPedido(r.Context(), c, d); err != nil || msg != "" {
		http.Redirect(w, r, "/carrito", http.StatusSeeOther)
		return
	}
	v, err := a.vistaCarrito(w, r, c, d)
	if err != nil {
		http.Error(w, "No se pudo cargar el pedido", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.RevisarPage(v).Render(r.Context(), w)
}

// ConfirmarPedido: POST /pedido
// Crea el pedido (pendiente, origen web), vacía el carrito y manda al seguimiento.
func (a *API) ConfirmarPedido(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c, d := leerCarrito(r), leerDatos(r)
	msg, err := a.validarPedido(ctx, c, d)
	if err != nil {
		http.Error(w, "No se pudo mandar el pedido", http.StatusInternalServerError)
		return
	}
	if msg != "" {
		http.Redirect(w, r, "/carrito", http.StatusSeeOther)
		return
	}
	n := pedidos.NuevoPedido{
		Cliente: d.Nombre, Telefono: soloDigitos.ReplaceAllString(d.Telefono, ""), Origen: pedidos.OrigenWeb,
		Entrega: d.Entrega, Direccion: d.Direccion, Pago: d.Pago, Notas: d.Notas,
	}
	for _, l := range c.Lineas {
		n.Items = append(n.Items, l.nuevoItem())
	}
	p, err := pedidos.Crear(ctx, a.db, n)
	if errors.Is(err, pedidos.ErrPedidoInvalido) {
		v, errV := a.vistaCarrito(w, r, c, d)
		if errV != nil {
			http.Error(w, "No se pudo mandar el pedido", http.StatusInternalServerError)
			return
		}
		v.Error = mensajeCliente(err)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		views.RevisarPage(v).Render(ctx, w)
		return
	}
	if err != nil {
		http.Error(w, "No se pudo mandar el pedido", http.StatusInternalServerError)
		return
	}
	borrarCookie(w, cookieCarrito) // los datos quedan, para el próximo pedido
	http.Redirect(w, r, "/pedido/"+p.Token, http.StatusSeeOther)
}

// validarPedido revisa lo que hace falta para mandar el pedido. Devuelve el mensaje para el
// cliente ("" si está todo bien); el error es solo de la base.
func (a *API) validarPedido(ctx context.Context, c carrito, d datosCliente) (string, error) {
	abierto, aj, _, err := a.estadoLocal(ctx)
	if err != nil {
		return "", err
	}
	telefono := soloDigitos.ReplaceAllString(d.Telefono, "")
	switch {
	case !abierto:
		return "Ahora estamos cerrados: no podemos tomar el pedido.", nil
	case len(c.Lineas) == 0:
		return "Tu pedido está vacío.", nil
	case strings.TrimSpace(d.Nombre) == "":
		return "Falta tu nombre.", nil
	case len(telefono) < 8 || len(telefono) > 13:
		return "Poné un teléfono con código de área (ej.: 223 555 1234).", nil
	case d.Entrega != pedidos.EntregaDelivery && d.Entrega != pedidos.EntregaRetiro:
		return "Elegí cómo lo recibís.", nil
	case d.Entrega == pedidos.EntregaDelivery && !aj.DeliveryActivo:
		return "Hoy no hacemos delivery: elegí retiro en el local.", nil
	case d.Entrega == pedidos.EntregaDelivery && strings.TrimSpace(d.Direccion) == "":
		return "Falta la dirección para el delivery.", nil
	case d.Pago != pedidos.PagoEfectivo && d.Pago != pedidos.PagoTransferencia:
		return "Elegí la forma de pago.", nil
	}
	// El pedido mínimo es sobre los productos (sin el envío).
	if aj.PedidoMinimo.Valid {
		minimo, _ := pedidos.CentavosDe(aj.PedidoMinimo.String)
		items := make([]pedidos.NuevoItem, len(c.Lineas))
		for i, l := range c.Lineas {
			items[i] = l.nuevoItem()
		}
		pres, err := pedidos.CalcularPresupuesto(ctx, a.db, pedidos.NuevoPedido{Items: items})
		if errors.Is(err, pedidos.ErrPedidoInvalido) {
			return mensajeCliente(err), nil
		}
		if err != nil {
			return "", err
		}
		if pres.Cuentas.Subtotal < minimo {
			return "El pedido mínimo es de " + views.Precio(minimo.Texto()) + ".", nil
		}
	}
	return "", nil
}

func (a *API) responderFormCarrito(w http.ResponseWriter, r *http.Request, c carrito, d datosCliente, msg string) {
	v, err := a.vistaCarrito(w, r, c, d)
	if err != nil {
		http.Error(w, "No se pudo cargar el pedido", http.StatusInternalServerError)
		return
	}
	v.Error = msg
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if esHTMX(r) {
		views.FormCarrito(v).Render(r.Context(), w)
		return
	}
	views.CarritoPage(v).Render(r.Context(), w)
}

// vistaCarrito cotiza cada renglón con los precios de la base. Los que ya no se pueden pedir (se
// agotó, se borró, un extra se apagó) se sacan del carrito con un aviso.
func (a *API) vistaCarrito(w http.ResponseWriter, r *http.Request, c carrito, d datosCliente) (views.VistaCarrito, error) {
	ctx := r.Context()
	abierto, aj, _, err := a.estadoLocal(ctx)
	if err != nil {
		return views.VistaCarrito{}, err
	}
	v := views.VistaCarrito{Abierto: abierto, DeliveryActivo: aj.DeliveryActivo, Datos: views.DatosVista(d)}
	if !aj.DeliveryActivo && v.Datos.Entrega == pedidos.EntregaDelivery {
		v.Datos.Entrega = pedidos.EntregaRetiro
	}
	validas, avisos, err := a.depurarCarrito(ctx, c)
	if err != nil {
		return v, err
	}
	if len(avisos) > 0 {
		guardarCarrito(w, validas)
	}
	v.Avisos = avisos
	if len(validas.Lineas) == 0 {
		return v, nil
	}
	productos, err := a.q.ListProductos(ctx)
	if err != nil {
		return v, err
	}
	conDoble := map[int32]bool{}
	for _, p := range productos {
		conDoble[p.ID] = p.PrecioDoble.Valid
	}
	n := pedidos.NuevoPedido{Entrega: v.Datos.Entrega}
	for _, l := range validas.Lineas {
		n.Items = append(n.Items, l.nuevoItem())
	}
	pres, err := pedidos.CalcularPresupuesto(ctx, a.db, n)
	if err != nil {
		return v, err
	}
	for i, l := range pres.Lineas {
		cl := views.CliLinea{Titulo: tituloLinea(l.Nombre, l.Tamano, conDoble[validas.Lineas[i].Producto]), Aclaraciones: l.Aclaraciones, Cantidad: l.Cantidad, Total: l.Total.Texto()}
		if l.EsPromo {
			cl.Titulo, cl.Detalle = l.Nombre+" · PROMO", detallePromo(l.Detalle)
		} else {
			for _, e := range l.ExtrasDetalle {
				cl.Detalle = append(cl.Detalle, "+ "+e)
			}
		}
		v.Lineas = append(v.Lineas, cl)
	}
	if pres.Cuentas.Envio > 0 {
		v.Envio = pres.Cuentas.Envio.Texto()
	}
	v.Total = pres.Cuentas.Total.Texto()
	return v, nil
}

// depurarCarrito saca los renglones que ya no se pueden pedir, con un aviso por cada uno.
func (a *API) depurarCarrito(ctx context.Context, c carrito) (carrito, []string, error) {
	var validas carrito
	var avisos []string
	for _, l := range c.Lineas {
		_, err := pedidos.CalcularPresupuesto(ctx, a.db, pedidos.NuevoPedido{Items: []pedidos.NuevoItem{l.nuevoItem()}})
		switch {
		case errors.Is(err, pedidos.ErrPedidoInvalido):
			avisos = append(avisos, "Sacamos algo de tu pedido: "+mensajeCliente(err)+".")
		case err != nil:
			return c, nil, err
		default:
			validas.Lineas = append(validas.Lineas, l)
		}
	}
	return validas, avisos, nil
}

// barraPedido es "N productos · $total" (sin el envío) para el menú.
func (a *API) barraPedido(ctx context.Context, c carrito) (views.BarraPedido, error) {
	validas, _, err := a.depurarCarrito(ctx, c)
	if err != nil || len(validas.Lineas) == 0 {
		return views.BarraPedido{}, err
	}
	n := pedidos.NuevoPedido{}
	for _, l := range validas.Lineas {
		n.Items = append(n.Items, l.nuevoItem())
	}
	pres, err := pedidos.CalcularPresupuesto(ctx, a.db, n)
	if err != nil {
		return views.BarraPedido{}, err
	}
	return views.BarraPedido{Unidades: validas.unidades(), Total: pres.Cuentas.Subtotal.Texto()}, nil
}

func datosDeForm(r *http.Request) datosCliente {
	return datosCliente{
		Nombre:    strings.TrimSpace(r.FormValue("nombre")),
		Telefono:  strings.TrimSpace(r.FormValue("telefono")),
		Entrega:   r.FormValue("entrega"),
		Direccion: strings.TrimSpace(r.FormValue("direccion")),
		Pago:      r.FormValue("pago"),
		Notas:     strings.TrimSpace(r.FormValue("notas")),
	}
}

// datosConDefecto completa la entrega (delivery si se hace) y el pago (efectivo) la primera vez.
func (a *API) datosConDefecto(ctx context.Context, d datosCliente) datosCliente {
	if d.Entrega == "" {
		d.Entrega = pedidos.EntregaRetiro
		if aj, err := a.q.GetAjustes(ctx); err == nil && aj.DeliveryActivo {
			d.Entrega = pedidos.EntregaDelivery
		}
	}
	if d.Pago == "" {
		d.Pago = pedidos.PagoEfectivo
	}
	return d
}

// ---------- 6 · Seguimiento ----------

// Seguimiento: GET /pedido/{token}
func (a *API) Seguimiento(w http.ResponseWriter, r *http.Request) {
	s, err := a.vistaSeguimiento(r)
	if errors.Is(err, sql.ErrNoRows) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		views.PedidoNoEncontrado().Render(r.Context(), w)
		return
	}
	if err != nil {
		http.Error(w, "No se pudo cargar el pedido", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.SeguimientoPage(s).Render(r.Context(), w)
}

// EstadoSeguimiento: GET /pedido/{token}/estado
// Lo pide la página cada 15 segundos: solo la tarjeta del estado y los pasos.
func (a *API) EstadoSeguimiento(w http.ResponseWriter, r *http.Request) {
	s, err := a.vistaSeguimiento(r)
	if err != nil {
		http.Error(w, "No se pudo cargar el pedido", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.EstadoSeguimiento(s).Render(r.Context(), w)
}

func (a *API) vistaSeguimiento(r *http.Request) (views.VistaSeguimiento, error) {
	ctx := r.Context()
	p, err := a.q.GetPedidoPorToken(ctx, r.PathValue("token"))
	if err != nil {
		return views.VistaSeguimiento{}, err
	}
	items, err := a.q.ListItemsPedido(ctx, p.ID)
	if err != nil {
		return views.VistaSeguimiento{}, err
	}
	extras, err := a.q.ListExtrasItemsPedido(ctx, p.ID)
	if err != nil {
		return views.VistaSeguimiento{}, err
	}
	eventos, err := a.q.ListEventosPedido(ctx, p.ID)
	if err != nil {
		return views.VistaSeguimiento{}, err
	}
	aj, err := a.q.GetAjustes(ctx)
	if err != nil {
		return views.VistaSeguimiento{}, err
	}
	productos, err := a.q.ListProductos(ctx)
	if err != nil {
		return views.VistaSeguimiento{}, err
	}
	conDoble := map[int32]bool{}
	for _, pr := range productos {
		conDoble[pr.ID] = pr.PrecioDoble.Valid
	}

	s := views.VistaSeguimiento{
		ID: p.ID, Token: p.Token, Estado: p.Estado, Total: p.Total,
		Transferencia: p.Pago == pedidos.PagoTransferencia, Alias: aj.AliasCbu, Titular: aj.Titular,
		Activa: p.Estado == pedidos.EstadoPendiente || p.Estado == pedidos.EstadoConfirmado || p.Estado == pedidos.EstadoEnCamino || p.Estado == pedidos.EstadoListo,
	}
	s.Link = linkSeguimiento(r, p.Token)
	s.Compartir = "https://wa.me/?text=" + url.QueryEscape("Seguí mi pedido de Forest Burger acá: "+s.Link)
	s.Comprobante = linkWhatsApp(aj.Whatsapp, fmt.Sprintf("Comprobante del pedido #%d", p.ID))
	s.Ayuda = linkWhatsApp(aj.Whatsapp, fmt.Sprintf("Hola, tengo una consulta por el pedido #%d", p.ID))
	if p.CostoEnvio != "0.00" {
		s.Envio = p.CostoEnvio
	}
	s.Entrega = "Retiro en el local"
	if aj.DireccionRetiro != "" {
		s.Entrega += " · " + aj.DireccionRetiro
	}
	if p.Entrega == pedidos.EntregaDelivery {
		s.Entrega = "Delivery · " + p.Direccion
	}
	if p.Notas != "" {
		s.Entrega += " · " + p.Notas
	}

	// Las líneas, con los montos copiados al pedido.
	extrasDe := map[int32][]sqlc.ItemPedidoExtra{}
	for _, e := range extras {
		extrasDe[e.ItemPedidoID] = append(extrasDe[e.ItemPedidoID], e)
	}
	for _, it := range items {
		precio, _ := pedidos.CentavosDe(it.PrecioUnitario)
		linea := pedidos.Linea{Cantidad: int(it.Cantidad), Precio: precio}
		cl := views.CliLinea{Titulo: tituloLinea(it.Nombre, it.Tamano, it.ProductoID.Valid && conDoble[it.ProductoID.Int32]), Aclaraciones: it.Aclaraciones, Cantidad: int(it.Cantidad)}
		esPromo := it.Detalle != ""
		if esPromo {
			cl.Titulo, cl.Detalle = it.Nombre+" · PROMO", []string{it.Detalle}
		}
		for _, e := range extrasDe[it.ID] {
			pe, _ := pedidos.CentavosDe(e.PrecioUnitario)
			linea.Extras = append(linea.Extras, pedidos.LineaExtra{Cantidad: int(e.Cantidad), Precio: pe})
			if !esPromo { // en una promo los extras ya están en el detalle
				cl.Detalle = append(cl.Detalle, fmt.Sprintf("+ %s x%d", e.Nombre, e.Cantidad))
			}
		}
		cl.Total = linea.Total().Texto()
		s.Lineas = append(s.Lineas, cl)
	}

	// El estado y los pasos, con la hora de cada uno.
	horaDe := map[string]time.Time{}
	rechazado := false
	for _, e := range eventos {
		horaDe[e.Estado] = e.CreadoEn.In(pedidos.Zona)
		if e.Estado == pedidos.EstadoCancelado && e.Detalle == pedidos.DetalleRechazado {
			rechazado = true
		}
	}
	llega := ""
	if c, ok := horaDe[pedidos.EstadoConfirmado]; ok && p.DemoraMin.Valid {
		llega = c.Add(time.Duration(p.DemoraMin.Int16) * time.Minute).Format("15:04")
	}
	s.Titulo, s.Texto = textosSeguimiento(p, llega, rechazado, aj.DireccionRetiro)
	if p.Estado != pedidos.EstadoCancelado {
		pasos := []struct{ evento, texto string }{
			{pedidos.EventoRecibido, "Recibimos tu pedido"},
			{pedidos.EstadoConfirmado, "Confirmado, en preparación"},
		}
		if p.Entrega == pedidos.EntregaDelivery {
			pasos = append(pasos, struct{ evento, texto string }{pedidos.EstadoEnCamino, "En camino"})
			pasos = append(pasos, struct{ evento, texto string }{pedidos.EstadoEntregado, "Entregado"})
		} else {
			pasos = append(pasos, struct{ evento, texto string }{pedidos.EstadoListo, "Listo para retirar"})
			pasos = append(pasos, struct{ evento, texto string }{pedidos.EstadoEntregado, "Retirado"})
		}
		for _, paso := range pasos {
			ps := views.PasoSeguimiento{Texto: paso.texto}
			if t, ok := horaDe[paso.evento]; ok {
				ps.Hecho, ps.Hora = true, t.Format("15:04")
			}
			s.Pasos = append(s.Pasos, ps)
		}
	}
	return s, nil
}

// textosSeguimiento es la tarjeta del estado, con los textos del Figma.
func textosSeguimiento(p sqlc.Pedido, llega string, rechazado bool, direccionRetiro string) (titulo, texto string) {
	switch p.Estado {
	case pedidos.EstadoPendiente:
		return "Recibimos tu pedido", "Esperando que Forest lo confirme. Suele tardar unos minutos."
	case pedidos.EstadoConfirmado:
		switch {
		case llega != "" && p.Entrega == pedidos.EntregaDelivery:
			texto = "Llega aprox. a las " + llega + "."
		case llega != "":
			texto = "Va a estar listo para retirar aprox. a las " + llega + "."
		}
		return "¡Confirmado! Lo estamos preparando", texto
	case pedidos.EstadoEnCamino:
		texto = "Ya sale para " + p.Direccion + "."
		if llega != "" {
			texto = "Llega aprox. a las " + llega + " a " + p.Direccion + "."
		}
		return "Tu pedido está en camino", texto
	case pedidos.EstadoListo:
		texto = "Ya podés pasar a retirarlo."
		if direccionRetiro != "" {
			texto = "Ya podés pasar a retirarlo por " + direccionRetiro + "."
		}
		return "¡Tu pedido está listo!", texto
	case pedidos.EstadoEntregado:
		if p.Entrega == pedidos.EntregaRetiro {
			return "¡Retirado! Que lo disfrutes", "Gracias por pedir en Forest."
		}
		return "¡Entregado! Que lo disfrutes", "Gracias por pedir en Forest."
	}
	if rechazado {
		return "No pudimos tomar tu pedido", "Nos quedamos sin algo de lo que pediste. Si ya transferiste, te devolvemos la plata."
	}
	return "Se canceló tu pedido", "Si ya transferiste, te devolvemos la plata. Cualquier duda, escribinos."
}

// ---------- ayudas ----------

// estadoLocal dice si la web toma pedidos ahora (el switch y el horario), con los ajustes y los
// horarios para mostrarlos.
func (a *API) estadoLocal(ctx context.Context) (bool, sqlc.Ajuste, []pedidos.Horario, error) {
	aj, err := a.q.GetAjustes(ctx)
	if err != nil {
		return false, aj, nil, err
	}
	filas, err := a.q.ListHorarios(ctx)
	if err != nil {
		return false, aj, nil, err
	}
	horarios := make([]pedidos.Horario, len(filas))
	for i, h := range filas {
		horarios[i] = pedidos.Horario{Dia: int(h.Dia), Abierto: h.Abierto, Desde: h.Desde, Hasta: h.Hasta}
	}
	return pedidos.AbiertoAhora(aj.LocalAbierto, horarios, time.Now()), aj, horarios, nil
}

// promosDeHoy son las promos activas que están hoy, para el menú.
func (a *API) promosDeHoy(ctx context.Context) ([]views.PromoMenu, error) {
	lista, err := a.q.ListPromos(ctx)
	if err != nil {
		return nil, err
	}
	hoy := int32(time.Now().In(pedidos.Zona).Weekday())
	var deHoy []sqlc.Promo
	for _, p := range lista {
		if p.Activa && slices.Contains(p.Dias, hoy) {
			deHoy = append(deHoy, p)
		}
	}
	tarjetas, err := a.tarjetasPromos(ctx, deHoy)
	if err != nil {
		return nil, err
	}
	var out []views.PromoMenu
	for _, t := range tarjetas {
		if len(t.Productos) == 0 {
			continue
		}
		out = append(out, views.PromoMenu{
			ID: t.ID, Nombre: t.Nombre, Precio: t.Precio, Ahorro: t.Ahorro, Foto: t.Foto,
			Descripcion: strings.TrimSuffix(t.Encabezado, ":") + " " + promos.Lista(t.Productos) + ".",
		})
	}
	return out, nil
}

func productoMenu(p sqlc.Producto, c carrito) views.ProductoMenu {
	return views.ProductoMenu{
		ID: p.ID, Nombre: p.Nombre, Descripcion: p.Descripcion.String, Simple: p.Precio, Doble: p.PrecioDoble.String,
		Agotado: !p.Disponible, EnPedido: c.cantidadDe(p.ID),
	}
}

// tituloLinea: "Classic · Doble", "Classic · Simple" (si tiene doble), o el nombre solo.
func tituloLinea(nombre, tamano string, tieneDoble bool) string {
	switch {
	case tamano == pedidos.TamanoDoble:
		return nombre + " · Doble"
	case tieneDoble:
		return nombre + " · Simple"
	}
	return nombre
}

// detallePromo separa lo elegido en una promo, una hamburguesa por renglón ("· Classic (simple)
// + Cheddar").
func detallePromo(detalle string) []string {
	var out []string
	for _, parte := range strings.Split(detalle, " · ") {
		out = append(out, "· "+parte)
	}
	return out
}

// mensajeCliente saca el texto de un error de pedidos ("Cheese Bacon está agotado").
func mensajeCliente(err error) string {
	m := strings.TrimPrefix(err.Error(), pedidos.ErrPedidoInvalido.Error()+": ")
	if m == "" {
		return m
	}
	r := []rune(m)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}
