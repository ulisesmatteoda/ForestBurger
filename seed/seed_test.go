package seed

import (
	"context"
	"testing"

	sqlc "forestburger/db/sqlc"
	"forestburger/internal/basetest"
)

func TestDatosPrueba_SinLaVariableNoHaceNada(t *testing.T) {
	db := basetest.Abrir(t)
	t.Setenv("DEV_DATOS_PRUEBA", "")

	res, err := DatosPrueba(context.Background(), db)
	if err != nil || res != (Resultado{}) {
		t.Fatalf("res = %+v, err = %v; quiero que no haga nada", res, err)
	}
	if n, _ := sqlc.New(db).CountProductos(context.Background()); n != 0 {
		t.Errorf("no tendría que haber cargado productos, hay %d", n)
	}
}

func TestDatosPrueba_CargaUnaSolaVez(t *testing.T) {
	db := basetest.Abrir(t)
	t.Setenv("DEV_DATOS_PRUEBA", "1")
	ctx := context.Background()

	res, err := DatosPrueba(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if res.Productos != len(productos) || res.Extras != len(extras) || res.Pedidos < 30 {
		t.Errorf("cargó %+v; quiero %d productos, %d extras y al menos 30 pedidos", res, len(productos), len(extras))
	}

	// Hay de todo para ver el panel: estados, pagos, entregas y pedidos recientes en curso.
	var estados, pagos, entregas, enCursoHoy int
	db.QueryRow(`SELECT count(DISTINCT estado) FROM pedido`).Scan(&estados)
	db.QueryRow(`SELECT count(DISTINCT pago) FROM pedido`).Scan(&pagos)
	db.QueryRow(`SELECT count(DISTINCT entrega) FROM pedido`).Scan(&entregas)
	db.QueryRow(`SELECT count(*) FROM pedido WHERE estado IN ('pendiente', 'confirmado') AND creado_en > now() - interval '1 day'`).Scan(&enCursoHoy)
	if estados < 3 || pagos != 2 || entregas != 2 || enCursoHoy == 0 {
		t.Errorf("datos poco variados: %d estados, %d pagos, %d entregas, %d en curso en las últimas 24 h", estados, pagos, entregas, enCursoHoy)
	}
	if aj, _ := sqlc.New(db).GetAjustes(ctx); aj.AliasCbu == "" || aj.CostoEnvio != "1500.00" {
		t.Errorf("ajustes de ejemplo no cargados: %+v", aj)
	}

	// Una segunda corrida (la base ya tiene productos) no duplica nada.
	res, err = DatosPrueba(ctx, db)
	if err != nil || res != (Resultado{}) {
		t.Errorf("la segunda corrida tendría que no hacer nada: %+v, %v", res, err)
	}
	if n, _ := sqlc.New(db).CountProductos(ctx); n != int64(len(productos)) {
		t.Errorf("hay %d productos, quiero %d (sin duplicar)", n, len(productos))
	}
}

// Las promos del Figma, con sus productos: una activa y una pausada.
func TestDatosPrueba_Promos(t *testing.T) {
	db := basetest.Abrir(t)
	t.Setenv("DEV_DATOS_PRUEBA", "1")
	ctx := context.Background()
	if _, err := DatosPrueba(ctx, db); err != nil {
		t.Fatal(err)
	}
	q := sqlc.New(db)
	lista, err := q.ListPromos(ctx)
	if err != nil || len(lista) != 2 || !lista[0].Activa || lista[1].Activa {
		t.Fatalf("promos = %+v, %v; quiero 2 (la primera activa, la segunda pausada)", lista, err)
	}
	for i, quiero := range []int{4, 2} {
		if ids, _ := q.ListProductosDePromo(ctx, lista[i].ID); len(ids) != quiero {
			t.Errorf("%s tiene %d productos, quiero %d", lista[i].Nombre, len(ids), quiero)
		}
	}
}
