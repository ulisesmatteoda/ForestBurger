package pedidos

import (
	"context"
	"errors"
	"testing"

	"forestburger/internal/basetest"
)

// El presupuesto que se ve al armar el pedido tiene que dar lo mismo que lo que guarda Crear.
func TestPresupuesto_IgualAlPedidoGuardado(t *testing.T) {
	db := basetest.Abrir(t)
	c := cargarCatalogo(t, db)
	ctx := context.Background()
	n := pedidoFigma(c)

	pres, err := CalcularPresupuesto(ctx, db, n)
	if err != nil {
		t.Fatal(err)
	}
	if pres.Cuentas.Total != pesos(21100) || pres.Cuentas.Descuento != pesos(4900) || pres.Cuentas.Envio != pesos(1500) {
		t.Errorf("presupuesto = %+v; quiero total 21.100", pres.Cuentas)
	}
	l := pres.Lineas[0]
	if l.Nombre != "Classic" || l.Tamano != TamanoDoble || len(l.Extras) != 1 || l.Extras[0] != "Cheddar" || l.Unidad != pesos(17500) || l.Total != pesos(17500) {
		t.Errorf("primer renglón = %+v; quiero Classic doble + Cheddar a $17.500", l)
	}
	if pres.Lineas[1].Total != pesos(7000) {
		t.Errorf("2 gaseosas = %d, quiero 7.000", pres.Lineas[1].Total/100)
	}

	p, err := Crear(ctx, db, n)
	if err != nil {
		t.Fatal(err)
	}
	if p.Total != pres.Cuentas.Total.Texto() {
		t.Errorf("guardado %s, presupuesto %s: tienen que coincidir", p.Total, pres.Cuentas.Total.Texto())
	}
	var hay int
	db.QueryRow(`SELECT count(*) FROM pedido`).Scan(&hay)
	if hay != 1 {
		t.Errorf("el presupuesto no tiene que guardar nada: hay %d pedidos", hay)
	}
}

func TestPresupuesto_DescuentoFijoYExtraApagado(t *testing.T) {
	db := basetest.Abrir(t)
	c := cargarCatalogo(t, db)
	ctx := context.Background()

	n := pedidoFigma(c)
	n.DescuentoPct, n.DescuentoFijo = 0, pesos(2000)
	pres, err := CalcularPresupuesto(ctx, db, n)
	if err != nil || pres.Cuentas.Descuento != pesos(2000) || pres.Cuentas.Total != pesos(24000) {
		t.Errorf("descuento fijo de $2.000: %+v, %v; quiero 24.500 - 2.000 + 1.500 = 24.000", pres.Cuentas, err)
	}

	n.Items[0].Extras[0].ExtraID = c.panceta.ID // apagada
	if _, err := CalcularPresupuesto(ctx, db, n); !errors.Is(err, ErrPedidoInvalido) {
		t.Errorf("un extra apagado no entra: quiero ErrPedidoInvalido, vino %v", err)
	}
	// Sin cliente igual se puede presupuestar (se está cargando).
	n.Items[0].Extras = nil
	n.Cliente = ""
	if _, err := CalcularPresupuesto(ctx, db, n); err != nil {
		t.Errorf("sin cliente el presupuesto igual se calcula: %v", err)
	}
}
