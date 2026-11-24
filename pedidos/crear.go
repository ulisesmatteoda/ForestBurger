package pedidos

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	sqlc "forestburger/db/sqlc"
)

// Valores que se guardan en la base (los CHECK de db/schema.sql aceptan solo estos).
const (
	OrigenWeb      = "web"
	OrigenWhatsapp = "whatsapp"
	OrigenTelefono = "telefono"
	OrigenLocal    = "local"

	EntregaRetiro   = "retiro"
	EntregaDelivery = "delivery"

	PagoEfectivo      = "efectivo"
	PagoTransferencia = "transferencia"

	EstadoPendiente  = "pendiente"
	EstadoConfirmado = "confirmado"
	EstadoEntregado  = "entregado"
	EstadoCancelado  = "cancelado"
	// EventoRecibido es el primer renglón del historial de todo pedido.
	EventoRecibido = "recibido"

	TamanoSimple = "simple"
	TamanoDoble  = "doble"
)

const (
	largoToken        = 8
	letrasToken       = "abcdefghijkmnpqrstuvwxyz23456789" // sin 0/o ni 1/l, que se confunden al dictarlo
	largoMaxCliente   = 100
	largoMaxDireccion = 200
)

// NuevoPedido es lo que hace falta para crear un pedido. No lleva precios: los pone Crear con los
// de la base.
type NuevoPedido struct {
	Cliente, Telefono string
	Origen            string
	Entrega           string
	Direccion         string
	Pago              string
	Notas             string
	DescuentoPct      int      // un porcentaje del subtotal...
	DescuentoFijo     Centavos // ...o un monto fijo (no los dos)
	DescuentoMotivo   string
	Items             []NuevoItem

	// Opcionales. Estado vacío es pendiente (lo que llega de la web); "Cargar pedido" lo crea
	// confirmado. CreadoEn vacío es ahora; el seed lo usa para armar pedidos de días anteriores.
	Estado    string
	DemoraMin int
	CreadoEn  time.Time
}

// NuevoItem es un producto del pedido, con su tamaño, cuántos, y los extras de cada unidad.
type NuevoItem struct {
	ProductoID int32
	Tamano     string
	Cantidad   int
	Extras     []NuevoExtra
}

type NuevoExtra struct {
	ExtraID  int32
	Cantidad int
}

// ErrPedidoInvalido envuelve los errores de datos (para mostrarlos al usuario); los demás son de
// la base.
var ErrPedidoInvalido = errors.New("pedido inválido")

func invalido(formato string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrPedidoInvalido, fmt.Sprintf(formato, args...))
}

// Crear valida el pedido, toma los precios de la base, hace las cuentas y lo guarda con sus ítems,
// sus extras y su historial, todo en una transacción.
func Crear(ctx context.Context, db *sql.DB, n NuevoPedido) (sqlc.Pedido, error) {
	if err := validar(&n); err != nil {
		return sqlc.Pedido{}, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return sqlc.Pedido{}, err
	}
	defer tx.Rollback() // no hace nada si ya se hizo Commit
	q := sqlc.New(db).WithTx(tx)

	items, c, err := cotizar(ctx, q, n)
	if err != nil {
		return sqlc.Pedido{}, err
	}

	token, err := nuevoToken()
	if err != nil {
		return sqlc.Pedido{}, err
	}
	demora := sql.NullInt16{}
	if n.DemoraMin > 0 {
		demora = sql.NullInt16{Int16: int16(n.DemoraMin), Valid: true}
	}
	p, err := q.CreatePedido(ctx, sqlc.CreatePedidoParams{
		CreadoEn:        n.CreadoEn,
		Cliente:         n.Cliente,
		Telefono:        n.Telefono,
		Origen:          n.Origen,
		Entrega:         n.Entrega,
		Direccion:       n.Direccion,
		Pago:            n.Pago,
		Notas:           n.Notas,
		Estado:          n.Estado,
		DemoraMin:       demora,
		Subtotal:        c.Subtotal.Texto(),
		DescuentoPct:    int16(n.DescuentoPct),
		DescuentoMotivo: n.DescuentoMotivo,
		Descuento:       c.Descuento.Texto(),
		CostoEnvio:      c.Envio.Texto(),
		Total:           c.Total.Texto(),
		Token:           token,
	})
	if err != nil {
		return sqlc.Pedido{}, err
	}
	for _, it := range items {
		it.item.PedidoID = p.ID
		guardado, err := q.CreateItemPedido(ctx, it.item)
		if err != nil {
			return sqlc.Pedido{}, err
		}
		for _, e := range it.extras {
			e.ItemPedidoID = guardado.ID
			if err := q.CreateItemPedidoExtra(ctx, e); err != nil {
				return sqlc.Pedido{}, err
			}
		}
	}
	if err := guardarEventosIniciales(ctx, q, p); err != nil {
		return sqlc.Pedido{}, err
	}
	return p, tx.Commit()
}

// validar revisa lo que no depende de la base y completa los valores por defecto.
func validar(n *NuevoPedido) error {
	n.Cliente = strings.TrimSpace(n.Cliente)
	n.Direccion = strings.TrimSpace(n.Direccion)
	switch {
	case n.Cliente == "":
		return invalido("falta el nombre del cliente")
	case len([]rune(n.Cliente)) > largoMaxCliente:
		return invalido("el nombre del cliente es muy largo")
	case n.Origen != OrigenWeb && n.Origen != OrigenWhatsapp && n.Origen != OrigenTelefono && n.Origen != OrigenLocal:
		return invalido("origen inválido %q", n.Origen)
	case n.Entrega != EntregaRetiro && n.Entrega != EntregaDelivery:
		return invalido("entrega inválida %q", n.Entrega)
	case n.Entrega == EntregaDelivery && n.Direccion == "":
		return invalido("falta la dirección para el delivery")
	case len([]rune(n.Direccion)) > largoMaxDireccion:
		return invalido("la dirección es muy larga")
	case n.Pago != PagoEfectivo && n.Pago != PagoTransferencia:
		return invalido("forma de pago inválida %q", n.Pago)
	case n.DescuentoPct < 0 || n.DescuentoPct > 100:
		return invalido("el descuento tiene que ser de 0 a 100%%")
	case n.DescuentoFijo < 0:
		return invalido("el descuento no puede ser negativo")
	case n.DescuentoPct > 0 && n.DescuentoFijo > 0:
		return invalido("el descuento es en %% o en $, no los dos")
	case len(n.Items) == 0:
		return invalido("el pedido no tiene productos")
	case n.DemoraMin < 0:
		return invalido("demora inválida")
	}
	if n.Entrega == EntregaRetiro {
		n.Direccion = "" // un retiro no lleva dirección aunque haya quedado escrita
	}
	if n.Estado == "" {
		n.Estado = EstadoPendiente
	}
	if n.Estado != EstadoPendiente && n.Estado != EstadoConfirmado && n.Estado != EstadoEntregado && n.Estado != EstadoCancelado {
		return invalido("estado inválido %q", n.Estado)
	}
	if n.CreadoEn.IsZero() {
		n.CreadoEn = time.Now()
	}
	return validarItems(n.Items)
}

// validarItems revisa los renglones sin ir a la base (lo usan Crear y Presupuesto).
func validarItems(items []NuevoItem) error {
	for _, it := range items {
		if it.Cantidad < 1 {
			return invalido("las cantidades tienen que ser al menos 1")
		}
		if it.Tamano != TamanoSimple && it.Tamano != TamanoDoble {
			return invalido("tamaño inválido %q", it.Tamano)
		}
		for _, e := range it.Extras {
			if e.Cantidad < 1 {
				return invalido("las cantidades de los extras tienen que ser al menos 1")
			}
		}
	}
	return nil
}

// cotizar busca los precios en la base (productos, extras y el envío de Ajustes) y hace las
// cuentas. Es lo mismo para el presupuesto que se ve al armar el pedido y para el que se guarda.
func cotizar(ctx context.Context, q *sqlc.Queries, n NuevoPedido) ([]itemArmado, Cuentas, error) {
	items, lineas, err := armarItems(ctx, q, n.Items)
	if err != nil {
		return nil, Cuentas{}, err
	}
	envio := Centavos(0)
	if n.Entrega == EntregaDelivery {
		aj, err := q.GetAjustes(ctx)
		if err != nil {
			return nil, Cuentas{}, err
		}
		if envio, err = CentavosDe(aj.CostoEnvio); err != nil {
			return nil, Cuentas{}, err
		}
	}
	return items, Calcular(lineas, Descuento{Pct: n.DescuentoPct, Fijo: n.DescuentoFijo}, envio), nil
}

// itemArmado es un ítem listo para guardar, con la copia del nombre y del precio.
type itemArmado struct {
	item   sqlc.CreateItemPedidoParams
	extras []sqlc.CreateItemPedidoExtraParams
}

// armarItems busca cada producto y extra en la base y copia su nombre y su precio de ahora.
func armarItems(ctx context.Context, q *sqlc.Queries, nuevos []NuevoItem) ([]itemArmado, []Linea, error) {
	items := make([]itemArmado, 0, len(nuevos))
	lineas := make([]Linea, 0, len(nuevos))
	for _, n := range nuevos {
		prod, err := q.GetProducto(ctx, n.ProductoID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, invalido("el producto %d no existe", n.ProductoID)
		}
		if err != nil {
			return nil, nil, err
		}
		if !prod.Disponible {
			return nil, nil, invalido("%s está agotado", prod.Nombre)
		}
		precioTexto := prod.Precio
		if n.Tamano == TamanoDoble {
			if !prod.PrecioDoble.Valid {
				return nil, nil, invalido("%s no tiene versión doble", prod.Nombre)
			}
			precioTexto = prod.PrecioDoble.String
		}
		precio, err := CentavosDe(precioTexto)
		if err != nil {
			return nil, nil, err
		}

		it := itemArmado{item: sqlc.CreateItemPedidoParams{
			ProductoID:     sql.NullInt32{Int32: prod.ID, Valid: true},
			Nombre:         prod.Nombre,
			Tamano:         n.Tamano,
			Cantidad:       int16(n.Cantidad),
			PrecioUnitario: precio.Texto(),
		}}
		linea := Linea{Cantidad: n.Cantidad, Precio: precio}
		for _, ne := range n.Extras {
			ext, err := q.GetExtra(ctx, ne.ExtraID)
			if errors.Is(err, sql.ErrNoRows) {
				return nil, nil, invalido("el extra %d no existe", ne.ExtraID)
			}
			if err != nil {
				return nil, nil, err
			}
			if !ext.Disponible {
				return nil, nil, invalido("%s no está disponible", ext.Nombre)
			}
			precioExtra, err := CentavosDe(ext.Precio)
			if err != nil {
				return nil, nil, err
			}
			it.extras = append(it.extras, sqlc.CreateItemPedidoExtraParams{
				ExtraID:        sql.NullInt32{Int32: ext.ID, Valid: true},
				Nombre:         ext.Nombre,
				Cantidad:       int16(ne.Cantidad),
				PrecioUnitario: precioExtra.Texto(),
			})
			linea.Extras = append(linea.Extras, LineaExtra{Cantidad: ne.Cantidad, Precio: precioExtra})
		}
		items = append(items, it)
		lineas = append(lineas, linea)
	}
	return items, lineas, nil
}

// guardarEventosIniciales arma el historial con el que nace el pedido: siempre "recibido", y si ya
// entra confirmado (o más adelante, en el seed), los pasos hasta su estado.
func guardarEventosIniciales(ctx context.Context, q *sqlc.Queries, p sqlc.Pedido) error {
	momento := p.CreadoEn
	evento := func(estado, detalle string) error {
		err := q.CreateEventoPedido(ctx, sqlc.CreateEventoPedidoParams{PedidoID: p.ID, CreadoEn: momento, Estado: estado, Detalle: detalle})
		momento = momento.Add(2 * time.Minute)
		return err
	}
	if err := evento(EventoRecibido, ""); err != nil {
		return err
	}
	demora := ""
	if p.DemoraMin.Valid {
		demora = fmt.Sprintf("%d min", p.DemoraMin.Int16)
	}
	switch p.Estado {
	case EstadoConfirmado:
		return evento(EstadoConfirmado, demora)
	case EstadoEntregado:
		if err := evento(EstadoConfirmado, demora); err != nil {
			return err
		}
		momento = momento.Add(30 * time.Minute)
		return evento(EstadoEntregado, "")
	case EstadoCancelado:
		return evento(EstadoCancelado, "")
	}
	return nil
}

// nuevoToken arma el código del link de seguimiento (forestburger.com.ar/pedido/q9w1e5yh).
func nuevoToken() (string, error) {
	azar := make([]byte, largoToken)
	if _, err := rand.Read(azar); err != nil {
		return "", err
	}
	for i, b := range azar {
		azar[i] = letrasToken[int(b)%len(letrasToken)]
	}
	return string(azar), nil
}
