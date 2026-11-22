package pedidos

import (
	"context"
	"database/sql"

	sqlc "forestburger/db/sqlc"
)

// Presupuesto es lo que se ve mientras se arma un pedido ("Cargar pedido"): cada renglón con su
// precio y las cuentas. Sale de la base igual que lo que guarda Crear, así lo que se ve es lo que
// se cobra.
type Presupuesto struct {
	Lineas  []LineaPresupuesto
	Cuentas Cuentas
}

// LineaPresupuesto es un renglón: el producto, sus extras y cuánto sale.
type LineaPresupuesto struct {
	Nombre   string
	Tamano   string
	Extras   []string // nombres de los extras de cada unidad
	Cantidad int
	Unidad   Centavos // precio de una unidad con sus extras ("$17.500 c/u")
	Total    Centavos
}

// CalcularPresupuesto hace las cuentas de un pedido sin guardarlo. No exige los datos del cliente
// (todavía se están cargando): solo que los renglones sean válidos.
func CalcularPresupuesto(ctx context.Context, db *sql.DB, n NuevoPedido) (Presupuesto, error) {
	if err := validarItems(n.Items); err != nil {
		return Presupuesto{}, err
	}
	items, c, err := cotizar(ctx, sqlc.New(db), n)
	if err != nil {
		return Presupuesto{}, err
	}
	p := Presupuesto{Cuentas: c}
	for _, it := range items {
		unidad, _ := CentavosDe(it.item.PrecioUnitario)
		l := LineaPresupuesto{Nombre: it.item.Nombre, Tamano: it.item.Tamano, Cantidad: int(it.item.Cantidad)}
		for _, e := range it.extras {
			precio, _ := CentavosDe(e.PrecioUnitario)
			unidad += Centavos(e.Cantidad) * precio
			l.Extras = append(l.Extras, e.Nombre)
		}
		l.Unidad = unidad
		l.Total = Centavos(l.Cantidad) * unidad
		p.Lineas = append(p.Lineas, l)
	}
	return p, nil
}
