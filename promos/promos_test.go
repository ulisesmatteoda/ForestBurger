package promos

import (
	"errors"
	"testing"

	"forestburger/pedidos"
)

const peso = pedidos.Centavos(100)

var (
	bigForest    = Producto{ID: 1, Nombre: "Big Forest", Simple: 14500 * peso, Doble: 16500 * peso}
	cheeseBurger = Producto{ID: 2, Nombre: "Cheese Burger", Simple: 14000 * peso, Doble: 16000 * peso}
	classic      = Producto{ID: 3, Nombre: "Classic", Simple: 14500 * peso, Doble: 16500 * peso}
	oklahoma     = Producto{ID: 4, Nombre: "Oklahoma", Simple: 14500 * peso, Doble: 16500 * peso}
	sinDoble     = Producto{ID: 5, Nombre: "Papas", Simple: 6500 * peso}
)

// La promo del Figma: 2 simples entre Big Forest, Cheese Burger, Classic y Oklahoma por $26.000.
func promoFigma() Promo {
	return Promo{
		Nombre: "2 Smash por $26.000", Precio: 26000 * peso, Cantidad: 2, Tamano: TamanoSimple,
		Dias:      []int{2, 3, 4, 5, 6, 0},
		Productos: []Producto{bigForest, cheeseBurger, classic, oklahoma},
	}
}

func TestCalcular_EjemploDelFigma(t *testing.T) {
	c := Calcular(promoFigma())
	if c.Min != 28000*peso || c.Max != 29000*peso || c.Ahorro != 3000*peso || c.MasCara {
		t.Errorf("cuentas = %+v; quiero 28.000 / 29.000 y ahorro hasta 3.000", c)
	}
}

func TestCalcular_ElClienteElige(t *testing.T) {
	p := promoFigma()
	p.Tamano = TamanoCliente
	// Lo barato en simple (2 × 14.000) y lo caro en doble (2 × 16.500).
	if c := Calcular(p); c.Min != 28000*peso || c.Max != 33000*peso || c.Ahorro != 7000*peso {
		t.Errorf("cuentas = %+v; quiero 28.000 / 33.000", c)
	}
	p.Tamano = TamanoDoble
	if c := Calcular(p); c.Min != 32000*peso || c.Max != 33000*peso {
		t.Errorf("dobles: %+v; quiero 32.000 / 33.000", c)
	}
}

func TestCalcular_MasCaraQuePorSeparado(t *testing.T) {
	p := promoFigma()
	p.Precio = 30000 * peso
	if c := Calcular(p); !c.MasCara || c.Ahorro != -1000*peso {
		t.Errorf("a $30.000 sale más cara que las más caras ($29.000): %+v", c)
	}
	p.Productos = nil
	if c := Calcular(p); c != (Cuentas{}) {
		t.Errorf("sin productos no hay cuentas: %+v", c)
	}
}

func TestTextoDias(t *testing.T) {
	casos := map[string][]int{
		"Todos los días":   {0, 1, 2, 3, 4, 5, 6},
		"Martes a domingo": {2, 3, 4, 5, 6, 0},
		"Solo martes":      {2},
		"Lun, Mié y Vie":   {1, 3, 5},
		"Sábado y domingo": {6, 0},
		"Lunes a viernes":  {5, 4, 3, 2, 1},
		"":                 nil,
	}
	for quiero, dias := range casos {
		if got := TextoDias(dias); got != quiero {
			t.Errorf("TextoDias(%v) = %q, quiero %q", dias, got, quiero)
		}
	}
}

func TestDescripcion(t *testing.T) {
	p := promoFigma()
	if got, quiero := Descripcion(p), "Elegí 2 hamburguesas simples entre Big Forest, Cheese Burger, Classic y Oklahoma. Martes a domingo."; got != quiero {
		t.Errorf("Descripcion = %q\n quiero    %q", got, quiero)
	}
	if got := Encabezado(p); got != "Elegí 2 hamburguesas simples entre:" {
		t.Errorf("Encabezado = %q", got)
	}
	veggie := Promo{Cantidad: 1, Tamano: TamanoDoble, Dias: []int{2}, Productos: []Producto{classic, bigForest}}
	if got := Descripcion(veggie); got != "Elegí 1 hamburguesa doble entre Classic y Big Forest. Solo martes." {
		t.Errorf("Descripcion = %q", got)
	}
	uno := Promo{Cantidad: 2, Tamano: TamanoCliente, Dias: []int{0, 1, 2, 3, 4, 5, 6}, Productos: []Producto{classic}}
	if got := Descripcion(uno); got != "Elegí 2 Classic simples o dobles. Todos los días." {
		t.Errorf("Descripcion = %q", got)
	}
}

func TestValidar(t *testing.T) {
	if err := Validar(promoFigma()); err != nil {
		t.Fatalf("la promo del Figma es válida: %v", err)
	}
	casos := map[string]struct {
		romper  func(p *Promo)
		mensaje string
	}{
		"sin nombre":      {func(p *Promo) { p.Nombre = " " }, "Falta el nombre de la promo"},
		"sin precio":      {func(p *Promo) { p.Precio = 0 }, "Poné el precio de la promo"},
		"cantidad cero":   {func(p *Promo) { p.Cantidad = 0 }, "La cantidad tiene que ser de 1 a 10"},
		"tamaño raro":     {func(p *Promo) { p.Tamano = "gigante" }, "Elegí el tamaño"},
		"sin productos":   {func(p *Promo) { p.Productos = nil }, "Marcá al menos un producto que entre en la promo"},
		"sin días":        {func(p *Promo) { p.Dias = nil }, "Elegí al menos un día"},
		"doble sin doble": {func(p *Promo) { p.Tamano = TamanoDoble; p.Productos = append(p.Productos, sinDoble) }, "Papas no tiene doble"},
	}
	for nombre, c := range casos {
		p := promoFigma()
		c.romper(&p)
		err := Validar(p)
		if !errors.Is(err, ErrPromoInvalida) || Mensaje(err) != c.mensaje {
			t.Errorf("%s: error = %v, quiero %q", nombre, err, c.mensaje)
		}
	}
}
