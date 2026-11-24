package handlers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	sqlc "forestburger/db/sqlc"
	"forestburger/pedidos"
	"forestburger/views"
)

// "Armá tu promo" (Figma 2b y 2c): el cliente elige las hamburguesas de la promo (puede repetir),
// el tamaño de cada una si la promo es a elección, y sus extras. Lo elegido viaja oculto en el form
// (slot_N_producto, slot_N_tamano, slot_N_extras) y cada toque pide el form recalculado. Al
// agregarla, va al carrito como un renglón; el precio lo pone pedidos (el de la promo más los
// extras).

const maxSlots = 10

// PromoArmar: GET /promo/{id}
func (a *API) PromoArmar(w http.ResponseWriter, r *http.Request) {
	a.responderPromo(w, r, nil, false)
}

// ArmarPromo: POST /promo/{id}/armar (accion: mas, menos, tamano, extra)
func (a *API) ArmarPromo(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	a.responderPromo(w, r, slotsDeForm(r), true)
}

// AgregarPromo: POST /carrito/promo
// Suma la promo armada al carrito (si está completa y se puede pedir) y vuelve al menú.
func (a *API) AgregarPromo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	id, _ := strconv.Atoi(r.FormValue("promo"))
	l := lineaCarrito{Promo: int32(id), Tamano: pedidos.TamanoSimple, Cantidad: 1, Elegidas: slotsDeForm(r)}
	abierto, _, _, err := a.estadoLocal(ctx)
	if err != nil {
		http.Error(w, "No se pudo guardar", http.StatusInternalServerError)
		return
	}
	if _, err := pedidos.CalcularPresupuesto(ctx, a.db, pedidos.NuevoPedido{Items: []pedidos.NuevoItem{l.nuevoItem()}}); abierto && err == nil {
		c := leerCarrito(r)
		c.agregar(l)
		guardarCarrito(w, c)
	}
	http.Redirect(w, r, "/menu", http.StatusSeeOther)
}

// responderPromo arma la pantalla. slots es lo elegido hasta ahora (nil al entrar); con armar,
// se aplica la acción del botón tocado y se devuelve solo el form.
func (a *API) responderPromo(w http.ResponseWriter, r *http.Request, slots []elegidaCarrito, armar bool) {
	ctx := r.Context()
	id, err := pathID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	pr, err := a.q.GetPromo(ctx, id)
	hoy := int32(time.Now().In(pedidos.Zona).Weekday())
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (!pr.Activa || !slices.Contains(pr.Dias, hoy))) {
		http.Redirect(w, r, "/menu", http.StatusSeeOther) // ya no está, o no es de hoy
		return
	}
	if err != nil {
		http.Error(w, "No se pudo cargar la promo", http.StatusInternalServerError)
		return
	}
	opciones, extras, err := a.opcionesPromo(ctx, pr.ID)
	if err != nil {
		http.Error(w, "No se pudo cargar la promo", http.StatusInternalServerError)
		return
	}
	abierto, _, _, err := a.estadoLocal(ctx)
	if err != nil {
		http.Error(w, "No se pudo cargar la promo", http.StatusInternalServerError)
		return
	}
	if armar {
		slots = aplicarAccionPromo(r, pr, slots, opciones, extras)
	}
	v := vistaPromo(pr, slots, opciones, extras)
	v.Abierto = abierto
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if armar {
		views.FormPromoArmar(v).Render(ctx, w)
		return
	}
	views.PromoArmarPage(v).Render(ctx, w)
}

// opcionesPromo trae las hamburguesas de la promo (por nombre) y los extras disponibles.
func (a *API) opcionesPromo(ctx context.Context, promoID int32) ([]sqlc.Producto, []sqlc.Extra, error) {
	ids, err := a.q.ListProductosDePromo(ctx, promoID)
	if err != nil {
		return nil, nil, err
	}
	todos, err := a.q.ListProductos(ctx)
	if err != nil {
		return nil, nil, err
	}
	var opciones []sqlc.Producto
	for _, p := range todos {
		if slices.Contains(ids, p.ID) {
			opciones = append(opciones, p)
		}
	}
	slices.SortFunc(opciones, func(x, y sqlc.Producto) int { return strings.Compare(x.Nombre, y.Nombre) })
	lista, err := a.q.ListExtras(ctx)
	if err != nil {
		return nil, nil, err
	}
	var extras []sqlc.Extra
	for _, e := range lista {
		if e.Disponible {
			extras = append(extras, e)
		}
	}
	return opciones, extras, nil
}

// slotsDeForm lee las hamburguesas elegidas del form (slot_0_…, slot_1_…).
func slotsDeForm(r *http.Request) []elegidaCarrito {
	var slots []elegidaCarrito
	for i := 0; i < maxSlots; i++ {
		p, err := strconv.Atoi(r.FormValue(fmt.Sprintf("slot_%d_producto", i)))
		if err != nil {
			break
		}
		s := elegidaCarrito{Producto: int32(p), Tamano: r.FormValue(fmt.Sprintf("slot_%d_tamano", i))}
		for _, t := range strings.Split(r.FormValue(fmt.Sprintf("slot_%d_extras", i)), ",") {
			if e, err := strconv.Atoi(t); err == nil && !slices.Contains(s.Extras, int32(e)) {
				s.Extras = append(s.Extras, int32(e))
			}
		}
		slices.Sort(s.Extras)
		slots = append(slots, s)
	}
	return slots
}

// aplicarAccionPromo aplica el botón tocado: sumar o sacar una hamburguesa, cambiar el tamaño de
// una (si la promo es a elección) o prender/apagar un extra.
func aplicarAccionPromo(r *http.Request, pr sqlc.Promo, slots []elegidaCarrito, opciones []sqlc.Producto, extras []sqlc.Extra) []elegidaCarrito {
	producto, _ := strconv.Atoi(r.FormValue("producto"))
	i, _ := strconv.Atoi(r.FormValue("i"))
	buscar := func(id int32) (sqlc.Producto, bool) {
		for _, o := range opciones {
			if o.ID == id {
				return o, true
			}
		}
		return sqlc.Producto{}, false
	}
	switch r.FormValue("accion") {
	case "mas":
		if p, ok := buscar(int32(producto)); ok && p.Disponible && len(slots) < int(pr.Cantidad) {
			slots = append(slots, elegidaCarrito{Producto: p.ID, Tamano: tamanoInicial(pr)})
		}
	case "menos":
		for j := len(slots) - 1; j >= 0; j-- {
			if slots[j].Producto == int32(producto) {
				slots = slices.Delete(slots, j, j+1)
				break
			}
		}
	case "tamano":
		t := r.FormValue("t")
		if i >= 0 && i < len(slots) && pr.Tamano == "cliente" && (t == pedidos.TamanoSimple || t == pedidos.TamanoDoble) {
			if p, ok := buscar(slots[i].Producto); ok && (t == pedidos.TamanoSimple || p.PrecioDoble.Valid) {
				slots[i].Tamano = t
			}
		}
	case "extra":
		e, _ := strconv.Atoi(r.FormValue("extra"))
		if i >= 0 && i < len(slots) && slices.ContainsFunc(extras, func(x sqlc.Extra) bool { return x.ID == int32(e) }) {
			if j := slices.Index(slots[i].Extras, int32(e)); j >= 0 {
				slots[i].Extras = slices.Delete(slots[i].Extras, j, j+1)
			} else {
				slots[i].Extras = append(slots[i].Extras, int32(e))
				slices.Sort(slots[i].Extras)
			}
		}
	}
	return slots
}

// tamanoInicial: el de la promo, o simple si el cliente elige.
func tamanoInicial(pr sqlc.Promo) string {
	if pr.Tamano == pedidos.TamanoDoble {
		return pedidos.TamanoDoble
	}
	return pedidos.TamanoSimple
}

// vistaPromo arma la pantalla con lo elegido: cuántas de cada una, cada hamburguesa con su tamaño
// y extras, y el total (el precio de la promo más los extras).
func vistaPromo(pr sqlc.Promo, slots []elegidaCarrito, opciones []sqlc.Producto, extras []sqlc.Extra) views.VistaPromoArmar {
	v := views.VistaPromoArmar{
		ID: pr.ID, Nombre: pr.Nombre, Foto: pr.ImagenUrl.String, Precio: pr.Precio,
		Cantidad: int(pr.Cantidad), Elegidas: len(slots), ElegirTamano: pr.Tamano == "cliente",
		Texto: textoArmarPromo(pr),
	}
	porID := map[int32]sqlc.Producto{}
	for _, o := range opciones {
		porID[o.ID] = o
		cuantas := 0
		for _, s := range slots {
			if s.Producto == o.ID {
				cuantas++
			}
		}
		v.Opciones = append(v.Opciones, views.OpcionPromo{ID: o.ID, Nombre: o.Nombre, Descripcion: o.Descripcion.String, Cuantas: cuantas, Agotado: !o.Disponible})
	}
	precio, _ := pedidos.CentavosDe(pr.Precio)
	var extrasTotal pedidos.Centavos
	for _, s := range slots {
		p := porID[s.Producto]
		sv := views.SlotPromo{Producto: s.Producto, Nombre: p.Nombre, Tamano: s.Tamano, TieneDoble: p.PrecioDoble.Valid}
		if sv.Tamano == "" {
			sv.Tamano = tamanoInicial(pr)
		}
		var suma pedidos.Centavos
		var ids []string
		for _, e := range extras {
			activo := slices.Contains(s.Extras, e.ID)
			sv.Extras = append(sv.Extras, views.ExtraSlot{ID: e.ID, Nombre: e.Nombre, Precio: e.Precio, Activo: activo})
			if activo {
				pe, _ := pedidos.CentavosDe(e.Precio)
				suma += pe
				ids = append(ids, strconv.Itoa(int(e.ID)))
			}
		}
		sv.ExtrasIDs = strings.Join(ids, ",")
		if suma > 0 {
			sv.ExtrasTotal = suma.Texto()
		}
		extrasTotal += suma
		v.Slots = append(v.Slots, sv)
	}
	if extrasTotal > 0 {
		v.ExtrasTotal = extrasTotal.Texto()
	}
	v.Total = (precio + extrasTotal).Texto()
	return v
}

// textoArmarPromo: "Elegí 2 hamburguesas simples. Podés repetir la misma." / "Elegí 2
// hamburguesas, simples o dobles: el precio es el mismo. Podés repetir la misma."
func textoArmarPromo(pr sqlc.Promo) string {
	n := int(pr.Cantidad)
	sust, tam := "hamburguesas", map[string]string{"simple": "simples", "doble": "dobles"}[pr.Tamano]
	if n == 1 {
		sust, tam = "hamburguesa", pr.Tamano
	}
	var t string
	if pr.Tamano == "cliente" {
		t = fmt.Sprintf("Elegí %d %s, simples o dobles: el precio es el mismo.", n, sust)
		if n == 1 {
			t = "Elegí 1 hamburguesa, simple o doble: el precio es el mismo."
		}
	} else {
		t = fmt.Sprintf("Elegí %d %s %s.", n, sust, tam)
	}
	if n > 1 {
		t += " Podés repetir la misma."
	}
	return t
}
