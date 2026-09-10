package db_test

import (
	"context"
	"database/sql"
	"testing"
	"forestburger/db/sqlc"

	_ "github.com/lib/pq"
)

// setupTestDB inicializa la conexión y devuelve la instancia de Queries y una función de limpieza.
// Se puede reutilizar en cualquier test nuevo que necesite la BD.
func setupTestDB(t *testing.T) (*Queries, func()) {
	t.Helper()

	connStr := "postgres://usuario:password@localhost:5432/forestburger?sslmode=disable"
	conn, err := sql.Open("postgres", connStr)
	if err != nil {
		t.Fatalf("Error abriendo conexión: %v", err)
	}

	if err := conn.Ping(); err != nil {
		conn.Close()
		t.Fatalf("No se pudo conectar a la base de datos: %v", err)
	}

	cleanup := func() {
		conn.Close()
	}

	return New(conn), cleanup
}

// probarCrearProducto inserta un producto y valida que no falle.
func probarCrearProducto(t *testing.T, queries *Queries, ctx context.Context) Producto {
	t.Helper()

	prod, err := queries.CreateProducto(ctx, CreateProductoParams{
		Nombre:      "Hamburguesa Clásica",
		Categoria:   "hamburguesa",
		Precio:      "8500.00",
		Descripcion: sql.NullString{String: "Carne, queso y lechuga", Valid: true},
	})
	if err != nil {
		t.Fatalf("Error al crear producto: %v", err)
	}

	if prod.ID == 0 {
		t.Fatal("Se esperaba un ID autogenerado válido y se obtuvo 0")
	}

	return prod
}

// probarObtenerProducto consulta el producto por ID y valida sus campos.
func probarObtenerProducto(t *testing.T, queries *Queries, ctx context.Context, id int32) Producto {
	t.Helper()

	obtenido, err := queries.GetProducto(ctx, id)
	if err != nil {
		t.Fatalf("Error al obtener producto: %v", err)
	}

	if obtenido.Nombre != "Hamburguesa Clásica" {
		t.Errorf("Se esperaba 'Hamburguesa Clásica', se obtuvo '%s'", obtenido.Nombre)
	}

	return obtenido
}

// probarListarProductos verifica que el producto creado aparezca en el listado general.
func probarListarProductos(t *testing.T, queries *Queries, ctx context.Context, idEsperado int32) {
	t.Helper()

	lista, err := queries.ListProductos(ctx)
	if err != nil {
		t.Fatalf("Error al listar productos: %v", err)
	}

	if len(lista) == 0 {
		t.Fatal("Se esperaba al menos un producto en la lista")
	}

	encontrado := false
	for _, p := range lista {
		if p.ID == idEsperado {
			encontrado = true
			break
		}
	}
	if !encontrado {
		t.Errorf("Se esperaba encontrar el producto con ID %d en el listado", idEsperado)
	}
}

// probarActualizarProducto modifica el producto y valida que los cambios se hayan guardado.
func probarActualizarProducto(t *testing.T, queries *Queries, ctx context.Context, id int32) Producto {
	t.Helper()

	actualizado, err := queries.UpdateProducto(ctx, UpdateProductoParams{
		ID:          id,
		Nombre:      "Hamburguesa Clásica Pro",
		Categoria:   "hamburguesa",
		Precio:      "9500.00",
		Descripcion: sql.NullString{String: "Carne, queso, lechuga y bacon", Valid: true},
	})
	if err != nil {
		t.Fatalf("Error al actualizar producto: %v", err)
	}

	if actualizado.Nombre != "Hamburguesa Clásica Pro" {
		t.Errorf("Se esperaba 'Hamburguesa Clásica Pro', se obtuvo '%s'", actualizado.Nombre)
	}

	return actualizado
}

// probarBorrarProducto elimina el producto y verifica que la eliminación se ejecute correctamente.
func probarBorrarProducto(t *testing.T, queries *Queries, ctx context.Context, id int32) {
	t.Helper()

	err := queries.DeleteProducto(ctx, id)
	if err != nil {
		t.Fatalf("Error al borrar producto: %v", err)
	}
}

// TestCRUDProducto orquesta el flujo completo invocando las funciones paso a paso.
func TestCRUDProducto(t *testing.T) {
	queries, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	var prod Producto

	t.Run("Crear Producto", func(t *testing.T) {
		prod = probarCrearProducto(t, queries, ctx)
	})

	t.Run("Obtener Producto", func(t *testing.T) {
		probarObtenerProducto(t, queries, ctx, prod.ID)
	})

	t.Run("Listar Productos", func(t *testing.T) {
		probarListarProductos(t, queries, ctx, prod.ID)
	})

	t.Run("Actualizar Producto", func(t *testing.T) {
		prod = probarActualizarProducto(t, queries, ctx, prod.ID)
	})

	t.Run("Borrar Producto", func(t *testing.T) {
		probarBorrarProducto(t, queries, ctx, prod.ID)
	})
}
