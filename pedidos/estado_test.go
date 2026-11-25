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

// nuevoPendiente crea un pedido pendiente con esa entrega.
func nuevoPendiente(t *testing.T, db *sql.DB, c catalogo, entrega string) sqlc.Pedido {
	t.Helper()
	n := pedidoFigma(c)
	n.Estado, n.DemoraMin, n.Entrega = "", 0, entrega
	p, err := Crear(context.Background(), db, n)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// historial devuelve los estados del historial en orden.
func historial(t *testing.T, db *sql.DB, id int32) []string {
	t.Helper()
	eventos, err := sqlc.New(db).ListEventosPedido(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	var estados []string
	for _, e := range eventos {
		estados = append(estados, e.Estado+":"+e.Detalle)
	}
	return estados
}

func TestCambiarEstado_DeliveryCompleto(t *testing.T) {
	db := basetest.Abrir(t)
	c := cargarCatalogo(t, db)
	p := nuevoPendiente(t, db, c, EntregaDelivery)
	ctx := context.Background()

	confirmado, err := CambiarEstado(ctx, db, p.ID, AccionConfirmar, 30)
	if err != nil || confirmado.Estado != EstadoConfirmado || confirmado.DemoraMin.Int16 != 30 {
		t.Fatalf("confirmar: %+v, %v", confirmado, err)
	}
	if got, err := CambiarEstado(ctx, db, p.ID, AccionEnCamino, 0); err != nil || got.Estado != EstadoEnCamino {
		t.Fatalf("en camino: %+v, %v", got, err)
	}
	entregado, err := CambiarEstado(ctx, db, p.ID, AccionEntregado, 0)
	if err != nil || entregado.Estado != EstadoEntregado || entregado.DemoraMin.Int16 != 30 {
		t.Fatalf("entregado (la demora no se pierde): %+v, %v", entregado, err)
	}

	quiero := []string{"recibido:", "confirmado:30 min", "en_camino:", "entregado:"}
	if got := historial(t, db, p.ID); len(got) != len(quiero) || got[1] != quiero[1] || got[3] != quiero[3] {
		t.Errorf("historial = %v, quiero %v", got, quiero)
	}
}

func TestCambiarEstado_RetiroPasaPorListo(t *testing.T) {
	db := basetest.Abrir(t)
	c := cargarCatalogo(t, db)
	p := nuevoPendiente(t, db, c, EntregaRetiro)
	ctx := context.Background()

	CambiarEstado(ctx, db, p.ID, AccionConfirmar, 45)
	if _, err := CambiarEstado(ctx, db, p.ID, AccionEnCamino, 0); !errors.Is(err, ErrAccionInvalida) {
		t.Errorf("un retiro no sale en camino: quiero ErrAccionInvalida, vino %v", err)
	}
	if _, err := CambiarEstado(ctx, db, p.ID, AccionEntregado, 0); !errors.Is(err, ErrEstadoCambio) {
		t.Errorf("un retiro confirmado primero queda listo: quiero ErrEstadoCambio, vino %v", err)
	}
	if got, err := CambiarEstado(ctx, db, p.ID, AccionListo, 0); err != nil || got.Estado != EstadoListo {
		t.Fatalf("listo para retirar: %+v, %v", got, err)
	}
	if got, err := CambiarEstado(ctx, db, p.ID, AccionEntregado, 0); err != nil || got.Estado != EstadoEntregado {
		t.Errorf("entregar un retiro listo: %+v, %v", got, err)
	}
	// Un delivery no pasa por "listo".
	d := nuevoPendiente(t, db, c, EntregaDelivery)
	CambiarEstado(ctx, db, d.ID, AccionConfirmar, 30)
	if _, err := CambiarEstado(ctx, db, d.ID, AccionListo, 0); !errors.Is(err, ErrAccionInvalida) {
		t.Errorf("un delivery no queda listo para retirar: %v", err)
	}
}

func TestCambiarEstado_RechazarYCancelar(t *testing.T) {
	db := basetest.Abrir(t)
	c := cargarCatalogo(t, db)
	ctx := context.Background()

	p := nuevoPendiente(t, db, c, EntregaDelivery)
	if got, err := CambiarEstado(ctx, db, p.ID, AccionRechazar, 0); err != nil || got.Estado != EstadoCancelado {
		t.Fatalf("rechazar: %+v, %v", got, err)
	}
	if h := historial(t, db, p.ID); h[len(h)-1] != "cancelado:"+DetalleRechazado {
		t.Errorf("el historial tendría que decir rechazado: %v", h)
	}

	q := nuevoPendiente(t, db, c, EntregaDelivery)
	CambiarEstado(ctx, db, q.ID, AccionConfirmar, 30)
	CambiarEstado(ctx, db, q.ID, AccionEnCamino, 0)
	if got, err := CambiarEstado(ctx, db, q.ID, AccionCancelar, 0); err != nil || got.Estado != EstadoCancelado {
		t.Errorf("cancelar un pedido en camino: %+v, %v", got, err)
	}
}

func TestCambiarEstado_PasosNoPermitidos(t *testing.T) {
	db := basetest.Abrir(t)
	c := cargarCatalogo(t, db)
	ctx := context.Background()
	p := nuevoPendiente(t, db, c, EntregaDelivery)

	casos := []struct {
		accion string
		demora int
		quiero error
	}{
		{AccionEntregado, 0, ErrEstadoCambio},   // un pendiente no se entrega sin confirmar
		{AccionCancelar, 0, ErrEstadoCambio},    // un pendiente se rechaza, no se cancela
		{AccionConfirmar, 0, ErrAccionInvalida}, // confirmar sin demora
		{"volar", 0, ErrAccionInvalida},
	}
	for _, c := range casos {
		if _, err := CambiarEstado(ctx, db, p.ID, c.accion, c.demora); !errors.Is(err, c.quiero) {
			t.Errorf("%s: quiero %v, vino %v", c.accion, c.quiero, err)
		}
	}

	// Sobre un pedido entregado ya no hay nada que hacer, ni rechazarlo.
	CambiarEstado(ctx, db, p.ID, AccionConfirmar, 30)
	CambiarEstado(ctx, db, p.ID, AccionEnCamino, 0)
	CambiarEstado(ctx, db, p.ID, AccionEntregado, 0)
	antes := len(historial(t, db, p.ID))
	for _, a := range []string{AccionRechazar, AccionCancelar, AccionConfirmar} {
		got, err := CambiarEstado(ctx, db, p.ID, a, 30)
		if !errors.Is(err, ErrEstadoCambio) || got.Estado != EstadoEntregado {
			t.Errorf("%s sobre un entregado: %v (estado real %s)", a, err, got.Estado)
		}
	}
	if len(historial(t, db, p.ID)) != antes {
		t.Error("una acción rechazada no tiene que sumar al historial")
	}

	if _, err := CambiarEstado(ctx, db, 9999, AccionConfirmar, 30); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("pedido inexistente: quiero sql.ErrNoRows, vino %v", err)
	}
}

func TestAccionesPara(t *testing.T) {
	casos := map[string][]string{
		EstadoPendiente + "/" + EntregaDelivery:  {AccionRechazar, AccionConfirmar},
		EstadoConfirmado + "/" + EntregaDelivery: {AccionCancelar, AccionEnCamino},
		EstadoConfirmado + "/" + EntregaRetiro:   {AccionCancelar, AccionListo},
		EstadoListo + "/" + EntregaRetiro:        {AccionCancelar, AccionEntregado},
		EstadoEnCamino + "/" + EntregaDelivery:   {AccionCancelar, AccionEntregado},
		EstadoEntregado + "/" + EntregaDelivery:  nil,
		EstadoCancelado + "/" + EntregaRetiro:    nil,
	}
	for clave, quiero := range casos {
		estado, entrega, _ := strings.Cut(clave, "/")
		got := AccionesPara(estado, entrega)
		if len(got) != len(quiero) || (len(got) > 0 && (got[0] != quiero[0] || got[1] != quiero[1])) {
			t.Errorf("%s: %v, quiero %v", clave, got, quiero)
		}
	}
}
