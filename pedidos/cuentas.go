// Package pedidos crea pedidos y hace sus cuentas. Lo usan el panel ("Cargar pedido") y el seed
// de desarrollo, y más adelante la web. Las cuentas siempre las hace el servidor con los precios
// de la base: quien crea un pedido solo manda qué productos y extras, y cuántos.
package pedidos

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Las cuentas se hacen en centavos enteros: con float64 una suma de precios puede dar
// 17499.999999 y el total quedaría mal por un centavo.
type Centavos int64

const centavosPorPeso = 100

// CentavosDe pasa un monto como lo devuelve la base ("17500.00") a centavos.
func CentavosDe(s string) (Centavos, error) {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, fmt.Errorf("monto inválido %q", s)
	}
	return Centavos(math.Round(f * centavosPorPeso)), nil
}

// Texto lo devuelve como lo guarda la base: "17500.00".
func (c Centavos) Texto() string {
	signo := ""
	if c < 0 {
		signo, c = "-", -c
	}
	return fmt.Sprintf("%s%d.%02d", signo, c/centavosPorPeso, c%centavosPorPeso)
}

// Linea es un ítem del pedido para las cuentas: cuántos, a qué precio, y los extras de cada unidad.
type Linea struct {
	Cantidad int
	Precio   Centavos
	Extras   []LineaExtra
}

// LineaExtra es un extra de cada unidad del ítem (doble cheddar: Cantidad 2).
type LineaExtra struct {
	Cantidad int
	Precio   Centavos
}

// Total de la línea: cantidad × (precio + extras de cada unidad).
func (l Linea) Total() Centavos {
	unidad := l.Precio
	for _, e := range l.Extras {
		unidad += Centavos(e.Cantidad) * e.Precio
	}
	return Centavos(l.Cantidad) * unidad
}

// Cuentas es el detalle que se guarda y se muestra: subtotal, descuento, envío y total.
type Cuentas struct {
	Subtotal  Centavos
	Descuento Centavos
	Envio     Centavos
	Total     Centavos
}

// Descuento es un porcentaje del subtotal o un monto fijo (uno o el otro, o ninguno).
type Descuento struct {
	Pct  int
	Fijo Centavos
}

// Calcular arma las cuentas de un pedido. El descuento se aplica sobre el subtotal (no sobre el
// envío): el porcentaje redondeado al peso, el fijo tal cual pero nunca más que el subtotal. El
// total nunca queda negativo.
func Calcular(lineas []Linea, d Descuento, envio Centavos) Cuentas {
	var c Cuentas
	for _, l := range lineas {
		c.Subtotal += l.Total()
	}
	switch {
	case d.Pct > 0:
		pesos := math.Round(float64(c.Subtotal) * float64(d.Pct) / 100 / centavosPorPeso)
		c.Descuento = Centavos(pesos) * centavosPorPeso
	case d.Fijo > 0:
		c.Descuento = min(d.Fijo, c.Subtotal)
	}
	c.Envio = envio
	c.Total = c.Subtotal - c.Descuento + c.Envio
	if c.Total < 0 {
		c.Total = 0
	}
	return c
}
