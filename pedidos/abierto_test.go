package pedidos

import (
	"reflect"
	"testing"
	"time"
)

// La semana del local: lunes cerrado, martes a jueves de 20 a 00, viernes y sábado de 20 a 01,
// domingo de 20 a 00 (0 = lunes, como la tabla horario).
func semanaLocal() []Horario {
	return []Horario{
		{Dia: 0, Abierto: false, Desde: "20:00", Hasta: "00:00"},
		{Dia: 1, Abierto: true, Desde: "20:00", Hasta: "00:00"},
		{Dia: 2, Abierto: true, Desde: "20:00", Hasta: "00:00"},
		{Dia: 3, Abierto: true, Desde: "20:00", Hasta: "00:00"},
		{Dia: 4, Abierto: true, Desde: "20:00", Hasta: "01:00"},
		{Dia: 5, Abierto: true, Desde: "20:00", Hasta: "01:00"},
		{Dia: 6, Abierto: true, Desde: "20:00", Hasta: "00:00"},
	}
}

// hora arma un momento de la semana del 23/11/2026 (lunes 23 … domingo 29) en la hora del local.
func hora(dia, h, m int) time.Time { return time.Date(2026, time.November, dia, h, m, 0, 0, Zona) }

func TestAbiertoAhora(t *testing.T) {
	casos := []struct {
		nombre string
		ahora  time.Time
		quiero bool
	}{
		{"martes 21:00, dentro del horario", hora(24, 21, 0), true},
		{"martes 19:59, todavía no abrió", hora(24, 19, 59), false},
		{"miércoles 00:00, cerró a la medianoche", hora(25, 0, 0), false},
		{"sábado 00:30, el viernes cierra a la 01:00", hora(28, 0, 30), true},
		{"sábado 01:00, ya cerró", hora(28, 1, 0), false},
		{"lunes 21:00, los lunes no abre", hora(23, 21, 0), false},
		{"lunes 00:15, el domingo cerró a las 00:00", hora(30, 0, 15), false},
	}
	for _, c := range casos {
		if got := AbiertoAhora(true, semanaLocal(), c.ahora); got != c.quiero {
			t.Errorf("%s: abierto = %v", c.nombre, got)
		}
	}
	if AbiertoAhora(false, semanaLocal(), hora(24, 21, 0)) {
		t.Error("con el switch apagado no toma pedidos aunque esté en horario")
	}
}

func TestTextoHorarios(t *testing.T) {
	quiero := []string{
		"Lunes: cerrado",
		"Martes a jueves: 20:00 a 00:00",
		"Viernes y sábado: 20:00 a 01:00",
		"Domingo: 20:00 a 00:00",
	}
	if got := TextoHorarios(semanaLocal()); !reflect.DeepEqual(got, quiero) {
		t.Errorf("TextoHorarios =\n%q\nquiero\n%q", got, quiero)
	}
}
