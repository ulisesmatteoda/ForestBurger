// Package promos hace las cuentas y los textos de las promos. Una promo es "elegí N hamburguesas
// entre estas, de este tamaño, por $X": el cliente arma la suya. No toca la base: el panel le pasa
// los datos y los precios actuales de los productos.
package promos

import (
	"errors"
	"fmt"
	"strings"

	"forestburger/pedidos"
)

// Tamaños de una promo. Con TamanoCliente el cliente elige simple o doble en cada una.
const (
	TamanoSimple  = "simple"
	TamanoDoble   = "doble"
	TamanoCliente = "cliente"
)

const (
	CantidadMin    = 1
	CantidadMax    = 10
	LargoMaxNombre = 100
)

// ErrPromoInvalida envuelve los errores de validación: el mensaje es para mostrarle al local.
var ErrPromoInvalida = errors.New("promo inválida")

// Producto es un producto que entra en la promo, con sus precios actuales.
type Producto struct {
	ID     int32
	Nombre string
	Simple pedidos.Centavos
	Doble  pedidos.Centavos // 0 si no tiene doble
}

// Promo son los datos de una promo para hacer sus cuentas y sus textos.
type Promo struct {
	Nombre    string
	Precio    pedidos.Centavos
	Cantidad  int
	Tamano    string
	Dias      []int // 0 = domingo … 6 = sábado
	Productos []Producto
}

// Cuentas compara la promo con pedir lo mismo por separado, a precio de menú. Como el cliente puede
// repetir (2 entre Classic y Oklahoma puede ser 2 Classic), lo más barato es Cantidad veces la más
// barata y lo más caro, Cantidad veces la más cara.
type Cuentas struct {
	Min, Max pedidos.Centavos
	Ahorro   pedidos.Centavos // Max - Precio: lo más que ahorra el cliente (negativo si no ahorra)
	MasCara  bool             // la promo sale más cara que pedirlas por separado, incluso las caras
}

// Calcular hace las cuentas. Sin productos, todo en cero.
func Calcular(p Promo) Cuentas {
	if len(p.Productos) == 0 {
		return Cuentas{}
	}
	var min, max pedidos.Centavos
	for i, prod := range p.Productos {
		barato, caro := precios(prod, p.Tamano)
		if i == 0 || barato < min {
			min = barato
		}
		if caro > max {
			max = caro
		}
	}
	n := pedidos.Centavos(p.Cantidad)
	c := Cuentas{Min: n * min, Max: n * max}
	c.Ahorro = c.Max - p.Precio
	c.MasCara = p.Precio > c.Max
	return c
}

// precios dice cuánto cuesta un producto por separado en el tamaño de la promo. Con "el cliente
// elige", lo barato es la simple y lo caro la doble (o la simple, si no tiene doble).
func precios(prod Producto, tamano string) (barato, caro pedidos.Centavos) {
	switch tamano {
	case TamanoDoble:
		return prod.Doble, prod.Doble
	case TamanoCliente:
		if prod.Doble > 0 {
			return prod.Simple, prod.Doble
		}
	}
	return prod.Simple, prod.Simple
}

// Validar revisa la promo antes de guardarla. El error envuelve ErrPromoInvalida.
func Validar(p Promo) error {
	switch {
	case strings.TrimSpace(p.Nombre) == "":
		return invalida("Falta el nombre de la promo")
	case len([]rune(p.Nombre)) > LargoMaxNombre:
		return invalida("El nombre es muy largo")
	case p.Precio <= 0:
		return invalida("Poné el precio de la promo")
	case p.Cantidad < CantidadMin || p.Cantidad > CantidadMax:
		return invalida("La cantidad tiene que ser de %d a %d", CantidadMin, CantidadMax)
	case p.Tamano != TamanoSimple && p.Tamano != TamanoDoble && p.Tamano != TamanoCliente:
		return invalida("Elegí el tamaño")
	case len(p.Productos) == 0:
		return invalida("Marcá al menos un producto que entre en la promo")
	case len(p.Dias) == 0:
		return invalida("Elegí al menos un día")
	}
	for _, d := range p.Dias {
		if d < 0 || d > 6 {
			return invalida("Día inválido")
		}
	}
	if p.Tamano == TamanoDoble {
		for _, prod := range p.Productos {
			if prod.Doble == 0 {
				return invalida("%s no tiene doble", prod.Nombre)
			}
		}
	}
	return nil
}

func invalida(formato string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrPromoInvalida, fmt.Sprintf(formato, args...))
}

// Mensaje saca el texto para el local de un error de Validar ("Falta el nombre de la promo").
func Mensaje(err error) string {
	return strings.TrimPrefix(err.Error(), ErrPromoInvalida.Error()+": ")
}

// Encabezado es la primera línea de la tarjeta: "Elegí 2 hamburguesas simples entre:".
func Encabezado(p Promo) string {
	return fmt.Sprintf("Elegí %d %s entre:", p.Cantidad, queElige(p))
}

// Descripcion es el texto completo que ve el cliente: "Elegí 2 hamburguesas simples entre Big
// Forest, Classic y Oklahoma. Martes a domingo." Con un solo producto: "Elegí 2 Classic simples".
func Descripcion(p Promo) string {
	nombres := make([]string, len(p.Productos))
	for i, prod := range p.Productos {
		nombres[i] = prod.Nombre
	}
	var texto string
	switch len(nombres) {
	case 0:
		texto = fmt.Sprintf("Elegí %d %s.", p.Cantidad, queElige(p))
	case 1:
		texto = fmt.Sprintf("Elegí %d %s %s.", p.Cantidad, nombres[0], tamanoPlural(p))
	default:
		texto = fmt.Sprintf("Elegí %d %s entre %s.", p.Cantidad, queElige(p), Lista(nombres))
	}
	if len(p.Dias) > 0 {
		texto += " " + TextoDias(p.Dias) + "."
	}
	return strings.TrimSpace(strings.ReplaceAll(texto, "  ", " "))
}

// queElige: "hamburguesa simple", "hamburguesas dobles", "hamburguesas, simples o dobles,".
func queElige(p Promo) string {
	sustantivo := "hamburguesas"
	if p.Cantidad == 1 {
		sustantivo = "hamburguesa"
	}
	if t := tamanoPlural(p); t != "" {
		if p.Tamano == TamanoCliente {
			return sustantivo + ", " + t + ","
		}
		return sustantivo + " " + t
	}
	return sustantivo
}

func tamanoPlural(p Promo) string {
	uno := p.Cantidad == 1
	switch p.Tamano {
	case TamanoSimple:
		if uno {
			return "simple"
		}
		return "simples"
	case TamanoDoble:
		if uno {
			return "doble"
		}
		return "dobles"
	case TamanoCliente:
		if uno {
			return "simple o doble"
		}
		return "simples o dobles"
	}
	return ""
}

// Los días en el orden de la semana argentina (lunes primero), con su número de time.Weekday.
var semana = []struct {
	n            int
	largo, corto string
}{
	{1, "Lunes", "Lun"}, {2, "Martes", "Mar"}, {3, "Miércoles", "Mié"}, {4, "Jueves", "Jue"},
	{5, "Viernes", "Vie"}, {6, "Sábado", "Sáb"}, {0, "Domingo", "Dom"},
}

// TextoDias arma cómo se lee cuándo está la promo: "Todos los días", "Solo martes", "Martes a
// domingo" (tres o más seguidos) o "Lun, Mié y Vie".
func TextoDias(dias []int) string {
	hay := map[int]bool{}
	for _, d := range dias {
		hay[d] = true
	}
	var elegidos []int // posiciones en semana
	for i, d := range semana {
		if hay[d.n] {
			elegidos = append(elegidos, i)
		}
	}
	switch {
	case len(elegidos) == 0:
		return ""
	case len(elegidos) == 7:
		return "Todos los días"
	case len(elegidos) == 1:
		return "Solo " + strings.ToLower(semana[elegidos[0]].largo)
	}
	seguidos := true
	for i := 1; i < len(elegidos); i++ {
		if elegidos[i] != elegidos[i-1]+1 {
			seguidos = false
		}
	}
	primero, ultimo := semana[elegidos[0]], semana[elegidos[len(elegidos)-1]]
	if seguidos && len(elegidos) >= 3 {
		return primero.largo + " a " + strings.ToLower(ultimo.largo)
	}
	if len(elegidos) == 2 {
		return primero.largo + " y " + strings.ToLower(ultimo.largo)
	}
	cortos := make([]string, len(elegidos))
	for i, e := range elegidos {
		cortos[i] = semana[e].corto
	}
	return Lista(cortos)
}

// Lista une con comas y una "y" al final: "A, B y C".
func Lista(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " y " + items[len(items)-1]
}
