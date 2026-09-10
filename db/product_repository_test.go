package db_test

import (
	"context"
	"database/sql"
	"testing"

	sqlc "forestburger/db/sqlc"

	_ "github.com/lib/pq"
)

// setupTestDB inicializa la conexión y devuelve la instancia de Queries y una función de limpieza.
func setupTestDB(t *testing.T) (*sqlc.Queries, func()) {
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

	return sqlc.New(conn), cleanup
}

// probarCrearProducto inserta un producto y valida que no falle.
func probarCrearProducto(t *testing.T, queries *sqlc.Queries, ctx context.Context) sqlc.Producto {
	t.Helper()

	prod, err := queries.CreateProducto(ctx, sqlc.CreateProductoParams{
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
func probarObtenerProducto(t *testing.T, queries *sqlc.Queries, ctx context.Context, id int32) sqlc.Producto {
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

// probarActualizarProducto modifica el producto y comprueba los cambios.
func probarActualizarProducto(t *testing.T, queries *sqlc.Queries, ctx context.Context, id int32) sqlc.Producto {
	t.Helper()

	actualizado, err := queries.UpdateProducto(ctx, sqlc.UpdateProductoParams{
		ID:          id,
		Nombre:      "Hamburguesa Doble Queso",
		Categoria:   "hamburguesa",
		Precio:      "9500.00",
		Descripcion: sql.NullString{String: "Doble cheddar y bacon", Valid: true},
	})
	if err != nil {
		t.Fatalf("Error al actualizar producto: %v", err)
	}

	if actualizado.Nombre != "Hamburguesa Doble Queso" {
		t.Errorf("Se esperaba 'Hamburguesa Doble Queso', se obtuvo '%s'", actualizado.Nombre)
	}

	return actualizado
}

// probarBorrarProducto elimina el producto por su ID.
func probarBorrarProducto(t *testing.T, queries *sqlc.Queries, ctx context.Context, id int32) {
	t.Helper()

	err := queries.DeleteProducto(ctx, id)
	if err != nil {
		t.Fatalf("Error al borrar producto: %v", err)
	}
}

// TestCRUDProducto orquesta el flujo completo de prueba invocando cada paso.
func TestCRUDProducto(t *testing.T) {
	queries, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	var prod sqlc.Producto

	t.Run("Crear Producto", func(t *testing.T) {
		prod = probarCrearProducto(t, queries, ctx)
	})

	t.Run("Obtener Producto", func(t *testing.T) {
		probarObtenerProducto(t, queries, ctx, prod.ID)
	})

	t.Run("Actualizar Producto", func(t *testing.T) {
		prod = probarActualizarProducto(t, queries, ctx, prod.ID)
	})

	t.Run("Borrar Producto", func(t *testing.T) {
		probarBorrarProducto(t, queries, ctx, prod.ID)
	})
}
