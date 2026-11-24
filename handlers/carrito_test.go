package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCarrito_AgregarYMismoArmado(t *testing.T) {
	var c carrito
	classic := lineaCarrito{Producto: 1, Tamano: "simple", Cantidad: 1, Extras: []extraCarrito{{ID: 2, Cantidad: 1}, {ID: 1, Cantidad: 1}}, Aclaraciones: "sin cebolla"}
	c.agregar(classic)
	// La misma (con los extras en otro orden y espacios de más) suma cantidad.
	c.agregar(lineaCarrito{Producto: 1, Tamano: "simple", Cantidad: 2, Extras: []extraCarrito{{ID: 1, Cantidad: 1}, {ID: 2, Cantidad: 1}}, Aclaraciones: " sin cebolla "})
	if len(c.Lineas) != 1 || c.Lineas[0].Cantidad != 3 {
		t.Fatalf("la misma burger tendría que sumar cantidad: %+v", c.Lineas)
	}
	// Otra aclaración, otro tamaño o sin extras es otro renglón.
	c.agregar(lineaCarrito{Producto: 1, Tamano: "simple", Cantidad: 1})
	c.agregar(lineaCarrito{Producto: 1, Tamano: "doble", Cantidad: 1, Extras: classic.Extras, Aclaraciones: "sin cebolla"})
	if len(c.Lineas) != 3 || c.cantidadDe(1) != 5 || c.unidades() != 5 {
		t.Errorf("renglones = %+v", c.Lineas)
	}
}

func TestCarrito_CambiarYRapido(t *testing.T) {
	var c carrito
	c.cambiarSimple(7, 1)
	c.cambiarSimple(7, 1)
	if len(c.Lineas) != 1 || c.Lineas[0].Cantidad != 2 {
		t.Fatalf("dos gaseosas = %+v", c.Lineas)
	}
	c.cambiar(0, -1)
	c.cambiar(0, -1) // en 0 se va
	if len(c.Lineas) != 0 {
		t.Errorf("el − en 1 tendría que sacar el renglón: %+v", c.Lineas)
	}
	c.cambiarSimple(7, -1) // restar lo que no está no hace nada
	c.cambiar(5, 1)        // un índice que no existe tampoco
	if len(c.Lineas) != 0 {
		t.Errorf("no tendría que haber nada: %+v", c.Lineas)
	}
}

func TestCarrito_CookieIdaYVuelta(t *testing.T) {
	c := carrito{Lineas: []lineaCarrito{{Producto: 3, Tamano: "doble", Cantidad: 2, Extras: []extraCarrito{{ID: 1, Cantidad: 2}}, Aclaraciones: "bien cocida"}}}
	rec := httptest.NewRecorder()
	guardarCarrito(rec, c)
	req := httptest.NewRequest(http.MethodGet, "/carrito", nil)
	for _, ck := range rec.Result().Cookies() {
		req.AddCookie(ck)
	}
	got := leerCarrito(req)
	if len(got.Lineas) != 1 || !mismoArmado(got.Lineas[0], c.Lineas[0]) || got.Lineas[0].Cantidad != 2 {
		t.Errorf("ida y vuelta: %+v", got)
	}
}

func TestCarrito_CookieRotaOTocadaSeIgnora(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/carrito", nil)
	req.AddCookie(&http.Cookie{Name: cookieCarrito, Value: "no-es-base64!!"})
	if c := leerCarrito(req); len(c.Lineas) != 0 {
		t.Errorf("una cookie rota es un carrito vacío: %+v", c)
	}
	// Renglones imposibles (cantidad 0 o 500, tamaño inventado) se descartan.
	rec := httptest.NewRecorder()
	guardarCookieJSON(rec, cookieCarrito, carrito{Lineas: []lineaCarrito{{Producto: 1, Tamano: "simple", Cantidad: 0}, {Producto: 1, Tamano: "gigante", Cantidad: 1}, {Producto: 1, Tamano: "simple", Cantidad: 500}, {Producto: 2, Tamano: "simple", Cantidad: 1}}})
	req = httptest.NewRequest(http.MethodGet, "/carrito", nil)
	for _, ck := range rec.Result().Cookies() {
		req.AddCookie(ck)
	}
	if c := leerCarrito(req); len(c.Lineas) != 1 || c.Lineas[0].Producto != 2 {
		t.Errorf("solo tendría que quedar el renglón válido: %+v", c.Lineas)
	}
}
