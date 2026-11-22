package pedidos

import "time"

// VentaDia es una fila de la consulta VentasPorDia: lo vendido un día con una forma de pago.
type VentaDia struct {
	Dia     time.Time // solo importa la fecha (año, mes, día)
	Pago    string
	Pedidos int
	Total   Centavos
}

// Periodo es lo vendido en un día o en la semana, con el desglose por forma de pago.
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

// DiaGrafico es una barra del gráfico de la semana.
type DiaGrafico struct {
	Fecha  time.Time
	Total  Centavos
	Alto   int // porcentaje del día que más vendió (0 a 100)
	EsHoy  bool
	Futuro bool // todavía no llegó: se muestra "—"
}

// Resumen es todo lo que muestran las tarjetas y el gráfico de Pedidos.
type Resumen struct {
	Hoy, Ayer, Semana Periodo
	Ticket            Centavos // promedio por pedido de hoy (0 si hoy no hubo pedidos)
	// VsAyer es la variación de hoy contra ayer, en %. HayVsAyer es false si ayer no se vendió
	// nada (no hay contra qué comparar).
	VsAyer    float64
	HayVsAyer bool
	Lunes     time.Time
	Dias      [7]DiaGrafico // de lunes a domingo
}

// RangoMetricas es lo que hay que pedirle a la base para armar el resumen: desde el lunes de esta
// semana (o desde ayer, si hoy es lunes) hasta el lunes siguiente.
func RangoMetricas(ahora time.Time) (desde, hasta time.Time) {
	hoy := inicioDia(ahora.In(Zona))
	lunes := LunesDe(hoy)
	desde = lunes
	if ayer := hoy.AddDate(0, 0, -1); ayer.Before(lunes) {
		desde = ayer
	}
	return desde, lunes.AddDate(0, 0, 7)
}

// LunesDe devuelve el lunes de la semana de ese día (las semanas van de lunes a domingo).
func LunesDe(dia time.Time) time.Time {
	return inicioDia(dia).AddDate(0, 0, -((int(dia.Weekday()) + 6) % 7))
}

// Metricas arma el resumen con las ventas por día (las de RangoMetricas).
func Metricas(ventas []VentaDia, ahora time.Time) Resumen {
	hoy := inicioDia(ahora.In(Zona))
	ayer := hoy.AddDate(0, 0, -1)
	r := Resumen{Lunes: LunesDe(hoy)}

	porDia := map[string]Centavos{}
	lunes := fechaDe(r.Lunes)
	for _, v := range ventas {
		dia := fechaDe(v.Dia)
		switch dia {
		case fechaDe(hoy):
			r.Hoy.sumar(v)
		case fechaDe(ayer):
			r.Ayer.sumar(v)
		}
		if dia >= lunes { // fechas AAAA-MM-DD: el orden de los textos es el de las fechas
			r.Semana.sumar(v)
			porDia[dia] += v.Total
		}
	}

	if r.Hoy.Pedidos > 0 {
		r.Ticket = r.Hoy.Total / Centavos(r.Hoy.Pedidos)
	}
	if r.Ayer.Total > 0 {
		r.HayVsAyer = true
		r.VsAyer = float64(r.Hoy.Total-r.Ayer.Total) * 100 / float64(r.Ayer.Total)
	}

	var maximo Centavos
	for i := range r.Dias {
		fecha := r.Lunes.AddDate(0, 0, i)
		r.Dias[i] = DiaGrafico{Fecha: fecha, Total: porDia[fechaDe(fecha)], EsHoy: fecha.Equal(hoy), Futuro: fecha.After(hoy)}
		if r.Dias[i].Total > maximo {
			maximo = r.Dias[i].Total
		}
	}
	for i := range r.Dias {
		if maximo > 0 {
			r.Dias[i].Alto = int(r.Dias[i].Total * 100 / maximo)
		}
	}
	return r
}

func inicioDia(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// fechaDe compara días por la fecha escrita (AAAA-MM-DD): la base devuelve el día como medianoche
// UTC, y acá los días se cuentan en la hora del local.
func fechaDe(t time.Time) string {
	return t.Format("2006-01-02")
}
