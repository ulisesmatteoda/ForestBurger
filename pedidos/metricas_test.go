package pedidos

import (
	"math"
	"testing"
	"time"
)

// dia arma la fecha como la devuelve la base: medianoche UTC.
func dia(a int, m time.Month, d int) time.Time { return time.Date(a, m, d, 0, 0, 0, 0, time.UTC) }

// local es el comienzo de ese día en la hora del local (como arma los períodos el panel).
func local(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, Zona) }

func pesos(p int64) Centavos { return Centavos(p * 100) }

func venta(m time.Month, d int, pago string, pedidos int, total int64) VentaDia {
	return VentaDia{Dia: dia(2026, m, d), Pago: pago, Pedidos: pedidos, Total: pesos(total)}
}

// Sábado 21/11/2026: la semana va del lunes 16 al domingo 22.
var sabado = time.Date(2026, time.November, 21, 22, 0, 0, 0, Zona)

func periodoHoy() PeriodoPedidos {
	return PeriodoPedidos{Desde: local(11, 21), Hasta: local(11, 22), Rango: RangoHoy}
}

func TestMetricas_HoyContraAyer(t *testing.T) {
	ventas := []VentaDia{
		venta(11, 16, PagoEfectivo, 2, 30000),      // lunes
		venta(11, 20, PagoEfectivo, 3, 60000),      // ayer
		venta(11, 20, PagoTransferencia, 1, 20000), // ayer
		venta(11, 21, PagoEfectivo, 2, 50000),      // hoy
		venta(11, 21, PagoTransferencia, 2, 40000), // hoy
	}
	r := Metricas(ventas, periodoHoy(), sabado)

	if r.Actual.Total != pesos(90000) || r.Actual.Efectivo != pesos(50000) || r.Actual.Transferencia != pesos(40000) || r.Actual.Pedidos != 4 {
		t.Errorf("hoy = %+v", r.Actual)
	}
	if r.Ticket != pesos(22500) {
		t.Errorf("ticket = %d, quiero 22.500", r.Ticket/100)
	}
	// Hoy 90.000 contra ayer 80.000: +12,5%.
	if !r.HayVs || math.Abs(r.VsAnterior-12.5) > 0.001 || r.VsTexto != "vs. ayer" {
		t.Errorf("vs = %v (%v, %q), quiero +12,5%% vs. ayer", r.VsAnterior, r.HayVs, r.VsTexto)
	}
	// El gráfico de un día es su semana, con el total de la semana.
	if len(r.Barras) != 7 || r.GraficoTotal != pesos(200000) || r.PorSemana {
		t.Errorf("gráfico: %d barras, total %d", len(r.Barras), r.GraficoTotal/100)
	}
}

func TestMetricas_BarrasDeLaSemana(t *testing.T) {
	ventas := []VentaDia{venta(11, 16, PagoEfectivo, 1, 45000), venta(11, 21, PagoEfectivo, 4, 90000)}
	r := Metricas(ventas, periodoHoy(), sabado)

	if !r.GraficoDesde.Equal(local(11, 16)) {
		t.Fatalf("el gráfico arranca el %v; quiero el lunes 16", r.GraficoDesde)
	}
	lunes, martes, sab, dom := r.Barras[0], r.Barras[1], r.Barras[5], r.Barras[6]
	if lunes.Total != pesos(45000) || lunes.Alto != 50 || lunes.EsHoy || lunes.Futuro {
		t.Errorf("lunes = %+v; quiero 45.000 y medio alto", lunes)
	}
	if martes.Total != 0 || martes.Alto != 0 || martes.Futuro {
		t.Errorf("martes (sin ventas, ya pasó) = %+v", martes)
	}
	if !sab.EsHoy || sab.Alto != 100 {
		t.Errorf("sábado (hoy, el que más vendió) = %+v", sab)
	}
	if !dom.Futuro {
		t.Errorf("el domingo todavía no llegó: %+v", dom)
	}
}

func TestMetricas_AyerContraElDiaAnterior(t *testing.T) {
	ventas := []VentaDia{venta(11, 19, PagoEfectivo, 1, 40000), venta(11, 20, PagoEfectivo, 1, 50000)}
	r := Metricas(ventas, PeriodoPedidos{Desde: local(11, 20), Hasta: local(11, 21), Rango: RangoAyer}, sabado)
	if r.Actual.Total != pesos(50000) || r.Anterior.Total != pesos(40000) || r.VsTexto != "vs. el día anterior" {
		t.Errorf("ayer = %+v contra %+v (%q)", r.Actual, r.Anterior, r.VsTexto)
	}
}

// Esta semana (lunes 16 a sábado 21, hoy) contra lunes 9 a sábado 14: el domingo 15 no cuenta,
// porque esta semana todavía no llegó a su domingo.
func TestMetricas_SemanaEnCursoContraElMismoTramo(t *testing.T) {
	ventas := []VentaDia{
		venta(11, 9, PagoEfectivo, 1, 10000),
		venta(11, 14, PagoEfectivo, 1, 30000),
		venta(11, 15, PagoEfectivo, 1, 99000), // domingo pasado: fuera de la comparación
		venta(11, 17, PagoTransferencia, 2, 50000),
	}
	r := Metricas(ventas, PeriodoPedidos{Desde: local(11, 16), Hasta: local(11, 23), Rango: RangoSemana}, sabado)
	if r.Actual.Total != pesos(50000) || r.Anterior.Total != pesos(40000) || r.VsTexto != "vs. semana pasada" {
		t.Errorf("semana = %d contra %d (%q); quiero 50.000 contra 40.000", r.Actual.Total/100, r.Anterior.Total/100, r.VsTexto)
	}
	if len(r.Barras) != 7 {
		t.Errorf("la semana tiene %d barras", len(r.Barras))
	}
}

// Este mes (1 al 21 de noviembre) contra el 1 al 21 de octubre.
func TestMetricas_MesEnCursoContraElMismoTramoDelMesPasado(t *testing.T) {
	ventas := []VentaDia{
		venta(10, 1, PagoEfectivo, 1, 20000),
		venta(10, 21, PagoEfectivo, 1, 20000),
		venta(10, 25, PagoEfectivo, 1, 99000), // después del 21: fuera de la comparación
		venta(11, 3, PagoEfectivo, 3, 60000),
		venta(11, 21, PagoTransferencia, 1, 30000),
	}
	r := Metricas(ventas, PeriodoPedidos{Desde: local(11, 1), Hasta: local(12, 1), Rango: RangoMes}, sabado)
	if r.Actual.Total != pesos(90000) || r.Actual.Pedidos != 4 || r.Ticket != pesos(22500) {
		t.Errorf("mes = %+v, ticket %d", r.Actual, r.Ticket/100)
	}
	if r.Anterior.Total != pesos(40000) || r.VsTexto != "vs. mes pasado" || math.Abs(r.VsAnterior-125) > 0.001 {
		t.Errorf("mes pasado = %d (%v%%, %q); quiero 40.000 y +125%%", r.Anterior.Total/100, r.VsAnterior, r.VsTexto)
	}
	// Una barra por día de noviembre; del 22 en adelante, futuro.
	if len(r.Barras) != 30 || r.PorSemana || !r.Barras[20].EsHoy || !r.Barras[21].Futuro || r.Barras[20].Futuro {
		t.Errorf("noviembre: %d barras; la del 21 = %+v, la del 22 = %+v", len(r.Barras), r.Barras[20], r.Barras[21])
	}
}

// Un rango a mano se compara con el tramo anterior del mismo largo; si es largo, va por semana.
func TestMetricas_RangoAMano(t *testing.T) {
	ventas := []VentaDia{venta(11, 3, PagoEfectivo, 1, 10000), venta(11, 13, PagoEfectivo, 2, 30000)}
	r := Metricas(ventas, PeriodoPedidos{Desde: local(11, 13), Hasta: local(11, 23)}, sabado)
	// Del 13 al 22, en curso hasta hoy (21): 9 días; el anterior es del 4 al 12 (el 3 queda afuera).
	if r.Actual.Total != pesos(30000) || r.HayVs || r.VsTexto != "vs. período anterior" {
		t.Errorf("rango = %+v, anterior %+v", r.Actual, r.Anterior)
	}
	if len(r.Barras) != 10 {
		t.Errorf("del 13 al 22 son 10 barras, hay %d", len(r.Barras))
	}
	largo := Metricas(ventas, PeriodoPedidos{Desde: local(9, 1), Hasta: local(11, 22)}, sabado)
	if !largo.PorSemana || len(largo.Barras) != 12 || largo.GraficoTotal != pesos(40000) {
		t.Errorf("del 1/9 al 21/11 (82 días): por semana %v, %d barras, total %d", largo.PorSemana, len(largo.Barras), largo.GraficoTotal/100)
	}
}

func TestMetricas_SinVentasAntesNoHayPorcentaje(t *testing.T) {
	r := Metricas([]VentaDia{venta(11, 21, PagoEfectivo, 1, 10000)}, periodoHoy(), sabado)
	if r.HayVs {
		t.Error("sin ventas ayer no hay contra qué comparar")
	}
	if vacio := Metricas(nil, periodoHoy(), sabado); vacio.Ticket != 0 || vacio.Actual.Pedidos != 0 {
		t.Errorf("sin ventas: %+v", vacio)
	}
}

// Una sola consulta tiene que cubrir el período, el anterior y el gráfico. Un lunes, "hoy" se
// compara con el domingo, que es de la semana anterior.
func TestRangoConsulta(t *testing.T) {
	lunes := time.Date(2026, time.November, 23, 21, 0, 0, 0, Zona)
	desde, hasta := RangoConsulta(PeriodoPedidos{Desde: local(11, 23), Hasta: local(11, 24), Rango: RangoHoy}, lunes)
	if !desde.Equal(local(11, 22)) || !hasta.Equal(local(11, 30)) {
		t.Errorf("rango = %v a %v; quiero del domingo 22 al lunes 30", desde, hasta)
	}
	desde, hasta = RangoConsulta(PeriodoPedidos{Desde: local(11, 1), Hasta: local(12, 1), Rango: RangoMes}, sabado)
	if !desde.Equal(local(10, 1)) || !hasta.Equal(local(12, 1)) {
		t.Errorf("mes = %v a %v; quiero del 1/10 al 1/12", desde, hasta)
	}
}
