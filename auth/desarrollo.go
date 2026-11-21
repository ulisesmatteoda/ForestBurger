package auth

import (
	"context"
	"errors"
	"fmt"
	"os"

	sqlc "forestburger/db/sqlc"
)

// LargoMinimoPassword es el mínimo de una contraseña de admin (lo usan crear-admin y el admin de
// desarrollo).
const LargoMinimoPassword = 8

// GuardadorAdmin es lo que necesita AdminDesarrollo de la base: en la app son las queries de sqlc,
// y en los tests un falso, así se prueba sin Postgres.
type GuardadorAdmin interface {
	GuardarAdmin(ctx context.Context, arg sqlc.GuardarAdminParams) (sqlc.GuardarAdminRow, error)
}

// AdminDesarrollo crea (o actualiza) el admin de DEV_ADMIN_EMAIL y DEV_ADMIN_PASSWORD al arrancar.
// Sirve en desarrollo, donde la base se borra en cada make run y si no habría que correr
// make crear-admin cada vez. Si falta alguna de las dos variables no hace nada y devuelve "": en
// producción no se ponen. Devuelve el email que guardó. Nunca devuelve ni loguea la contraseña.
func AdminDesarrollo(ctx context.Context, db GuardadorAdmin) (string, error) {
	email := NormalizarEmail(os.Getenv("DEV_ADMIN_EMAIL"))
	password := os.Getenv("DEV_ADMIN_PASSWORD")
	if email == "" || password == "" {
		return "", nil
	}
	if !EmailPermitido(email) {
		return "", fmt.Errorf("DEV_ADMIN_EMAIL (%s) no está en ADMIN_EMAILS", email)
	}
	if len(password) < LargoMinimoPassword {
		return "", fmt.Errorf("DEV_ADMIN_PASSWORD tiene que tener al menos %d caracteres", LargoMinimoPassword)
	}
	hash, err := HashearPassword(password)
	if err != nil {
		return "", errors.New("no se pudo hashear DEV_ADMIN_PASSWORD")
	}
	if _, err := db.GuardarAdmin(ctx, sqlc.GuardarAdminParams{Email: email, PasswordHash: hash}); err != nil {
		return "", fmt.Errorf("no se pudo guardar el admin de desarrollo: %w", err)
	}
	return email, nil
}
