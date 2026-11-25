package handlers

import (
	"context"
	"database/sql"
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

	"github.com/a-h/templ"
)

// Sección Pedidos del panel: la lista con sus filtros y el detalle de cada pedido, con las
// acciones de su estado. Las fechas se cuentan con la hora de la Argentina (pedidos.Zona).

const (
	rangoHoy    = pedidos.RangoHoy
	rangoAyer   = pedidos.RangoAyer
	rangoSemana = pedidos.RangoSemana
	rangoMes    = pedidos.RangoMes
	formatoDia  = "2006-01-02" // el de los <input type="date">
)

var (
	diasSemana = [7]string{"Domingo", "Lunes", "Martes", "Miércoles", "Jueves", "Viernes", "Sábado"}
	// Las abreviaturas van escritas y no recortadas: "Sábado"[:3] corta la "á" al medio (los
	// strings de Go se recortan por bytes, no por letras).
	diasCortos = [7]string{"Dom", "Lun", "Mar", "Mié", "Jue", "Vie", "Sáb"}
	meses      = [12]string{"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"}
)

// filtroPedidos es el filtro ya resuelto: lo que se muestra y el rango [desde, hasta) a consultar.
type filtroPedidos struct {
	vista        views.FiltroPedidos
	desde, hasta time.Time
	variosDias   bool
}

// Pedidos: GET /admin/pedidos (?rango=hoy|ayer|semana|mes, o ?desde=…&hasta=…, y ?estado=…)
// Con htmx (un filtro) devuelve solo el panel; sin htmx, la página.
func (a *API) Pedidos(w http.ResponseWriter, r *http.Request) {
	f, err := a.armarFiltro(r.Context(), r.URL.Query(), time.Now())
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
	if esHTMX(r) {
		// Las tarjetas y el gráfico son del mismo período: van por OOB junto con la lista.
		views.PedidosPanel(f.vista, filas, false).Render(r.Context(), w)
		views.Metricas(m, true).Render(r.Context(), w)
		return
	}
	views.PedidosPage(fechaLarga(time.Now()), m, f.vista, filas, nil).Render(r.Context(), w)
}

// DetallePedido: GET /admin/pedidos/{id}
// El modal del pedido. Al abrirlo queda visto (para el aviso de pedido nuevo). Sin htmx, la página
// de hoy con el detalle abierto.
func (a *API) DetallePedido(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, "id inválido", http.StatusBadRequest)
		return
	}
	if err := a.q.MarcarPedidoVisto(r.Context(), id); err != nil {
		http.Error(w, "No se pudo cargar el pedido", http.StatusInternalServerError)
		return
	}
	// Ya está visto: el aviso que va con la respuesta tiene que reflejarlo.
	if aviso, err := a.avisoPedido(r.Context()); err == nil {
		r = r.WithContext(views.ConAviso(r.Context(), aviso))
	}
	a.responderDetalle(w, r, id, "")
}

// ConfirmarBajaPedido: GET /admin/pedidos/{id}/baja?accion=rechazar|cancelar
// Pregunta antes de rechazar o cancelar.
func (a *API) ConfirmarBajaPedido(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, "id inválido", http.StatusBadRequest)
		return
	}
	accion := r.URL.Query().Get("accion")
	if accion != pedidos.AccionRechazar && accion != pedidos.AccionCancelar {
		http.Error(w, "acción inválida", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.ModalConfirmarBaja(id, accion).Render(r.Context(), w)
}

// AccionPedido: POST /admin/pedidos/{id}/{accion} (confirmar, rechazar, en-camino, entregado,
// cancelar). Devuelve el detalle actualizado y, por OOB, la etiqueta del estado en la lista. Si el
// pedido ya había cambiado (lo tocó otra persona), el detalle vuelve con el aviso y su estado real.
func (a *API) AccionPedido(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, "id inválido", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	demora, _ := strconv.Atoi(r.FormValue("demora"))
	_, err = pedidos.CambiarEstado(r.Context(), a.db, id, r.PathValue("accion"), demora)
	aviso := ""
	switch {
	case errors.Is(err, sql.ErrNoRows):
		noEncontrado(w, "pedido")
		return
	case errors.Is(err, pedidos.ErrEstadoCambio):
		aviso = "Este pedido ya cambió de estado. Mirá cómo está ahora."
	case errors.Is(err, pedidos.ErrAccionInvalida):
		http.Error(w, "acción inválida para este pedido", http.StatusBadRequest)
		return
	case err != nil:
		http.Error(w, "No se pudo cambiar el estado", http.StatusInternalServerError)
		return
	}
	if !esHTMX(r) {
		http.Redirect(w, r, fmt.Sprintf("/admin/pedidos/%d", id), http.StatusSeeOther)
		return
	}
	a.responderDetalle(w, r, id, aviso)
}

// AvisoPedido: GET /admin/pedidos/aviso?actual=17-2&titulo=Productos
// Lo consulta el panel cada 10 segundos. Si el aviso que se ve (actual) sigue siendo el mismo,
// 204: htmx no toca nada y la alarma sigue sonando sin cortarse. Si cambió, el aviso nuevo (con el
// banner o vacío) y el título de la pestaña.
func (a *API) AvisoPedido(w http.ResponseWriter, r *http.Request) {
	aviso, ok := views.AvisoDe(r.Context()) // lo calculó el middleware
	if !ok {
		http.Error(w, "No se pudo consultar los pedidos", http.StatusInternalServerError)
		return
	}
	q := r.URL.Query()
	if aviso.Clave() == q.Get("actual") {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	titulo := strings.TrimSpace(q.Get("titulo"))
	if titulo == "" {
		titulo = "Panel"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.Aviso(aviso, titulo, true, false).Render(r.Context(), w)
}

// ---------- helpers ----------

// avisoPedido busca el pendiente sin ver más viejo y cuántos hay (Cantidad 0 si no hay ninguno).
func (a *API) avisoPedido(ctx context.Context) (views.AvisoPedido, error) {
	row, err := a.q.AvisoPedidos(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return views.AvisoPedido{}, nil
	}
	if err != nil {
		return views.AvisoPedido{}, err
	}
	return views.AvisoPedido{ID: row.ID, Cliente: row.Cliente, Total: row.Total, Cantidad: int(row.Cantidad)}, nil
}

// responderDetalle arma el detalle y lo devuelve: con htmx el modal (más la etiqueta de la lista
// por OOB), sin htmx la página de hoy con el modal abierto.
func (a *API) responderDetalle(w http.ResponseWriter, r *http.Request, id int32, aviso string) {
	d, err := a.detallePedido(r, id)
	if errors.Is(err, sql.ErrNoRows) {
		noEncontrado(w, "pedido")
		return
	}
	if err != nil {
		http.Error(w, "No se pudo cargar el pedido", http.StatusInternalServerError)
		return
	}
	d.Aviso = aviso
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if esHTMX(r) {
		views.ModalPedido(d).Render(r.Context(), w)
		views.EtiquetaEstado(d.ID, d.Estado, true).Render(r.Context(), w)
		if aviso, ok := views.AvisoDe(r.Context()); ok {
			views.Aviso(aviso, "Pedidos", true, true).Render(r.Context(), w)
		}
		return
	}
	a.paginaPedidosCon(w, r, views.ModalPedido(d))
}

// paginaPedidosCon devuelve la página de Pedidos de hoy con ese modal abierto (para entrar sin
// htmx: un link directo o F5 sobre el detalle o sobre "Cargar pedido").
func (a *API) paginaPedidosCon(w http.ResponseWriter, r *http.Request, modal templ.Component) {
	f, err := a.armarFiltro(r.Context(), url.Values{}, time.Now())
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
	views.PedidosPage(fechaLarga(time.Now()), m, f.vista, filas, modal).Render(r.Context(), w)
}

// armarFiltro es leerFiltroPedidos más lo que sale de la base: cuántos pendientes hay en el período,
// para el chip "Pendientes (2)".
func (a *API) armarFiltro(ctx context.Context, q url.Values, ahora time.Time) (filtroPedidos, error) {
	f := leerFiltroPedidos(q, ahora)
	n, err := a.q.CountPendientesRango(ctx, sqlc.CountPendientesRangoParams{Desde: f.desde, Hasta: f.hasta})
	if err != nil {
		return f, err
	}
	if n > 0 {
		for i, c := range f.vista.ChipsEstado {
			if c.Label == "Pendientes" {
				f.vista.ChipsEstado[i].Label = fmt.Sprintf("Pendientes (%d)", n)
			}
		}
	}
	return f, nil
}

// leerFiltroPedidos interpreta la URL. Un rango desde/hasta válido gana; si no, el rango rápido
// (hoy por defecto). Un estado desconocido es "todos".
func leerFiltroPedidos(q url.Values, ahora time.Time) filtroPedidos {
	hoy := inicioDelDia(ahora.In(pedidos.Zona))
	estado := q.Get("estado")
	switch estado {
	case pedidos.EstadoPendiente, pedidos.EstadoConfirmado, pedidos.EstadoEntregado, pedidos.EstadoCancelado:
	default:
		estado = ""
	}

	var f filtroPedidos
	desde, errD := time.ParseInLocation(formatoDia, q.Get("desde"), pedidos.Zona)
	hasta, errH := time.ParseInLocation(formatoDia, q.Get("hasta"), pedidos.Zona)
	if errD == nil && errH == nil && !hasta.Before(desde) {
		f.desde, f.hasta = desde, hasta.AddDate(0, 0, 1)
		f.vista.Titulo = fmt.Sprintf("PEDIDOS DEL %s AL %s", desde.Format("02/01"), hasta.Format("02/01"))
	} else {
		f.vista.Rango = q.Get("rango")
		switch f.vista.Rango {
		case rangoAyer:
			f.desde, f.hasta = hoy.AddDate(0, 0, -1), hoy
			f.vista.Titulo = "PEDIDOS DE AYER"
		case rangoSemana:
			// De lunes a domingo, como el gráfico de ventas.
			lunes := pedidos.LunesDe(hoy)
			f.desde, f.hasta = lunes, lunes.AddDate(0, 0, 7)
			f.vista.Titulo = "PEDIDOS DE ESTA SEMANA"
		case rangoMes:
			f.desde = time.Date(hoy.Year(), hoy.Month(), 1, 0, 0, 0, 0, pedidos.Zona)
			f.hasta = f.desde.AddDate(0, 1, 0)
			f.vista.Titulo = "PEDIDOS DE ESTE MES"
		default:
			f.vista.Rango = rangoHoy
			f.desde, f.hasta = hoy, hoy.AddDate(0, 0, 1)
			f.vista.Titulo = "PEDIDOS DE HOY"
		}
	}
	f.vista.Desde = f.desde.Format(formatoDia)
	f.vista.Hasta = f.hasta.AddDate(0, 0, -1).Format(formatoDia)
	f.vista.Estado = estado
	f.variosDias = f.hasta.Sub(f.desde) > 25*time.Hour // un día puede tener 25 h con cambio de horario

	for _, c := range []struct{ rango, label string }{{rangoHoy, "Hoy"}, {rangoAyer, "Ayer"}, {rangoSemana, "Esta semana"}, {rangoMes, "Este mes"}} {
		f.vista.ChipsRango = append(f.vista.ChipsRango, views.Chip{
			Label: c.label, URL: urlPedidos(url.Values{"rango": {c.rango}}, estado), Activo: f.vista.Rango == c.rango,
		})
	}
	// Los chips de estado conservan el rango elegido.
	rango := url.Values{"rango": {f.vista.Rango}}
	if f.vista.Rango == "" {
		rango = url.Values{"desde": {f.vista.Desde}, "hasta": {f.vista.Hasta}}
	}
	for _, c := range []struct{ estado, label string }{{"", "Todos"}, {pedidos.EstadoPendiente, "Pendientes"}, {pedidos.EstadoConfirmado, "Confirmados"}, {pedidos.EstadoEntregado, "Entregados"}, {pedidos.EstadoCancelado, "Cancelados"}} {
		f.vista.ChipsEstado = append(f.vista.ChipsEstado, views.Chip{
			Label: c.label, URL: urlPedidos(rango, c.estado), Activo: estado == c.estado,
		})
	}
	return f
}

func urlPedidos(v url.Values, estado string) string {
	if estado != "" {
		v.Set("estado", estado)
	}
	return "/admin/pedidos?" + v.Encode()
}

// metricasPedidos arma las tarjetas y el gráfico del período del filtro (el mismo de la lista):
// ventas, pedidos y ticket promedio, con la comparación contra el período anterior. No miran el
// filtro de estado: las ventas nunca cuentan los cancelados.
func (a *API) metricasPedidos(ctx context.Context, f filtroPedidos, ahora time.Time) (views.MetricasPedidos, error) {
	periodo := pedidos.PeriodoPedidos{Desde: f.desde, Hasta: f.hasta, Rango: f.vista.Rango}
	desde, hasta := pedidos.RangoConsulta(periodo, ahora)
	filas, err := a.q.VentasPorDia(ctx, sqlc.VentasPorDiaParams{Zona: pedidos.NombreZona, Desde: desde, Hasta: hasta})
	if err != nil {
		return views.MetricasPedidos{}, err
	}
	ventas := make([]pedidos.VentaDia, len(filas))
	for i, fila := range filas {
		total, err := pedidos.CentavosDe(fila.Total)
		if err != nil {
			return views.MetricasPedidos{}, err
		}
		ventas[i] = pedidos.VentaDia{Dia: fila.Dia, Pago: fila.Pago, Pedidos: int(fila.Pedidos), Total: total}
	}
	cancelados, err := a.q.CountCanceladosRango(ctx, sqlc.CountCanceladosRangoParams{Desde: f.desde, Hasta: f.hasta})
	if err != nil {
		return views.MetricasPedidos{}, err
	}
	r := pedidos.Metricas(ventas, periodo, ahora)

	m := views.MetricasPedidos{
		// "PEDIDOS DE ESTE MES" → "VENTAS DE ESTE MES": el mismo período que la lista.
		VentasTitulo:        strings.Replace(f.vista.Titulo, "PEDIDOS", "VENTAS", 1),
		VentasTotal:         r.Actual.Total.Texto(),
		VentasDetalle:       conteoPedidos(r.Actual.Pedidos),
		VentasEfectivo:      r.Actual.Efectivo.Texto(),
		VentasTransferencia: r.Actual.Transferencia.Texto(),
		PedidosCantidad:     strconv.Itoa(r.Actual.Pedidos),
		PedidosDetalle:      textoCancelados(int(cancelados)),
		TicketDetalle:       "Por pedido, " + periodoCorto(f.vista.Rango),
		GraficoTitulo:       "VENTAS POR DÍA",
		GraficoSub:          tituloGrafico(f.vista.Rango, r.GraficoDesde, r.GraficoHasta),
		TotalTitulo:         totalTitulo(f.vista.Rango),
		GraficoTotal:        r.GraficoTotal.Texto(),
	}
	if r.HayVs {
		m.VentasDetalle += " · " + strings.Replace(fmt.Sprintf("%+.1f%% %s", r.VsAnterior, r.VsTexto), ".", ",", 1)
	}
	if r.Actual.Pedidos > 0 {
		m.Ticket = r.Ticket.Texto()
	}
	if r.PorSemana {
		m.GraficoTitulo = "VENTAS POR SEMANA"
	}
	semana := len(r.Barras) == 7 && !r.PorSemana && r.GraficoDesde.Weekday() == time.Monday
	for _, d := range r.Barras {
		b := views.BarraVenta{Alto: d.Alto, EsHoy: d.EsHoy}
		switch {
		case r.PorSemana:
			b.Dia = d.Fecha.Format("02/01")
		case semana:
			b.Dia = diasCortos[d.Fecha.Weekday()]
			if d.EsHoy {
				b.Dia += " (hoy)"
			}
		default:
			b.Dia = strconv.Itoa(d.Fecha.Day())
		}
		if !d.Futuro {
			b.Valor = d.Total.Texto()
		}
		m.Barras = append(m.Barras, b)
	}
	return m, nil
}

// textoCancelados: "2 cancelados", "1 cancelado" o "Ninguno cancelado".
func textoCancelados(n int) string {
	switch n {
	case 0:
		return "Ninguno cancelado"
	case 1:
		return "1 cancelado"
	}
	return fmt.Sprintf("%d cancelados", n)
}

// periodoCorto es el período para "Por pedido, …".
func periodoCorto(rango string) string {
	switch rango {
	case rangoHoy:
		return "hoy"
	case rangoAyer:
		return "ayer"
	case rangoSemana:
		return "esta semana"
	case rangoMes:
		return "este mes"
	}
	return "en el período"
}

func totalTitulo(rango string) string {
	switch rango {
	case rangoHoy, rangoAyer, rangoSemana:
		return "Total de la semana"
	case rangoMes:
		return "Total del mes"
	}
	return "Total del período"
}

// tituloGrafico dice qué días muestra el gráfico: "Semana del 16 al 22 de noviembre",
// "Noviembre de 2026" o "Del 13/11 al 22/11", siempre aclarando que no cuenta los cancelados.
func tituloGrafico(rango string, desde, hasta time.Time) string {
	ultimo := hasta.AddDate(0, 0, -1)
	var titulo string
	switch rango {
	case rangoHoy, rangoAyer, rangoSemana:
		titulo = semanaTitulo(desde, ultimo)
	case rangoMes:
		mes := meses[desde.Month()-1]
		titulo = fmt.Sprintf("%s%s de %d", strings.ToUpper(mes[:1]), mes[1:], desde.Year())
	default:
		titulo = fmt.Sprintf("Del %s al %s", desde.Format("02/01"), ultimo.Format("02/01"))
	}
	return titulo + " · no incluye pedidos cancelados"
}

func conteoPedidos(n int) string {
	if n == 1 {
		return "1 pedido"
	}
	return fmt.Sprintf("%d pedidos", n)
}

// semanaTitulo: "Semana del 16 al 22 de noviembre" (o "del 28 de noviembre al 4 de diciembre").
func semanaTitulo(lunes, domingo time.Time) string {
	if lunes.Month() == domingo.Month() {
		return fmt.Sprintf("Semana del %d al %d de %s", lunes.Day(), domingo.Day(), meses[domingo.Month()-1])
	}
	return fmt.Sprintf("Semana del %d de %s al %d de %s", lunes.Day(), meses[lunes.Month()-1], domingo.Day(), meses[domingo.Month()-1])
}

// filasPedidos trae los pedidos del filtro con el resumen de sus ítems (dos consultas en total).
func (a *API) filasPedidos(ctx context.Context, f filtroPedidos) ([]views.FilaPedido, error) {
	lista, err := a.q.ListPedidos(ctx, sqlc.ListPedidosParams{Desde: f.desde, Hasta: f.hasta, Estado: f.vista.Estado})
	if err != nil || len(lista) == 0 {
		return nil, err
	}
	ids := make([]int32, len(lista))
	for i, p := range lista {
		ids[i] = p.ID
	}
	items, err := a.q.ListResumenItems(ctx, ids)
	if err != nil {
		return nil, err
	}
	resumen := map[int32][]string{}
	for _, it := range items {
		resumen[it.PedidoID] = append(resumen[it.PedidoID], textoItemPedido(int(it.Cantidad), it.Nombre, it.Tamano, it.Extras, it.Aclaraciones, it.Detalle))
	}
	filas := make([]views.FilaPedido, len(lista))
	for i, p := range lista {
		hora := p.CreadoEn.In(pedidos.Zona)
		textoHora := hora.Format("15:04")
		if f.variosDias {
			textoHora = diasCortos[hora.Weekday()] + " " + textoHora
		}
		filas[i] = views.FilaPedido{
			ID: p.ID, Hora: textoHora, Cliente: p.Cliente, Resumen: strings.Join(resumen[p.ID], ", "),
			Entrega: textoEntrega(p.Entrega), Pago: textoPago(p.Pago), Total: p.Total, Estado: p.Estado,
		}
	}
	return filas, nil
}

// detallePedido arma todo lo que muestra el modal de un pedido.
func (a *API) detallePedido(r *http.Request, id int32) (views.DetallePedido, error) {
	ctx := r.Context()
	p, err := a.q.GetPedido(ctx, id)
	if err != nil {
		return views.DetallePedido{}, err
	}
	items, err := a.q.ListItemsPedido(ctx, id)
	if err != nil {
		return views.DetallePedido{}, err
	}
	extras, err := a.q.ListExtrasItemsPedido(ctx, id)
	if err != nil {
		return views.DetallePedido{}, err
	}
	eventos, err := a.q.ListEventosPedido(ctx, id)
	if err != nil {
		return views.DetallePedido{}, err
	}

	d := views.DetallePedido{
		ID:        p.ID,
		Subtitulo: momento(p.CreadoEn, time.Now()) + " · " + textoOrigen(p.Origen),
		Estado:    p.Estado,
		Cliente:   p.Cliente,
		Telefono:  p.Telefono,
		Entrega:   textoEntrega(p.Entrega),
		Pago:      textoPago(p.Pago),
		Notas:     p.Notas,
		Total:     p.Total,
		Acciones:  pedidos.AccionesPara(p.Estado, p.Entrega),
	}
	if p.Entrega == pedidos.EntregaDelivery {
		d.Entrega += " · " + p.Direccion
	}

	// Cada línea: cantidad × (precio + extras de cada unidad), con los montos copiados al pedido.
	extrasDe := map[int32][]sqlc.ItemPedidoExtra{}
	for _, e := range extras {
		extrasDe[e.ItemPedidoID] = append(extrasDe[e.ItemPedidoID], e)
	}
	for _, it := range items {
		precio, _ := pedidos.CentavosDe(it.PrecioUnitario)
		linea := pedidos.Linea{Cantidad: int(it.Cantidad), Precio: precio}
		var nombres []string
		for _, e := range extrasDe[it.ID] {
			pe, _ := pedidos.CentavosDe(e.PrecioUnitario)
			linea.Extras = append(linea.Extras, pedidos.LineaExtra{Cantidad: int(e.Cantidad), Precio: pe})
			nombre := strings.ToLower(e.Nombre)
			if e.Cantidad > 1 {
				nombre = fmt.Sprintf("%dx %s", e.Cantidad, nombre)
			}
			nombres = append(nombres, nombre)
		}
		d.Lineas = append(d.Lineas, views.LineaPedido{
			Texto: textoItemPedido(int(it.Cantidad), it.Nombre, it.Tamano, strings.Join(nombres, ", "), it.Aclaraciones, it.Detalle),
			Total: linea.Total().Texto(),
		})
	}
	// Subtotal, descuento y envío solo si hay algo entre las líneas y el total.
	// El descuento puede ser en % ("Descuento 20%") o fijo ("Descuento").
	if p.Descuento != "0.00" || p.CostoEnvio != "0.00" {
		d.Subtotal = p.Subtotal
		if p.Descuento != "0.00" {
			d.Descuento = p.Descuento
			d.DescuentoTexto = "Descuento"
			if p.DescuentoPct > 0 {
				d.DescuentoTexto = fmt.Sprintf("Descuento %d%%", p.DescuentoPct)
			}
			if p.DescuentoMotivo != "" {
				d.DescuentoTexto += " (" + p.DescuentoMotivo + ")"
			}
		}
		if p.CostoEnvio != "0.00" {
			d.Envio = p.CostoEnvio
		}
	}
	for _, e := range eventos {
		d.Historial = append(d.Historial, views.EventoHistorial{Hora: e.CreadoEn.In(pedidos.Zona).Format("15:04"), Texto: textoEvento(e.Estado, e.Detalle)})
	}
	d.Link = linkSeguimiento(r, p.Token)
	d.WhatsApp = linkWhatsApp(p.Telefono, fmt.Sprintf("¡Hola %s! Podés seguir tu pedido #%d de Forest Burger acá: %s", p.Cliente, p.ID, d.Link))
	return d, nil
}

// textoItem: "2x Cheese Bacon (doble) + cheddar". Las bebidas y acompañamientos (simple) no dicen
// el tamaño.
func textoItem(cantidad int, nombre, tamano, extras, aclaraciones string) string {
	t := fmt.Sprintf("%dx %s", cantidad, nombre)
	if tamano == pedidos.TamanoDoble {
		t += " (doble)"
	}
	if extras != "" {
		t += " + " + strings.ToLower(extras)
	}
	if aclaraciones != "" {
		t += " · “" + aclaraciones + "”"
	}
	return t
}

// textoItemPedido es textoItem, salvo para una promo del cliente: "1x 2 Smash (promo): Classic
// (simple) + Cheddar · Big Forest (simple)" (sus extras ya están en el detalle).
func textoItemPedido(cantidad int, nombre, tamano, extras, aclaraciones, detalle string) string {
	if detalle != "" {
		return fmt.Sprintf("%dx %s (promo): %s", cantidad, nombre, detalle)
	}
	return textoItem(cantidad, nombre, tamano, extras, aclaraciones)
}

func textoEntrega(e string) string {
	if e == pedidos.EntregaDelivery {
		return "Delivery"
	}
	return "Retiro"
}

func textoPago(p string) string {
	if p == pedidos.PagoTransferencia {
		return "Transferencia"
	}
	return "Efectivo"
}

func textoOrigen(o string) string {
	switch o {
	case pedidos.OrigenWhatsapp:
		return "Vino por WhatsApp"
	case pedidos.OrigenTelefono:
		return "Vino por teléfono"
	case pedidos.OrigenLocal:
		return "Lo cargaron en el local"
	}
	return "Vino por la web"
}

// textoEvento es el renglón del historial: "Confirmado · 30 min", "Rechazado".
func textoEvento(estado, detalle string) string {
	switch estado {
	case pedidos.EventoRecibido:
		return "Recibido"
	case pedidos.EstadoConfirmado:
		if detalle != "" {
			return "Confirmado · " + detalle
		}
		return "Confirmado"
	case pedidos.EstadoEnCamino:
		return "En camino"
	case pedidos.EstadoListo:
		return "Listo para retirar"
	case pedidos.EstadoEntregado:
		return "Entregado"
	case pedidos.EstadoCancelado:
		if detalle == pedidos.DetalleRechazado {
			return "Rechazado"
		}
		return "Cancelado"
	}
	return estado
}

// momento: "Hoy 21:05", "Ayer 21:05" o "Vie 21/11 21:05".
func momento(t, ahora time.Time) string {
	t = t.In(pedidos.Zona)
	hoy := inicioDelDia(ahora.In(pedidos.Zona))
	hora := t.Format("15:04")
	switch dia := inicioDelDia(t); {
	case dia.Equal(hoy):
		return "Hoy " + hora
	case dia.Equal(hoy.AddDate(0, 0, -1)):
		return "Ayer " + hora
	default:
		return diasCortos[t.Weekday()] + " " + t.Format("02/01") + " " + hora
	}
}

// fechaLarga: "Sábado 22 de noviembre".
func fechaLarga(t time.Time) string {
	t = t.In(pedidos.Zona)
	return fmt.Sprintf("%s %d de %s", diasSemana[t.Weekday()], t.Day(), meses[t.Month()-1])
}

func inicioDelDia(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// linkSeguimiento es el link que se le pasa al cliente, con el mismo dominio por el que se entró
// al panel (localhost en desarrollo, el dominio real en producción).
func linkSeguimiento(r *http.Request, token string) string {
	esquema := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		esquema = "https"
	}
	return fmt.Sprintf("%s://%s/pedido/%s", esquema, r.Host, token)
}

// linkWhatsApp abre el chat con el cliente y el mensaje ya escrito (wa.me, sin JS). Los teléfonos
// se guardan con código de área sin el 54 ("2236001122"); para WhatsApp va 549 adelante. Sin
// teléfono, no hay link.
func linkWhatsApp(telefono, mensaje string) string {
	numero := separadoresTelefono.ReplaceAllString(telefono, "")
	if numero == "" {
		return ""
	}
	if !strings.HasPrefix(numero, "54") {
		numero = "549" + numero
	}
	return "https://wa.me/" + numero + "?text=" + url.QueryEscape(mensaje)
}
