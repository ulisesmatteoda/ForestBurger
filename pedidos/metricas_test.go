package pedidos

import (
	"math"
	"testing"
	"time"
)

// dia arma la fecha como la devuelve la base: medianoche UTC.
func dia(a int, m time.Month, d int) time.Time { return time.Date(a, m, d, 0, 0, 0, 0, time.UTC) }

func pesos(p int64) Centavos { return Centavos(p * 100) }

// Sábado 21/11/2026: la semana va del lunes 16 al domingo 22.
var sabado = time.Date(2026, time.November, 21, 22, 0, 0, 0, Zona)

func TestMetricas_HoyAyerYSemana(t *testing.T) {
	ventas := []VentaDia{
		{Dia: dia(2026, 11, 16), Pago: PagoEfectivo, Pedidos: 2, Total: pesos(30000)},      // lunes
		{Dia: dia(2026, 11, 20), Pago: PagoEfectivo, Pedidos: 3, Total: pesos(60000)},      // ayer
		{Dia: dia(2026, 11, 20), Pago: PagoTransferencia, Pedidos: 1, Total: pesos(20000)}, // ayer
		{Dia: dia(2026, 11, 21), Pago: PagoEfectivo, Pedidos: 2, Total: pesos(50000)},      // hoy
		{Dia: dia(2026, 11, 21), Pago: PagoTransferencia, Pedidos: 2, Total: pesos(40000)}, // hoy
	}
	r := Metricas(ventas, sabado)

	if r.Hoy.Total != pesos(90000) || r.Hoy.Efectivo != pesos(50000) || r.Hoy.Transferencia != pesos(40000) || r.Hoy.Pedidos != 4 {
		t.Errorf("hoy = %+v", r.Hoy)
	}
	if r.Semana.Total != pesos(200000) || r.Semana.Efectivo != pesos(140000) || r.Semana.Transferencia != pesos(60000) || r.Semana.Pedidos != 10 {
		t.Errorf("semana = %+v", r.Semana)
	}
	if r.Ticket != pesos(22500) {
		t.Errorf("ticket = %d, quiero 22.500", r.Ticket/100)
	}
	// Hoy 90.000 contra ayer 80.000: +12,5%.
	if !r.HayVsAyer || math.Abs(r.VsAyer-12.5) > 0.001 {
		t.Errorf("vs ayer = %v (%v), quiero +12,5%%", r.VsAyer, r.HayVsAyer)
	}
}

func TestMetricas_Barras(t *testing.T) {
	ventas := []VentaDia{
		{Dia: dia(2026, 11, 16), Pago: PagoEfectivo, Pedidos: 1, Total: pesos(45000)},
		{Dia: dia(2026, 11, 21), Pago: PagoEfectivo, Pedidos: 4, Total: pesos(90000)},
	}
	r := Metricas(ventas, sabado)

	if !r.Lunes.Equal(time.Date(2026, time.November, 16, 0, 0, 0, 0, Zona)) {
		t.Fatalf("lunes = %v", r.Lunes)
	}
	lunes, martes, sab, dom := r.Dias[0], r.Dias[1], r.Dias[5], r.Dias[6]
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

func TestMetricas_SinVentasAyerNoHayPorcentaje(t *testing.T) {
	r := Metricas([]VentaDia{{Dia: dia(2026, 11, 21), Pago: PagoEfectivo, Pedidos: 1, Total: pesos(10000)}}, sabado)
	if r.HayVsAyer {
		t.Error("sin ventas ayer no hay contra qué comparar")
	}
	if vacio := Metricas(nil, sabado); vacio.Ticket != 0 || vacio.Hoy.Pedidos != 0 {
		t.Errorf("sin ventas: %+v", vacio)
	}
}

// Un lunes, ayer es el domingo de la semana anterior: cuenta para "ayer" pero no para la semana.
func TestMetricas_UnLunes(t *testing.T) {
	lunes := time.Date(2026, time.November, 23, 21, 0, 0, 0, Zona)
	desde, hasta := RangoMetricas(lunes)
	if desde.Day() != 22 || hasta.Day() != 30 {
		t.Errorf("rango = %v a %v; quiero desde el domingo 22 hasta el lunes 30", desde, hasta)
	}
	r := Metricas([]VentaDia{
		{Dia: dia(2026, 11, 22), Pago: PagoEfectivo, Pedidos: 2, Total: pesos(40000)},
		{Dia: dia(2026, 11, 23), Pago: PagoEfectivo, Pedidos: 1, Total: pesos(20000)},
	}, lunes)
	if r.Ayer.Total != pesos(40000) || r.Semana.Total != pesos(20000) || !r.Dias[0].EsHoy {
		t.Errorf("ayer = %d, semana = %d, el lunes es hoy = %v", r.Ayer.Total/100, r.Semana.Total/100, r.Dias[0].EsHoy)
	}
}
