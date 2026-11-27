// Package basetest abre la base de los tests y la deja vacía. La usan los tests que necesitan
// Postgres (handlers, pedidos, seed). Nunca apunta a la base de desarrollo: make test usa
// forestburger_test, y sin DATABASE_URL también.
package basetest

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

const baseDeTest = "postgres://usuario:password@localhost:5432/forestburger_test?sslmode=disable"

// Abrir conecta con la base de test, borra los datos de todas las tablas y deja ajustes y horario
// con sus valores de fábrica. La conexión se cierra sola al terminar el test.
func Abrir(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = baseDeTest
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("no se pudo abrir la base: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatalf("no se pudo conectar a la base: %v", err)
	}
	// ajustes y horario tienen siempre una fila (y siete): se vuelven a sus valores de fábrica.
	if _, err := db.Exec(`TRUNCATE pedido, producto, admin, extra, promo, push_suscripcion RESTART IDENTITY CASCADE;
		TRUNCATE ajustes, horario;
		INSERT INTO ajustes (id) VALUES (1);
		INSERT INTO horario (dia) SELECT generate_series(0, 6)`); err != nil {
		t.Fatalf("no se pudo limpiar la base: %v", err)
	}
	return db
}
