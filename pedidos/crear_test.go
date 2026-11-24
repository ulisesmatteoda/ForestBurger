package pedidos

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	sqlc "forestburger/db/sqlc"
	"forestburger/internal/basetest"
)

// catalogo carga una hamburguesa con doble, una bebida sin doble, un extra disponible y uno
// apagado, y un envío de $1.500.
type catalogo struct {
	classic, gaseosa sqlc.Producto
	cheddar, panceta sqlc.Extra
}

func cargarCatalogo(t *testing.T, db *sql.DB) catalogo {
	t.Helper()
	ctx := context.Background()
	q := sqlc.New(db)
	var c catalogo
	var err error
	if c.classic, err = q.CreateProducto(ctx, sqlc.CreateProductoParams{Nombre: "Classic", Categoria: "hamburguesa", Precio: "14500", PrecioDoble: sql.NullString{String: "16500", Valid: true}}); err != nil {
		t.Fatal(err)
	}
	if c.gaseosa, err = q.CreateProducto(ctx, sqlc.CreateProductoParams{Nombre: "Gaseosa lata", Categoria: "bebida", Precio: "3500"}); err != nil {
		t.Fatal(err)
	}
	if c.cheddar, err = q.CreateExtra(ctx, sqlc.CreateExtraParams{Nombre: "Cheddar", Precio: "1000", Disponible: true}); err != nil {
		t.Fatal(err)
	}
	if c.panceta, err = q.CreateExtra(ctx, sqlc.CreateExtraParams{Nombre: "Panceta", Precio: "1500", Disponible: false}); err != nil {
		t.Fatal(err)
	}
	if err := q.UpdateAjustes(ctx, sqlc.UpdateAjustesParams{DeliveryActivo: true, CostoEnvio: "1500"}); err != nil {
		t.Fatal(err)
	}
	return c
}

// pedidoFigma es el del Figma: Classic doble + cheddar, 2 gaseosas, delivery y 20% de descuento.
func pedidoFigma(c catalogo) NuevoPedido {
	return NuevoPedido{
		Cliente: "Juan Pablo", Telefono: "2236001122",
		Origen: OrigenWhatsapp, Entrega: EntregaDelivery, Direccion: "Güemes 2850", Pago: PagoEfectivo,
		DescuentoPct: 20, DescuentoMotivo: "amigos",
		Estado: EstadoConfirmado, DemoraMin: 30,
		Items: []NuevoItem{
			{ProductoID: c.classic.ID, Tamano: TamanoDoble, Cantidad: 1, Extras: []NuevoExtra{{ExtraID: c.cheddar.ID, Cantidad: 1}}},
			{ProductoID: c.gaseosa.ID, Tamano: TamanoSimple, Cantidad: 2},
		},
	}
}

func TestCrear_GuardaCuentasCopiasYHistorial(t *testing.T) {
	db := basetest.Abrir(t)
	c := cargarCatalogo(t, db)
	ctx := context.Background()

	p, err := Crear(ctx, db, pedidoFigma(c))
	if err != nil {
		t.Fatal(err)
	}
	if p.Subtotal != "24500.00" || p.Descuento != "4900.00" || p.CostoEnvio != "1500.00" || p.Total != "21100.00" {
		t.Errorf("cuentas guardadas = %s - %s + %s = %s; quiero 24500 - 4900 + 1500 = 21100", p.Subtotal, p.Descuento, p.CostoEnvio, p.Total)
	}
	if p.Estado != EstadoConfirmado || len(p.Token) != largoToken || p.VistoEn.Valid {
		t.Errorf("pedido guardado incorrecto: %+v", p)
	}

	q := sqlc.New(db)
	items, _ := q.ListItemsPedido(ctx, p.ID)
	if len(items) != 2 || items[0].Nombre != "Classic" || items[0].Tamano != TamanoDoble || items[0].PrecioUnitario != "16500.00" {
		t.Errorf("ítems guardados incorrectos: %+v", items)
	}
	extras, _ := q.ListExtrasItemsPedido(ctx, p.ID)
	if len(extras) != 1 || extras[0].Nombre != "Cheddar" || extras[0].PrecioUnitario != "1000.00" {
		t.Errorf("extras guardados incorrectos: %+v", extras)
	}
	eventos, _ := q.ListEventosPedido(ctx, p.ID)
	if len(eventos) != 2 || eventos[0].Estado != EventoRecibido || eventos[1].Estado != EstadoConfirmado || eventos[1].Detalle != "30 min" {
		t.Errorf("historial incorrecto: %+v", eventos)
	}
}

func TestCrear_RetiroSinEnvioNiDireccion(t *testing.T) {
	db := basetest.Abrir(t)
	c := cargarCatalogo(t, db)

	n := pedidoFigma(c)
	n.Entrega, n.DescuentoPct, n.Estado = EntregaRetiro, 0, ""
	p, err := Crear(context.Background(), db, n)
	if err != nil {
		t.Fatal(err)
	}
	if p.CostoEnvio != "0.00" || p.Direccion != "" || p.Total != "24500.00" || p.Estado != EstadoPendiente {
		t.Errorf("un retiro sin descuento: envío %s, dirección %q, total %s, estado %s", p.CostoEnvio, p.Direccion, p.Total, p.Estado)
	}
}

func TestCrear_Errores(t *testing.T) {
	casos := map[string]func(c catalogo, n *NuevoPedido){
		"sin cliente":            func(c catalogo, n *NuevoPedido) { n.Cliente = "  " },
		"sin productos":          func(c catalogo, n *NuevoPedido) { n.Items = nil },
		"delivery sin dirección": func(c catalogo, n *NuevoPedido) { n.Direccion = "" },
		"origen inválido":        func(c catalogo, n *NuevoPedido) { n.Origen = "paloma" },
		"cantidad cero":          func(c catalogo, n *NuevoPedido) { n.Items[0].Cantidad = 0 },
		"doble sin precio doble": func(c catalogo, n *NuevoPedido) { n.Items[1].Tamano = TamanoDoble },
		"extra apagado":          func(c catalogo, n *NuevoPedido) { n.Items[0].Extras[0].ExtraID = c.panceta.ID },
		"producto inexistente":   func(c catalogo, n *NuevoPedido) { n.Items[0].ProductoID = 9999 },
	}
	for nombre, romper := range casos {
		t.Run(nombre, func(t *testing.T) {
			db := basetest.Abrir(t)
			c := cargarCatalogo(t, db)
			n := pedidoFigma(c)
			romper(c, &n)

			if _, err := Crear(context.Background(), db, n); !errors.Is(err, ErrPedidoInvalido) {
				t.Errorf("quiero ErrPedidoInvalido, vino %v", err)
			}
			var hay int
			db.QueryRow(`SELECT count(*) FROM pedido`).Scan(&hay)
			if hay != 0 {
				t.Errorf("con un error no tendría que quedar nada guardado, hay %d pedidos", hay)
			}
		})
	}
}

// Un producto agotado (el switch de la lista de Productos) no se puede pedir.
func TestCrear_ProductoAgotado(t *testing.T) {
	db := basetest.Abrir(t)
	c := cargarCatalogo(t, db)
	if _, err := sqlc.New(db).UpdateDisponibleProducto(context.Background(), sqlc.UpdateDisponibleProductoParams{ID: c.gaseosa.ID, Disponible: false}); err != nil {
		t.Fatal(err)
	}
	_, err := Crear(context.Background(), db, pedidoFigma(c))
	if !errors.Is(err, ErrPedidoInvalido) || !strings.Contains(err.Error(), "Gaseosa lata está agotado") {
		t.Errorf("quiero el error de agotado, vino %v", err)
	}
}

// Borrar un producto o un extra no cambia los pedidos: la referencia queda en NULL y el pedido
// conserva el nombre y el precio que tenía.
func TestCrear_BorrarProductoYExtraNoCambiaElPedido(t *testing.T) {
	db := basetest.Abrir(t)
	c := cargarCatalogo(t, db)
	ctx := context.Background()
	p, err := Crear(ctx, db, pedidoFigma(c))
	if err != nil {
		t.Fatal(err)
	}

	q := sqlc.New(db)
	if err := q.DeleteProducto(ctx, c.classic.ID); err != nil {
		t.Fatalf("no se pudo borrar el producto usado en un pedido: %v", err)
	}
	if err := q.DeleteExtra(ctx, c.cheddar.ID); err != nil {
		t.Fatalf("no se pudo borrar el extra usado en un pedido: %v", err)
	}

	items, _ := q.ListItemsPedido(ctx, p.ID)
	if items[0].ProductoID.Valid || items[0].Nombre != "Classic" || items[0].PrecioUnitario != "16500.00" {
		t.Errorf("el ítem tendría que conservar nombre y precio, con la referencia en NULL: %+v", items[0])
	}
	extras, _ := q.ListExtrasItemsPedido(ctx, p.ID)
	if extras[0].ExtraID.Valid || extras[0].Nombre != "Cheddar" {
		t.Errorf("el extra tendría que conservar su nombre, con la referencia en NULL: %+v", extras[0])
	}
	if guardado, _ := q.GetPedido(ctx, p.ID); guardado.Total != "21100.00" {
		t.Errorf("el total no tendría que cambiar: %s", guardado.Total)
	}
}

// Una promo armada por el cliente: el precio de la promo más los extras de cada hamburguesa, con
// lo elegido en el detalle. Con "el cliente elige", la doble cuesta lo mismo.
func TestCrear_Promo(t *testing.T) {
	db := basetest.Abrir(t)
	c := cargarCatalogo(t, db)
	ctx := context.Background()
	q := sqlc.New(db)
	pr, err := q.CreatePromo(ctx, sqlc.CreatePromoParams{Nombre: "2 a elección", Precio: "29000", Cantidad: 2, Tamano: "cliente", Dias: []int32{0, 1, 2, 3, 4, 5, 6}, Activa: true})
	if err != nil {
		t.Fatal(err)
	}
	q.AddProductoAPromo(ctx, sqlc.AddProductoAPromoParams{PromoID: pr.ID, ProductoID: c.classic.ID})
	promo := func(elegidas ...Elegida) NuevoPedido {
		return NuevoPedido{Cliente: "Ulises", Origen: OrigenWeb, Entrega: EntregaRetiro, Pago: PagoEfectivo,
			Items: []NuevoItem{{Tamano: TamanoSimple, Cantidad: 1, Promo: &NuevaPromo{PromoID: pr.ID, Elegidas: elegidas}}}}
	}
	p, err := Crear(ctx, db, promo(Elegida{ProductoID: c.classic.ID, Tamano: TamanoDoble, Extras: []int32{c.cheddar.ID}}, Elegida{ProductoID: c.classic.ID, Tamano: TamanoSimple}))
	if err != nil {
		t.Fatal(err)
	}
	if p.Total != "30000.00" {
		t.Errorf("total = %s; quiero 29.000 + 1.000 de cheddar", p.Total)
	}
	items, _ := q.ListItemsPedido(ctx, p.ID)
	if len(items) != 1 || items[0].Nombre != "2 a elección" || !items[0].PromoID.Valid || items[0].Detalle != "Classic (doble) + Cheddar · Classic (simple)" {
		t.Errorf("ítem de la promo: %+v", items)
	}
	extras, _ := q.ListExtrasItemsPedido(ctx, p.ID)
	if len(extras) != 1 || extras[0].Nombre != "Cheddar (Classic)" {
		t.Errorf("extras: %+v", extras)
	}

	casos := map[string]NuevoPedido{
		"faltan hamburguesas": promo(Elegida{ProductoID: c.classic.ID, Tamano: TamanoSimple}),
		"una que no entra":    promo(Elegida{ProductoID: c.classic.ID, Tamano: TamanoSimple}, Elegida{ProductoID: c.gaseosa.ID, Tamano: TamanoSimple}),
		"un extra apagado":    promo(Elegida{ProductoID: c.classic.ID, Tamano: TamanoSimple, Extras: []int32{c.panceta.ID}}, Elegida{ProductoID: c.classic.ID, Tamano: TamanoSimple}),
	}
	for nombre, n := range casos {
		if _, err := Crear(ctx, db, n); !errors.Is(err, ErrPedidoInvalido) {
			t.Errorf("%s: quiero ErrPedidoInvalido, vino %v", nombre, err)
		}
	}
	// Pausada no se puede pedir.
	q.UpdateActivaPromo(ctx, sqlc.UpdateActivaPromoParams{ID: pr.ID, Activa: false})
	if _, err := Crear(ctx, db, promo(Elegida{ProductoID: c.classic.ID}, Elegida{ProductoID: c.classic.ID})); !errors.Is(err, ErrPedidoInvalido) || !strings.Contains(err.Error(), "no está disponible hoy") {
		t.Errorf("una promo pausada: %v", err)
	}
}
