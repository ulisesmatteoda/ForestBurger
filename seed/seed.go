// Package seed carga datos de prueba para desarrollo: productos, extras, ajustes y pedidos de los
// últimos días, para poder ver el panel (lista de pedidos, métricas, gráfico) sin cargar todo a
// mano. Solo con DEV_DATOS_PRUEBA=1 y la base sin productos: en producción no se pone, y con la
// base que guarda los datos se carga una sola vez.
package seed

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"time"

	sqlc "forestburger/db/sqlc"
	"forestburger/pedidos"
)

const (
	diasDePedidos = 10
	// Semilla fija: siempre salen los mismos datos, así dos personas ven lo mismo.
	semilla = 24
)

// Resultado dice qué se cargó (todo en cero si no hizo nada).
type Resultado struct {
	Productos, Extras, Pedidos int
}

type productoSeed struct {
	nombre, categoria, descripcion string
	precio, precioDoble            string // precioDoble vacío: no tiene doble
}

var productos = []productoSeed{
	{"Cheese Burger", "hamburguesa", "Medallón de 120 gr., pan de batata y doble queso cheddar.", "14000", "16000"},
	{"Classic", "hamburguesa", "Medallón de 120 gr., pan de batata, doble cheddar, lechuga, cebolla, tomate y salsa Forest.", "14500", "16500"},
	{"Big Forest", "hamburguesa", "Medallón de 120 gr., pan de batata, doble cheddar, lechuga, cebolla, pepinillos y salsa Forest.", "14500", "16500"},
	{"Cheese Bacon", "hamburguesa", "Medallón de 120 gr., pan de batata, doble cheddar y doble panceta.", "15500", "17000"},
	{"Not Cheese", "veggie", "Medallón Not Burger, pan de batata y doble queso cheddar.", "14500", "16500"},
	{"Papas Forest", "acompañamiento", "Papas rústicas con cheddar y verdeo.", "6500", ""},
	{"Gaseosa lata", "bebida", "Lata de 354 ml.", "3500", ""},
	{"Gaseosa botella", "bebida", "Botella de 1,5 l.", "5000", ""},
	{"Agua mineral", "bebida", "Botella de 500 ml.", "2500", ""},
}

type extraSeed struct {
	nombre, precio string
	disponible     bool
}

var extras = []extraSeed{
	{"Cheddar", "1000", true},
	{"Medallón 120 gr.", "3500", true},
	{"Panceta", "1500", false}, // apagado, como en el Figma
	{"Porción de papas", "4000", true},
}

var (
	nombres     = []string{"Martina", "Juan Pablo", "Sofía", "Lucas", "Camila", "Ulises", "Valentina", "Tomás", "Agustina", "Mateo", "Lucía", "Franco", "Delfina", "Joaquín"}
	direcciones = []string{"Güemes 2850", "Alem 3120", "Colón 2450, 3° B", "Rawson 1980", "Olavarría 2200", "Moreno 3400"}
	motivos     = []string{"amigos", "cumpleaños", "cliente de siempre"}
)

// DatosPrueba carga todo si corresponde. Devuelve qué cargó.
func DatosPrueba(ctx context.Context, db *sql.DB) (Resultado, error) {
	if os.Getenv("DEV_DATOS_PRUEBA") != "1" {
		return Resultado{}, nil
	}
	q := sqlc.New(db)
	hay, err := q.CountProductos(ctx)
	if err != nil {
		return Resultado{}, err
	}
	if hay > 0 {
		return Resultado{}, nil // ya hay datos (de una corrida anterior o cargados a mano): no se tocan
	}

	var res Resultado
	hamburguesas, otros, err := cargarProductos(ctx, q)
	if err != nil {
		return res, err
	}
	res.Productos = len(productos)
	extrasDisponibles, err := cargarExtras(ctx, q)
	if err != nil {
		return res, err
	}
	res.Extras = len(extras)
	if err := q.UpdateAjustes(ctx, sqlc.UpdateAjustesParams{
		Whatsapp:        "2236001122",
		DireccionRetiro: "Güemes 2850, Mar del Plata",
		AliasCbu:        "forest.burger.mp",
		Titular:         "Forest Burger SRL",
		DeliveryActivo:  true,
		CostoEnvio:      "1500.00",
	}); err != nil {
		return res, err
	}
	res.Pedidos, err = cargarPedidos(ctx, db, hamburguesas, otros, extrasDisponibles)
	return res, err
}

func cargarProductos(ctx context.Context, q *sqlc.Queries) (hamburguesas, otros []int32, err error) {
	for _, p := range productos {
		doble := sql.NullString{}
		if p.precioDoble != "" {
			doble = sql.NullString{String: p.precioDoble, Valid: true}
		}
		creado, err := q.CreateProducto(ctx, sqlc.CreateProductoParams{
			Nombre:      p.nombre,
			Categoria:   p.categoria,
			Precio:      p.precio,
			PrecioDoble: doble,
			Descripcion: sql.NullString{String: p.descripcion, Valid: true},
		})
		if err != nil {
			return nil, nil, err
		}
		if p.precioDoble != "" {
			hamburguesas = append(hamburguesas, creado.ID)
		} else {
			otros = append(otros, creado.ID)
		}
	}
	return hamburguesas, otros, nil
}

func cargarExtras(ctx context.Context, q *sqlc.Queries) ([]int32, error) {
	var disponibles []int32
	for _, e := range extras {
		creado, err := q.CreateExtra(ctx, sqlc.CreateExtraParams{Nombre: e.nombre, Precio: e.precio, Disponible: e.disponible})
		if err != nil {
			return nil, err
		}
		if e.disponible {
			disponibles = append(disponibles, creado.ID)
		}
	}
	return disponibles, nil
}

// cargarPedidos arma pedidos de los últimos días, más los viernes y sábados, entre las 20 y la 1.
// Los de días anteriores quedan entregados (algún cancelado); los de hoy, mezclados.
func cargarPedidos(ctx context.Context, db *sql.DB, hamburguesas, otros, extrasIDs []int32) (int, error) {
	r := rand.New(rand.NewSource(semilla))
	ahora := time.Now()
	hoy := time.Date(ahora.Year(), ahora.Month(), ahora.Day(), 0, 0, 0, 0, ahora.Location())
	total := 0
	for dias := diasDePedidos - 1; dias >= 0; dias-- {
		dia := hoy.AddDate(0, 0, -dias)
		cantidad := 3 + r.Intn(3)
		if wd := dia.Weekday(); wd == time.Friday || wd == time.Saturday {
			cantidad += 2
		}
		for i := 0; i < cantidad; i++ {
			// Entre las 20:00 y la 01:00, en orden a lo largo de la noche.
			minutos := 20*60 + (i*300)/cantidad + r.Intn(300/cantidad)
			creado := dia.Add(time.Duration(minutos) * time.Minute)
			if dias == 0 && creado.After(ahora) {
				creado = ahora.Add(-time.Duration(cantidad-i) * 7 * time.Minute) // hoy: nada en el futuro
			}
			n := pedidoAlAzar(r, creado, hamburguesas, otros, extrasIDs)
			n.Estado = estadoAlAzar(r, dias == 0, i, cantidad)
			if n.Estado != pedidos.EstadoPendiente && n.Estado != pedidos.EstadoCancelado {
				n.DemoraMin = []int{20, 30, 45}[r.Intn(3)]
			}
			if _, err := pedidos.Crear(ctx, db, n); err != nil {
				return total, fmt.Errorf("pedido de prueba: %w", err)
			}
			total++
		}
	}
	return total, nil
}

func pedidoAlAzar(r *rand.Rand, creado time.Time, hamburguesas, otros, extrasIDs []int32) pedidos.NuevoPedido {
	n := pedidos.NuevoPedido{
		Cliente:  nombres[r.Intn(len(nombres))],
		Telefono: fmt.Sprintf("223%07d", r.Intn(10000000)),
		Origen:   []string{pedidos.OrigenWeb, pedidos.OrigenWeb, pedidos.OrigenWhatsapp, pedidos.OrigenTelefono, pedidos.OrigenLocal}[r.Intn(5)],
		Entrega:  pedidos.EntregaRetiro,
		Pago:     pedidos.PagoEfectivo,
		CreadoEn: creado,
	}
	if r.Intn(10) < 6 {
		n.Entrega = pedidos.EntregaDelivery
		n.Direccion = direcciones[r.Intn(len(direcciones))]
	}
	if r.Intn(10) < 5 {
		n.Pago = pedidos.PagoTransferencia
	}
	if r.Intn(10) == 0 {
		n.DescuentoPct = []int{10, 15, 20}[r.Intn(3)]
		n.DescuentoMotivo = motivos[r.Intn(len(motivos))]
	}
	// Una o dos hamburguesas (a veces con extras) y a veces algo para tomar o acompañar.
	for i := 0; i < 1+r.Intn(2); i++ {
		it := pedidos.NuevoItem{
			ProductoID: hamburguesas[r.Intn(len(hamburguesas))],
			Tamano:     []string{pedidos.TamanoSimple, pedidos.TamanoDoble}[r.Intn(2)],
			Cantidad:   1 + r.Intn(2),
		}
		if len(extrasIDs) > 0 && r.Intn(3) == 0 {
			it.Extras = []pedidos.NuevoExtra{{ExtraID: extrasIDs[r.Intn(len(extrasIDs))], Cantidad: 1}}
		}
		n.Items = append(n.Items, it)
	}
	if r.Intn(2) == 0 {
		n.Items = append(n.Items, pedidos.NuevoItem{ProductoID: otros[r.Intn(len(otros))], Tamano: pedidos.TamanoSimple, Cantidad: 1 + r.Intn(2)})
	}
	return n
}

// estadoAlAzar: los días anteriores casi todo entregado; hoy, los últimos pedidos todavía en curso.
func estadoAlAzar(r *rand.Rand, esHoy bool, i, cantidad int) string {
	if r.Intn(12) == 0 {
		return pedidos.EstadoCancelado
	}
	if !esHoy {
		return pedidos.EstadoEntregado
	}
	switch faltan := cantidad - i; {
	case faltan <= 2:
		return pedidos.EstadoPendiente
	case faltan <= 3:
		return pedidos.EstadoConfirmado
	default:
		return pedidos.EstadoEntregado
	}
}
