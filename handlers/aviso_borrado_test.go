package handlers

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	sqlc "forestburger/db/sqlc"
	"forestburger/pedidos"
)

// pedidoCon crea un pedido con ese producto (y ese extra) y lo deja en el estado pedido.
func pedidoCon(t *testing.T, db *sql.DB, q *sqlc.Queries, productoID, extraID int32, estado string) {
	t.Helper()
	p, err := pedidos.Crear(context.Background(), db, pedidos.NuevoPedido{
		Cliente: "Martina", Origen: pedidos.OrigenWeb, Entrega: pedidos.EntregaRetiro, Pago: pedidos.PagoEfectivo,
		Items: []pedidos.NuevoItem{{ProductoID: productoID, Tamano: pedidos.TamanoSimple, Cantidad: 1,
			Extras: []pedidos.NuevoExtra{{ExtraID: extraID, Cantidad: 1}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE pedido SET estado = $1 WHERE id = $2`, estado, p.ID); err != nil {
		t.Fatal(err)
	}
}

func TestBorrar_AvisaSiEstaEnPedidosEnCurso(t *testing.T) {
	mux, q, api := setupAPI(t)
	guardarAdminTest(t, q)
	conCookie := conSesion(mux, adminTest)
	prod, _ := q.CreateProducto(context.Background(), sqlc.CreateProductoParams{Nombre: "Classic", Categoria: "hamburguesa", Precio: "14500"})
	ext := crearExtra(t, q, "Cheddar", "1000.00", true)

	// Un pedido entregado y uno cancelado no cuentan: todavía no hay aviso.
	pedidoCon(t, api.db, q, prod.ID, ext.ID, pedidos.EstadoEntregado)
	pedidoCon(t, api.db, q, prod.ID, ext.ID, pedidos.EstadoCancelado)
	for _, ruta := range []string{"/admin/productos/" + idTexto(prod.ID) + "/borrar", "/admin/extras/" + idTexto(ext.ID) + "/borrar"} {
		if body := pedirPanel(conCookie, ruta, true).Body.String(); strings.Contains(body, "en curso") {
			t.Errorf("%s: sin pedidos en curso no tendría que avisar: %s", ruta, body)
		}
	}

	// Uno pendiente y uno confirmado sí.
	pedidoCon(t, api.db, q, prod.ID, ext.ID, pedidos.EstadoPendiente)
	pedidoCon(t, api.db, q, prod.ID, ext.ID, pedidos.EstadoConfirmado)
	for _, ruta := range []string{"/admin/productos/" + idTexto(prod.ID) + "/borrar", "/admin/extras/" + idTexto(ext.ID) + "/borrar"} {
		body := pedirPanel(conCookie, ruta, true).Body.String()
		if !strings.Contains(body, "Está en 2 pedidos en curso") || !strings.Contains(body, "BORRAR") {
			t.Errorf("%s: tendría que avisar de 2 pedidos en curso y dejar borrar igual: %s", ruta, body)
		}
	}
}
