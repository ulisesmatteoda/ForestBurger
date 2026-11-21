package db_test

import (
	"context"
	"testing"

	sqlc "forestburger/db/sqlc"
)

// Guardar dos veces el mismo email cambia la contraseña (el hash) en vez de crear otro admin:
// así make crear-admin sirve también para "me olvidé la contraseña".
func TestGuardarAdmin_MismoEmailActualizaElHash(t *testing.T) {
	queries, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	const email = "repo-test@forestburger.com"

	primero, err := queries.GuardarAdmin(ctx, sqlc.GuardarAdminParams{Email: email, PasswordHash: "hash-viejo"})
	if err != nil {
		t.Fatalf("primer guardado: %v", err)
	}
	segundo, err := queries.GuardarAdmin(ctx, sqlc.GuardarAdminParams{Email: email, PasswordHash: "hash-nuevo"})
	if err != nil {
		t.Fatalf("segundo guardado: %v", err)
	}
	if segundo.ID != primero.ID {
		t.Errorf("se creó otro admin (id %d y %d), quiero que se actualice el mismo", primero.ID, segundo.ID)
	}

	admin, err := queries.AdminPorEmail(ctx, email)
	if err != nil {
		t.Fatalf("AdminPorEmail: %v", err)
	}
	if admin.PasswordHash != "hash-nuevo" {
		t.Errorf("PasswordHash = %q, quiero el nuevo", admin.PasswordHash)
	}
}
