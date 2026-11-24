package pedidos

import "time"

// Los rangos rápidos del filtro de Pedidos. Vacío es un rango desde/hasta elegido a mano.
const (
	RangoHoy    = "hoy"
	RangoAyer   = "ayer"
	RangoSemana = "semana"
	RangoMes    = "mes"
)

// Con más de estos días, el gráfico va por semana (una barra cada 7 días).
const maxBarrasPorDia = 31

// VentaDia es una fila de la consulta VentasPorDia: lo vendido un día con una forma de pago.
type VentaDia struct {
	Dia     time.Time // solo importa la fecha (año, mes, día)
	Pago    string
	Pedidos int
	Total   Centavos
}

// Periodo es lo vendido en un período, con el desglose por forma de pago.
type Periodo struct {
	Pedidos       int
	Total         Centavos
	Efectivo      Centavos
	Transferencia Centavos
}

func (p *Periodo) sumar(v VentaDia) {
	p.Pedidos += v.Pedidos
	p.Total += v.Total
	if v.Pago == PagoTransferencia {
		p.Transferencia += v.Total
	} else {
		p.Efectivo += v.Total
	}
}

// PeriodoPedidos es el período que se está mirando en Pedidos: los días [Desde, Hasta) en la hora
// del local (medianoche a medianoche) y qué rango rápido es ("" si es un desde/hasta).
type PeriodoPedidos struct {
	Desde, Hasta time.Time
	Rango        string
}

// DiaGrafico es una barra del gráfico: un día, o una semana si el período es largo.
type DiaGrafico struct {
	Fecha  time.Time // el día (o el primero de la semana)
	Total  Centavos
	Alto   int  // porcentaje de la barra más alta (0 a 100)
	EsHoy  bool // es hoy (o la semana que incluye hoy)
	Futuro bool // todavía no llegó: se muestra "—"
}

// Resumen es todo lo que muestran las tarjetas y el gráfico de Pedidos.
type Resumen struct {
	Actual   Periodo
	Anterior Periodo  // el período con el que se compara (ver Comparacion)
	Ticket   Centavos // promedio por pedido del período (0 si no hubo pedidos)
	// VsAnterior es la variación contra el período anterior, en %. HayVs es false si en el
	// anterior no se vendió nada (no hay contra qué comparar). VsTexto: "vs. ayer", …
	VsAnterior float64
	HayVs      bool
	VsTexto    string

	// El gráfico: sus días [GraficoDesde, GraficoHasta), las barras y su total.
	GraficoDesde, GraficoHasta time.Time
	PorSemana                  bool
	Barras                     []DiaGrafico
	GraficoTotal               Centavos
}

// Comparacion dice contra qué se compara el período: el anterior, cortado en el mismo punto si el
// actual todavía está en curso (esta semana se compara del lunes a hoy contra del lunes al mismo
// día de la semana pasada; este mes, del 1 a hoy contra del 1 al mismo día del mes pasado).
func Comparacion(p PeriodoPedidos, ahora time.Time) (desde, hasta time.Time, texto string) {
	hoy := inicioDia(ahora.In(Zona))
	fin := p.Hasta // hasta dónde llegó el período actual
	if manana := hoy.AddDate(0, 0, 1); manana.Before(fin) {
		fin = manana
	}
	if fin.Before(p.Desde) {
		fin = p.Desde
	}
	dias := diasEntre(p.Desde, fin)

	switch p.Rango {
	case RangoHoy:
		return p.Desde.AddDate(0, 0, -1), p.Desde, "vs. ayer"
	case RangoAyer:
		return p.Desde.AddDate(0, 0, -1), p.Desde, "vs. el día anterior"
	case RangoSemana:
		return p.Desde.AddDate(0, 0, -7), p.Desde.AddDate(0, 0, dias-7), "vs. semana pasada"
	case RangoMes:
		desde = p.Desde.AddDate(0, -1, 0)
		hasta = desde.AddDate(0, 0, dias)
		if hasta.After(p.Desde) { // el mes pasado era más corto
			hasta = p.Desde
		}
		return desde, hasta, "vs. mes pasado"
	}
	return p.Desde.AddDate(0, 0, -dias), p.Desde, "vs. período anterior"
}

// rangoGrafico son los días del gráfico: la semana (lunes a domingo) si se mira un solo día, y si
// no, los del período.
func rangoGrafico(p PeriodoPedidos) (desde, hasta time.Time) {
	if p.Rango == RangoHoy || p.Rango == RangoAyer {
		lunes := LunesDe(p.Desde)
		return lunes, lunes.AddDate(0, 0, 7)
	}
	return p.Desde, p.Hasta
}

// RangoConsulta es lo que hay que pedirle a VentasPorDia para armar el resumen: el período, el
// anterior y los días del gráfico, en una sola consulta.
func RangoConsulta(p PeriodoPedidos, ahora time.Time) (desde, hasta time.Time) {
	antDesde, _, _ := Comparacion(p, ahora)
	gDesde, gHasta := rangoGrafico(p)
	desde, hasta = p.Desde, p.Hasta
	for _, d := range []time.Time{antDesde, gDesde} {
		if d.Before(desde) {
			desde = d
		}
	}
	if gHasta.After(hasta) {
		hasta = gHasta
	}
	return desde, hasta
}

// LunesDe devuelve el lunes de la semana de ese día (las semanas van de lunes a domingo).
func LunesDe(dia time.Time) time.Time {
	return inicioDia(dia).AddDate(0, 0, -((int(dia.Weekday()) + 6) % 7))
}

// Metricas arma el resumen del período con las ventas por día (las de RangoConsulta).
func Metricas(ventas []VentaDia, p PeriodoPedidos, ahora time.Time) Resumen {
	hoy := inicioDia(ahora.In(Zona))
	antDesde, antHasta, vsTexto := Comparacion(p, ahora)
	r := Resumen{VsTexto: vsTexto}
	r.GraficoDesde, r.GraficoHasta = rangoGrafico(p)

	porDia := map[string]Centavos{}
	for _, v := range ventas {
		dia := fechaDe(v.Dia) // fechas AAAA-MM-DD: el orden de los textos es el de las fechas
		if entre(dia, p.Desde, p.Hasta) {
			r.Actual.sumar(v)
		}
		if entre(dia, antDesde, antHasta) {
			r.Anterior.sumar(v)
		}
		porDia[dia] += v.Total
	}
	if r.Actual.Pedidos > 0 {
		r.Ticket = r.Actual.Total / Centavos(r.Actual.Pedidos)
	}
	if r.Anterior.Total > 0 {
		r.HayVs = true
		r.VsAnterior = float64(r.Actual.Total-r.Anterior.Total) * 100 / float64(r.Anterior.Total)
	}

	// Las barras: una por día, o una cada 7 días si el período es largo.
	paso := 1
	if diasEntre(r.GraficoDesde, r.GraficoHasta) > maxBarrasPorDia {
		paso, r.PorSemana = 7, true
	}
	var maximo Centavos
	for inicio := r.GraficoDesde; inicio.Before(r.GraficoHasta); inicio = inicio.AddDate(0, 0, paso) {
		fin := inicio.AddDate(0, 0, paso)
		if fin.After(r.GraficoHasta) {
			fin = r.GraficoHasta
		}
		b := DiaGrafico{Fecha: inicio, EsHoy: !hoy.Before(inicio) && hoy.Before(fin), Futuro: inicio.After(hoy)}
		for d := inicio; d.Before(fin); d = d.AddDate(0, 0, 1) {
			b.Total += porDia[fechaDe(d)]
		}
		r.GraficoTotal += b.Total
		if b.Total > maximo {
			maximo = b.Total
		}
		r.Barras = append(r.Barras, b)
	}
	for i := range r.Barras {
		if maximo > 0 {
			r.Barras[i].Alto = int(r.Barras[i].Total * 100 / maximo)
		}
	}
	return r
}

// entre dice si el día (AAAA-MM-DD) cae en [desde, hasta).
func entre(dia string, desde, hasta time.Time) bool {
	return dia >= fechaDe(desde) && dia < fechaDe(hasta)
}

// diasEntre cuenta los días de [desde, hasta) contando fechas (un día con cambio de horario no
// tiene 24 h, así que no se puede dividir la duración).
func diasEntre(desde, hasta time.Time) int {
	n := 0
	for d := desde; d.Before(hasta); d = d.AddDate(0, 0, 1) {
		n++
	}
	return n
}

func inicioDia(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// fechaDe compara días por la fecha escrita (AAAA-MM-DD): la base devuelve el día como medianoche
// UTC, y acá los días se cuentan en la hora del local.
func fechaDe(t time.Time) string {
	return t.Format("2006-01-02")
}
