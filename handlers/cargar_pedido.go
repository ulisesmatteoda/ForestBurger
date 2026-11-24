package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	sqlc "forestburger/db/sqlc"
	"forestburger/pedidos"
	"forestburger/views"
)

// "Cargar pedido": los pedidos que entran por WhatsApp, por teléfono o en el local. El pedido que
// se va armando viaja en el form (no hay estado guardado entre clics): cada acción lo lee, lo
// cambia y devuelve el modal recalculado con los precios de la base.

// Categorías de la izquierda, en el orden del Figma.
var categoriasCarga = []views.CategoriaCarga{
	{Valor: "hamburguesa", Label: "Hamburguesas"},
	{Valor: "veggie", Label: "Veggie"},
	{Valor: "acompañamiento", Label: "Acomp."},
	{Valor: "bebida", Label: "Bebidas"},
}

// Solo las hamburguesas y las veggies llevan extras.
func llevaExtras(categoria string) bool { return categoria == "hamburguesa" || categoria == "veggie" }

// carga es el form del modal ya leído.
type carga struct {
	cliente, telefono, direccion string
	origen, entrega, pago        string
	descTipo, descValor, motivo  string
	categoria                    string
	items                        []itemCarga
}

type itemCarga struct {
	productoID int32
	tamano     string
	cantidad   int
	extras     []int32
}

// CargarPedido: GET /admin/pedidos/cargar
// El modal vacío. Sin htmx, la página de Pedidos con el modal abierto.
func (a *API) CargarPedido(w http.ResponseWriter, r *http.Request) {
	c := carga{origen: pedidos.OrigenWhatsapp, entrega: pedidos.EntregaRetiro, pago: pedidos.PagoEfectivo, categoria: categoriasCarga[0].Valor}
	vista, err := a.vistaCarga(r.Context(), c, "")
	if err != nil {
		http.Error(w, "No se pudo armar el pedido", http.StatusInternalServerError)
		return
	}
	if esHTMX(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		views.ModalCargarPedido(vista).Render(r.Context(), w)
		return
	}
	a.paginaPedidosCon(w, r, views.ModalCargarPedido(vista))
}

// ArmarPedido: POST /admin/pedidos/cargar/armar
// Aplica la acción del botón (agregar, más, menos, un extra, la categoría que se ve) o solo
// recalcula (un cambio de entrega o de descuento), y devuelve el modal.
func (a *API) ArmarPedido(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	c := leerCarga(r)
	aplicarAccion(&c, r)
	a.responderCarga(w, r, c, "")
}

// GuardarPedido: POST /admin/pedidos/cargar
// Crea el pedido (confirmado) y muestra el link para el cliente. Además actualiza la lista y las
// métricas de atrás, por OOB, así el pedido nuevo se ve sin recargar.
func (a *API) GuardarPedido(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	c := leerCarga(r)
	n, msg := nuevoPedidoDe(c)
	if msg != "" {
		a.responderCarga(w, r, c, msg)
		return
	}
	n.Estado = pedidos.EstadoConfirmado
	p, err := pedidos.Crear(r.Context(), a.db, n)
	if errors.Is(err, pedidos.ErrPedidoInvalido) {
		a.responderCarga(w, r, c, mensajePedido(err))
		return
	}
	if err != nil {
		http.Error(w, "No se pudo guardar el pedido", http.StatusInternalServerError)
		return
	}
	if !esHTMX(r) {
		http.Redirect(w, r, fmt.Sprintf("/admin/pedidos/%d", p.ID), http.StatusSeeOther)
		return
	}

	link := linkSeguimiento(r, p.Token)
	whatsapp := linkWhatsApp(p.Telefono, fmt.Sprintf("¡Hola %s! Ya tenemos tu pedido #%d de Forest Burger. Lo podés seguir acá: %s", p.Cliente, p.ID, link))
	// La lista de atrás se vuelve a armar con el filtro que tiene la página (HX-Current-URL).
	filtro := url.Values{}
	if pagina, err := url.Parse(r.Header.Get("HX-Current-URL")); err == nil {
		filtro = pagina.Query()
	}
	f, err := a.armarFiltro(r.Context(), filtro, time.Now())
	if err != nil {
		http.Error(w, "No se pudieron cargar los pedidos", http.StatusInternalServerError)
		return
	}
	filas, err := a.filasPedidos(r.Context(), f)
	if err != nil {
		http.Error(w, "No se pudieron cargar los pedidos", http.StatusInternalServerError)
		return
	}
	m, err := a.metricasPedidos(r.Context(), f, time.Now())
	if err != nil {
		http.Error(w, "No se pudieron calcular las ventas", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.ModalPedidoCargado(p.ID, link, whatsapp).Render(r.Context(), w)
	views.PedidosPanel(f.vista, filas, true).Render(r.Context(), w)
	views.Metricas(m, true).Render(r.Context(), w)
}

// ---------- helpers ----------

// responderCarga devuelve el modal (con el mensaje de error si hay).
func (a *API) responderCarga(w http.ResponseWriter, r *http.Request, c carga, msg string) {
	vista, err := a.vistaCarga(r.Context(), c, msg)
	if err != nil {
		http.Error(w, "No se pudo armar el pedido", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.ModalCargarPedido(vista).Render(r.Context(), w)
}

// leerCarga lee el form del modal: los datos del cliente y los renglones ocultos.
func leerCarga(r *http.Request) carga {
	c := carga{
		cliente:   strings.TrimSpace(r.PostFormValue("cliente")),
		telefono:  strings.TrimSpace(r.PostFormValue("telefono")),
		direccion: strings.TrimSpace(r.PostFormValue("direccion")),
		origen:    r.PostFormValue("origen"),
		entrega:   r.PostFormValue("entrega"),
		pago:      r.PostFormValue("pago"),
		descTipo:  r.PostFormValue("desc_tipo"),
		descValor: strings.TrimSpace(r.PostFormValue("desc_valor")),
		motivo:    strings.TrimSpace(r.PostFormValue("motivo")),
		categoria: r.PostFormValue("categoria"),
	}
	if c.categoria == "" {
		c.categoria = categoriasCarga[0].Valor
	}
	for i := 0; ; i++ {
		prefijo := fmt.Sprintf("item_%d_", i)
		id, err := strconv.ParseInt(r.PostFormValue(prefijo+"producto"), 10, 32)
		if err != nil {
			break // no hay más renglones
		}
		cant, _ := strconv.Atoi(r.PostFormValue(prefijo + "cantidad"))
		it := itemCarga{productoID: int32(id), tamano: r.PostFormValue(prefijo + "tamano"), cantidad: cant}
		for _, e := range strings.Split(r.PostFormValue(prefijo+"extras"), ",") {
			if eid, err := strconv.ParseInt(e, 10, 32); err == nil {
				it.extras = append(it.extras, int32(eid))
			}
		}
		c.items = append(c.items, it)
	}
	return c
}

// aplicarAccion cambia el carrito según el botón que se tocó (viene en hx-vals).
func aplicarAccion(c *carga, r *http.Request) {
	i, _ := strconv.Atoi(r.PostFormValue("i"))
	valido := i >= 0 && i < len(c.items)
	switch r.PostFormValue("accion") {
	case "categoria":
		c.categoria = r.PostFormValue("ver")
	case "agregar":
		id, err := strconv.ParseInt(r.PostFormValue("producto"), 10, 32)
		if err != nil {
			return
		}
		tamano := r.PostFormValue("tamano")
		// El mismo producto y tamaño, todavía sin extras: suma uno a ese renglón.
		for k := range c.items {
			if c.items[k].productoID == int32(id) && c.items[k].tamano == tamano && len(c.items[k].extras) == 0 {
				c.items[k].cantidad++
				return
			}
		}
		c.items = append(c.items, itemCarga{productoID: int32(id), tamano: tamano, cantidad: 1})
	case "mas":
		if valido {
			c.items[i].cantidad++
		}
	case "menos":
		if valido {
			c.items[i].cantidad--
			if c.items[i].cantidad < 1 {
				c.items = append(c.items[:i], c.items[i+1:]...)
			}
		}
	case "extra":
		eid, err := strconv.ParseInt(r.PostFormValue("extra"), 10, 32)
		if !valido || err != nil {
			return
		}
		it := &c.items[i]
		for k, e := range it.extras {
			if e == int32(eid) {
				it.extras = append(it.extras[:k], it.extras[k+1:]...)
				return
			}
		}
		it.extras = append(it.extras, int32(eid))
	}
}

// nuevoPedidoDe pasa el form a un pedido. Los errores de forma (el valor del descuento) vuelven
// como mensaje; los demás los revisa pedidos.Crear.
func nuevoPedidoDe(c carga) (pedidos.NuevoPedido, string) {
	n := pedidos.NuevoPedido{
		Cliente: c.cliente, Telefono: separadoresTelefono.ReplaceAllString(c.telefono, ""),
		Origen: c.origen, Entrega: c.entrega, Direccion: c.direccion, Pago: c.pago,
		DescuentoMotivo: c.motivo,
	}
	for _, it := range c.items {
		ni := pedidos.NuevoItem{ProductoID: it.productoID, Tamano: it.tamano, Cantidad: it.cantidad}
		for _, e := range it.extras {
			ni.Extras = append(ni.Extras, pedidos.NuevoExtra{ExtraID: e, Cantidad: 1})
		}
		n.Items = append(n.Items, ni)
	}
	if c.descTipo != "" && c.descValor != "" {
		v, err := strconv.ParseFloat(c.descValor, 64)
		if err != nil || v < 0 {
			return n, "El descuento tiene que ser un número"
		}
		if c.descTipo == "pct" {
			if v > 100 {
				return n, "El descuento tiene que ser de 0 a 100%"
			}
			n.DescuentoPct = int(v)
		} else {
			n.DescuentoFijo = pedidos.Centavos(v * 100)
		}
	}
	if n.DescuentoPct == 0 && n.DescuentoFijo == 0 {
		n.DescuentoMotivo = ""
	}
	return n, ""
}

// mensajePedido saca el "pedido inválido: " del error para mostrarlo.
func mensajePedido(err error) string {
	msg := strings.TrimPrefix(err.Error(), pedidos.ErrPedidoInvalido.Error()+": ")
	return strings.ToUpper(msg[:1]) + msg[1:]
}

// vistaCarga arma lo que muestra el modal: los productos de la categoría, cada renglón con sus
// extras y las cuentas (pedidos.CalcularPresupuesto, con los precios de la base).
func (a *API) vistaCarga(ctx context.Context, c carga, msg string) (views.CargaPedido, error) {
	v := views.CargaPedido{
		Cliente: c.cliente, Telefono: c.telefono, Direccion: c.direccion,
		Origen: c.origen, Entrega: c.entrega, Pago: c.pago,
		DescTipo: c.descTipo, DescValor: c.descValor, Motivo: c.motivo,
		Categoria: c.categoria, Categorias: categoriasCarga, Error: msg,
	}
	productos, err := a.q.ListProductos(ctx)
	if err != nil {
		return v, err
	}
	extras, err := a.q.ListExtras(ctx)
	if err != nil {
		return v, err
	}
	porID := map[int32]sqlc.Producto{}
	for _, p := range productos {
		porID[p.ID] = p
		if p.Categoria != c.categoria {
			continue
		}
		pc := views.ProductoCarga{ID: p.ID, Nombre: p.Nombre, Simple: p.Precio, Agotado: !p.Disponible}
		if p.PrecioDoble.Valid {
			pc.Doble = p.PrecioDoble.String
		}
		v.Productos = append(v.Productos, pc)
	}

	n, msgDesc := nuevoPedidoDe(c)
	if v.Error == "" {
		v.Error = msgDesc
	}
	pres, err := pedidos.CalcularPresupuesto(ctx, a.db, n)
	if errors.Is(err, pedidos.ErrPedidoInvalido) {
		// Por ejemplo, un extra que alguien apagó mientras se cargaba: se avisa y se ve sin cuentas.
		v.Error = mensajePedido(err)
	} else if err != nil {
		return v, err
	}

	for i, it := range c.items {
		ic := views.ItemCarga{ProductoID: it.productoID, Tamano: it.tamano, Cantidad: it.cantidad, ExtrasIDs: idsTexto(it.extras)}
		if i < len(pres.Lineas) {
			l := pres.Lineas[i]
			ic.Nombre = l.Nombre
			if l.Tamano == pedidos.TamanoDoble {
				ic.Nombre += " (doble)"
			}
			ic.Detalle = formatoPrecio(l.Unidad) + " c/u"
			if len(l.Extras) > 0 {
				ic.Detalle = "+ " + strings.ToLower(strings.Join(l.Extras, ", ")) + " · " + ic.Detalle
			}
			ic.Total = l.Total.Texto()
		}
		if p, ok := porID[it.productoID]; ok && llevaExtras(p.Categoria) {
			for _, e := range extras {
				if e.Disponible {
					ic.Extras = append(ic.Extras, views.ExtraCarga{ID: e.ID, Nombre: e.Nombre, Precio: e.Precio, Activo: contiene(it.extras, e.ID)})
				}
			}
		}
		v.Items = append(v.Items, ic)
	}

	cu := pres.Cuentas
	v.Subtotal, v.Total = cu.Subtotal.Texto(), cu.Total.Texto()
	if cu.Descuento > 0 {
		v.Descuento = cu.Descuento.Texto()
		v.DescuentoTexto = "Descuento"
		if n.DescuentoPct > 0 {
			v.DescuentoTexto = fmt.Sprintf("Descuento %d%%", n.DescuentoPct)
		}
		if n.DescuentoMotivo != "" {
			v.DescuentoTexto += " (" + n.DescuentoMotivo + ")"
		}
	}
	if cu.Envio > 0 {
		v.Envio = cu.Envio.Texto()
	}
	return v, nil
}

// idsTexto pasa los ids de los extras al input oculto: "1,3".
func idsTexto(ids []int32) string {
	partes := make([]string, len(ids))
	for i, id := range ids {
		partes[i] = strconv.Itoa(int(id))
	}
	return strings.Join(partes, ",")
}

func contiene(ids []int32, id int32) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// formatoPrecio: 1750000 centavos → "$17.500", como se ven los precios en el panel.
func formatoPrecio(c pedidos.Centavos) string {
	return views.Precio(c.Texto())
}
