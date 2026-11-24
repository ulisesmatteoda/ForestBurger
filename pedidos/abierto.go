package pedidos

import (
	"fmt"
	"strings"
	"time"
)

// Horario es el de un día, como está en la tabla horario: Dia 0 es lunes … 6 es domingo, y las
// horas "HH:MM". Si Hasta no es mayor que Desde, cierra al día siguiente (de 20:00 a 01:00).
type Horario struct {
	Dia          int
	Abierto      bool
	Desde, Hasta string
}

// AbiertoAhora dice si la web toma pedidos: el switch "Local abierto" prendido y dentro del
// horario. Cuenta el horario de hoy y el de ayer si cerraba pasada la medianoche (un viernes de
// 20:00 a 01:00 sigue abierto el sábado a las 00:30).
func AbiertoAhora(localAbierto bool, horarios []Horario, ahora time.Time) bool {
	if !localAbierto {
		return false
	}
	t := ahora.In(Zona)
	minuto := t.Hour()*60 + t.Minute()
	hoy := (int(t.Weekday()) + 6) % 7 // lunes = 0
	ayer := (hoy + 6) % 7
	for _, h := range horarios {
		if !h.Abierto {
			continue
		}
		desde, hasta, ok := minutosDe(h)
		if !ok {
			continue
		}
		pasaMedianoche := hasta <= desde
		switch {
		case h.Dia == hoy && !pasaMedianoche && minuto >= desde && minuto < hasta:
			return true
		case h.Dia == hoy && pasaMedianoche && minuto >= desde:
			return true
		case h.Dia == ayer && pasaMedianoche && minuto < hasta:
			return true
		}
	}
	return false
}

func minutosDe(h Horario) (desde, hasta int, ok bool) {
	d, errD := time.Parse("15:04", h.Desde)
	a, errA := time.Parse("15:04", h.Hasta)
	if errD != nil || errA != nil {
		return 0, 0, false
	}
	return d.Hour()*60 + d.Minute(), a.Hour()*60 + a.Minute(), true
}

var nombresDiaLunes = [7]string{"Lunes", "Martes", "Miércoles", "Jueves", "Viernes", "Sábado", "Domingo"}

// TextoHorarios arma los horarios para el cliente, juntando los días seguidos que abren igual:
// "Lunes: cerrado", "Martes a jueves: 20:00 a 00:00", "Viernes y sábado: 20:00 a 01:00".
func TextoHorarios(horarios []Horario) []string {
	porDia := make([]string, 7)
	for i := range porDia {
		porDia[i] = "cerrado"
	}
	for _, h := range horarios {
		if h.Dia >= 0 && h.Dia < 7 && h.Abierto {
			porDia[h.Dia] = fmt.Sprintf("%s a %s", h.Desde, h.Hasta)
		}
	}
	var lineas []string
	for i := 0; i < 7; {
		j := i
		for j+1 < 7 && porDia[j+1] == porDia[i] {
			j++
		}
		dias := nombresDiaLunes[i]
		switch {
		case j == i+1:
			dias += " y " + strings.ToLower(nombresDiaLunes[j])
		case j > i+1:
			dias += " a " + strings.ToLower(nombresDiaLunes[j])
		}
		lineas = append(lineas, dias+": "+porDia[i])
		i = j + 1
	}
	return lineas
}
