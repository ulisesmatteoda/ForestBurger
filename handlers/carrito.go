package handlers

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"time"

	"forestburger/pedidos"
)

// El carrito del cliente va en una cookie: no hace falta registrarse ni guardar nada en la base
// hasta que confirma. Solo dice qué eligió (producto, tamaño, cantidad, extras y aclaración); los
// precios los pone siempre el servidor con los de la base, así que tocar la cookie no cambia
// cuánto se cobra.

const (
	cookieCarrito  = "fb_carrito"
	cookieDatos    = "fb_datos"
	duracionCookie = 7 * 24 * time.Hour
	maxLineas      = 30 // un pedido de verdad no tiene más; la cookie tiene un tope de 4 KB
	maxPorLinea    = 20
)

type extraCarrito struct {
	ID       int32 `json:"i"`
	Cantidad int   `json:"c"`
}

// lineaCarrito es un renglón: la misma hamburguesa con otros extras u otra aclaración es otro
// renglón.
type lineaCarrito struct {
	Producto     int32          `json:"p"`
	Tamano       string         `json:"t"`
	Cantidad     int            `json:"c"`
	Extras       []extraCarrito `json:"e,omitempty"`
	Aclaraciones string         `json:"a,omitempty"`
	// Una promo armada: cuál y las hamburguesas elegidas (Producto y Extras no se usan).
	Promo    int32            `json:"r,omitempty"`
	Elegidas []elegidaCarrito `json:"s,omitempty"`
}

// elegidaCarrito es una hamburguesa de una promo: cuál, el tamaño y sus extras (uno de cada uno).
type elegidaCarrito struct {
	Producto int32   `json:"p"`
	Tamano   string  `json:"t"`
	Extras   []int32 `json:"e,omitempty"`
}

type carrito struct {
	Lineas []lineaCarrito `json:"l"`
}

// datosCliente son los datos del formulario "Tus datos". Se guardan en otra cookie para no tener
// que escribirlos de nuevo (al volver a modificar, o en el próximo pedido).
type datosCliente struct {
	Nombre    string `json:"n"`
	Telefono  string `json:"t"`
	Entrega   string `json:"e"`
	Direccion string `json:"d"`
	Pago      string `json:"p"`
	Notas     string `json:"o"`
}

// leerCookieJSON lee una cookie en JSON (base64url). Una cookie rota o vieja cuenta como vacía.
func leerCookieJSON(r *http.Request, nombre string, destino any) {
	c, err := r.Cookie(nombre)
	if err != nil {
		return
	}
	crudo, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil {
		return
	}
	json.Unmarshal(crudo, destino)
}

func guardarCookieJSON(w http.ResponseWriter, nombre string, valor any) {
	crudo, _ := json.Marshal(valor)
	http.SetCookie(w, &http.Cookie{
		Name: nombre, Value: base64.RawURLEncoding.EncodeToString(crudo), Path: "/",
		MaxAge: int(duracionCookie.Seconds()), HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
}

func borrarCookie(w http.ResponseWriter, nombre string) {
	http.SetCookie(w, &http.Cookie{Name: nombre, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

// leerCarrito devuelve el carrito, descartando renglones sin sentido (de una cookie tocada a mano).
func leerCarrito(r *http.Request) carrito {
	var c carrito
	leerCookieJSON(r, cookieCarrito, &c)
	var limpias []lineaCarrito
	for _, l := range c.Lineas {
		if l.Cantidad < 1 || l.Cantidad > maxPorLinea || (l.Tamano != pedidos.TamanoSimple && l.Tamano != pedidos.TamanoDoble) {
			continue
		}
		limpias = append(limpias, l)
	}
	if len(limpias) > maxLineas {
		limpias = limpias[:maxLineas]
	}
	c.Lineas = limpias
	return c
}

func guardarCarrito(w http.ResponseWriter, c carrito) {
	if len(c.Lineas) == 0 {
		borrarCookie(w, cookieCarrito)
		return
	}
	guardarCookieJSON(w, cookieCarrito, c)
}

func leerDatos(r *http.Request) datosCliente {
	var d datosCliente
	leerCookieJSON(r, cookieDatos, &d)
	return d
}

// agregar suma el renglón: si ya hay uno igual (mismo producto, tamaño, extras y aclaración),
// le suma la cantidad.
func (c *carrito) agregar(l lineaCarrito) {
	l.Aclaraciones = strings.TrimSpace(l.Aclaraciones)
	for i, otra := range c.Lineas {
		if mismoArmado(otra, l) {
			c.Lineas[i].Cantidad = min(otra.Cantidad+l.Cantidad, maxPorLinea)
			return
		}
	}
	if len(c.Lineas) < maxLineas {
		c.Lineas = append(c.Lineas, l)
	}
}

// cambiar suma o resta en el renglón i; en 0 lo saca.
func (c *carrito) cambiar(i, paso int) {
	if i < 0 || i >= len(c.Lineas) {
		return
	}
	c.Lineas[i].Cantidad = min(c.Lineas[i].Cantidad+paso, maxPorLinea)
	if c.Lineas[i].Cantidad <= 0 {
		c.Lineas = slices.Delete(c.Lineas, i, i+1)
	}
}

// cambiarSimple es el − n + de las bebidas y los acompañamientos en el menú: el renglón del
// producto solo (sin extras ni aclaración).
func (c *carrito) cambiarSimple(producto int32, paso int) {
	solo := lineaCarrito{Producto: producto, Tamano: pedidos.TamanoSimple, Cantidad: 1}
	for i, l := range c.Lineas {
		if mismoArmado(l, solo) {
			c.cambiar(i, paso)
			return
		}
	}
	if paso > 0 {
		c.agregar(solo)
	}
}

// cantidadDe cuenta cuántas unidades de ese producto hay en el carrito (en todos sus renglones).
func (c carrito) cantidadDe(producto int32) int {
	n := 0
	for _, l := range c.Lineas {
		if l.Promo == 0 && l.Producto == producto {
			n += l.Cantidad
		}
	}
	return n
}

func (c carrito) unidades() int {
	n := 0
	for _, l := range c.Lineas {
		n += l.Cantidad
	}
	return n
}

func mismoArmado(a, b lineaCarrito) bool {
	if a.Promo != b.Promo || !slices.EqualFunc(a.Elegidas, b.Elegidas, func(x, y elegidaCarrito) bool {
		return x.Producto == y.Producto && x.Tamano == y.Tamano && slices.Equal(x.Extras, y.Extras)
	}) {
		return false
	}
	if a.Producto != b.Producto || a.Tamano != b.Tamano || a.Aclaraciones != b.Aclaraciones || len(a.Extras) != len(b.Extras) {
		return false
	}
	ordenar := func(e []extraCarrito) []extraCarrito {
		e = slices.Clone(e)
		slices.SortFunc(e, func(x, y extraCarrito) int { return int(x.ID - y.ID) })
		return e
	}
	return slices.Equal(ordenar(a.Extras), ordenar(b.Extras))
}

// nuevoItem pasa un renglón al formato de pedidos (para cotizarlo o crearlo).
func (l lineaCarrito) nuevoItem() pedidos.NuevoItem {
	if l.Promo != 0 {
		np := &pedidos.NuevaPromo{PromoID: l.Promo}
		for _, e := range l.Elegidas {
			np.Elegidas = append(np.Elegidas, pedidos.Elegida{ProductoID: e.Producto, Tamano: e.Tamano, Extras: e.Extras})
		}
		return pedidos.NuevoItem{Tamano: pedidos.TamanoSimple, Cantidad: l.Cantidad, Promo: np}
	}
	it := pedidos.NuevoItem{ProductoID: l.Producto, Tamano: l.Tamano, Cantidad: l.Cantidad, Aclaraciones: l.Aclaraciones}
	for _, e := range l.Extras {
		it.Extras = append(it.Extras, pedidos.NuevoExtra{ExtraID: e.ID, Cantidad: e.Cantidad})
	}
	return it
}
