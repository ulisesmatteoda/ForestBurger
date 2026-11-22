package pedidos

import "testing"

func TestCentavos(t *testing.T) {
	c, err := CentavosDe("17500.50")
	if err != nil || c != 1750050 {
		t.Fatalf("CentavosDe = %d, %v", c, err)
	}
	if c.Texto() != "17500.50" || Centavos(150000).Texto() != "1500.00" {
		t.Errorf("Texto: %q, %q", c.Texto(), Centavos(150000).Texto())
	}
	if _, err := CentavosDe("mil"); err == nil {
		t.Error("un monto que no es número tendría que dar error")
	}
}

// El ejemplo del Figma: Classic doble + cheddar ($16.500 + $1.000) y 2 gaseosas ($3.500), 20% de
// descuento y envío $1.500.
func TestCalcular_EjemploDelFigma(t *testing.T) {
	lineas := []Linea{
		{Cantidad: 1, Precio: 1650000, Extras: []LineaExtra{{Cantidad: 1, Precio: 100000}}},
		{Cantidad: 2, Precio: 350000},
	}
	c := Calcular(lineas, Descuento{Pct: 20}, 150000)
	if c.Subtotal != 2450000 || c.Descuento != 490000 || c.Envio != 150000 || c.Total != 2110000 {
		t.Errorf("cuentas = %+v; quiero subtotal 24.500, descuento 4.900, envío 1.500 y total 21.100", c)
	}
}

func TestCalcular_ExtrasPorCadaUnidad(t *testing.T) {
	// 2 hamburguesas de $14.000, cada una con doble cheddar ($1.000 × 2): 2 × (14.000 + 2.000).
	l := Linea{Cantidad: 2, Precio: 1400000, Extras: []LineaExtra{{Cantidad: 2, Precio: 100000}}}
	if l.Total() != 3200000 {
		t.Errorf("total de la línea = %d, quiero 32.000", l.Total()/100)
	}
}

func TestCalcular_DescuentoRedondeadoAlPeso(t *testing.T) {
	// 15% de $14.555 = $2.183,25 → $2.183.
	c := Calcular([]Linea{{Cantidad: 1, Precio: 1455500}}, Descuento{Pct: 15}, 0)
	if c.Descuento != 218300 || c.Total != 1237200 {
		t.Errorf("cuentas = %+v; quiero descuento 2.183 y total 12.372", c)
	}
}

func TestCalcular_SinDescuentoNiEnvio(t *testing.T) {
	c := Calcular([]Linea{{Cantidad: 3, Precio: 250000}}, Descuento{}, 0)
	if c.Subtotal != 750000 || c.Descuento != 0 || c.Total != 750000 {
		t.Errorf("cuentas = %+v", c)
	}
}

func TestCalcular_DescuentoFijo(t *testing.T) {
	lineas := []Linea{{Cantidad: 2, Precio: 1000000}} // $20.000
	if c := Calcular(lineas, Descuento{Fijo: 250000}, 150000); c.Descuento != 250000 || c.Total != 1900000 {
		t.Errorf("descuento fijo de $2.500: %+v; quiero total 20.000 - 2.500 + 1.500 = 19.000", c)
	}
	// Más que el subtotal: se queda en el subtotal (el envío se cobra igual).
	if c := Calcular(lineas, Descuento{Fijo: 5000000}, 150000); c.Descuento != 2000000 || c.Total != 150000 {
		t.Errorf("descuento mayor al subtotal: %+v", c)
	}
}
