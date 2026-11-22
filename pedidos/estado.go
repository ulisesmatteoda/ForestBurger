package pedidos

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
	_ "time/tzdata" // la zona de la Argentina viaja con el binario, aunque el servidor no la tenga

	sqlc "forestburger/db/sqlc"
)

// EstadoEnCamino: un delivery que ya salió del local.
const EstadoEnCamino = "en_camino"

// Las acciones del detalle de un pedido (son también el último tramo de su ruta en el panel).
const (
	AccionConfirmar = "confirmar"
	AccionRechazar  = "rechazar"
	AccionEnCamino  = "en-camino"
	AccionEntregado = "entregado"
	AccionCancelar  = "cancelar"
)

// DetalleRechazado distingue en el historial un pedido rechazado (cancelado antes de confirmarlo).
const DetalleRechazado = "rechazado"

// Zona es la hora del local: los "hoy", "ayer" y las horas del panel se cuentan en la Argentina,
// esté donde esté el servidor.
var Zona = cargarZona()

// NombreZona es la zona del local con el nombre que entiende Postgres (AT TIME ZONE).
const NombreZona = "America/Argentina/Buenos_Aires"

func cargarZona() *time.Location {
	z, err := time.LoadLocation(NombreZona)
	if err != nil {
		return time.FixedZone("ART", -3*60*60)
	}
	return z
}

var (
	// ErrEstadoCambio: el pedido ya no está en un estado que permita la acción (por ejemplo, lo
	// confirmó otra persona un segundo antes, o ya está entregado).
	ErrEstadoCambio = errors.New("este pedido ya cambió de estado")
	// ErrAccionInvalida: la acción no existe o no corresponde a este pedido (un "en camino" para
	// un retiro, una demora que no es positiva).
	ErrAccionInvalida = errors.New("acción inválida para este pedido")
)

// estadoTras dice a qué estado lleva cada acción. Desde qué estados se puede, lo dice
// AccionesPara.
var estadoTras = map[string]string{
	AccionConfirmar: EstadoConfirmado,
	AccionRechazar:  EstadoCancelado,
	AccionEnCamino:  EstadoEnCamino,
	AccionEntregado: EstadoEntregado,
	AccionCancelar:  EstadoCancelado,
}

// AccionesPara devuelve las acciones posibles para un pedido en su estado actual, en el orden en
// que se muestran (la principal al final). Un delivery confirmado sale "en camino"; un retiro
// confirmado se entrega directo.
func AccionesPara(estado, entrega string) []string {
	switch estado {
	case EstadoPendiente:
		return []string{AccionRechazar, AccionConfirmar}
	case EstadoConfirmado:
		if entrega == EntregaDelivery {
			return []string{AccionCancelar, AccionEnCamino}
		}
		return []string{AccionCancelar, AccionEntregado}
	case EstadoEnCamino:
		return []string{AccionCancelar, AccionEntregado}
	}
	return nil
}

// CambiarEstado aplica una acción del detalle y deja el renglón en el historial, en una
// transacción. demoraMin solo se usa al confirmar (es la que se le avisa al cliente).
func CambiarEstado(ctx context.Context, db *sql.DB, id int32, accion string, demoraMin int) (sqlc.Pedido, error) {
	hacia, ok := estadoTras[accion]
	if !ok {
		return sqlc.Pedido{}, ErrAccionInvalida
	}
	demora := sql.NullInt16{}
	if accion == AccionConfirmar {
		if demoraMin <= 0 {
			return sqlc.Pedido{}, ErrAccionInvalida
		}
		demora = sql.NullInt16{Int16: int16(demoraMin), Valid: true}
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return sqlc.Pedido{}, err
	}
	defer tx.Rollback() // no hace nada si ya se hizo Commit
	q := sqlc.New(db).WithTx(tx)

	actual, err := q.GetPedido(ctx, id)
	if err != nil {
		return sqlc.Pedido{}, err // sql.ErrNoRows si no existe
	}
	if !permitida(accion, actual) {
		if accion == AccionEnCamino && actual.Entrega == EntregaRetiro {
			return sqlc.Pedido{}, ErrAccionInvalida
		}
		return actual, ErrEstadoCambio
	}
	// El UPDATE va condicionado al estado que se acaba de leer: si alguien lo cambió en el medio,
	// no pisa nada.
	cambiado, err := q.CambiarEstadoPedido(ctx, sqlc.CambiarEstadoPedidoParams{
		ID: id, Desde: actual.Estado, Nuevo: hacia, Demora: demora,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return actual, ErrEstadoCambio
	}
	if err != nil {
		return sqlc.Pedido{}, err
	}
	detalle := ""
	switch accion {
	case AccionConfirmar:
		detalle = fmt.Sprintf("%d min", demoraMin)
	case AccionRechazar:
		detalle = DetalleRechazado
	}
	if err := q.CreateEventoPedido(ctx, sqlc.CreateEventoPedidoParams{
		PedidoID: id, CreadoEn: time.Now(), Estado: hacia, Detalle: detalle,
	}); err != nil {
		return sqlc.Pedido{}, err
	}
	return cambiado, tx.Commit()
}

// permitida dice si la acción corresponde al pedido tal como está ahora.
func permitida(accion string, p sqlc.Pedido) bool {
	for _, a := range AccionesPara(p.Estado, p.Entrega) {
		if a == accion {
			return true
		}
	}
	return false
}
