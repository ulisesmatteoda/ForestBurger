// Package auth tiene lo que comparten el comando crear-admin y (más adelante) el login:
// el hash de contraseñas, la forma de escribir un email y la lista de emails permitidos.
package auth

import (
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// HashearPassword devuelve el hash bcrypt de la contraseña; es lo único que se guarda en la base.
func HashearPassword(p string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
	return string(hash), err
}

// PasswordCorrecta compara la contraseña con el hash guardado.
func PasswordCorrecta(hash, p string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(p)) == nil
}

// NormalizarEmail lo deja de una sola forma ("  Ana@Mail.com " → "ana@mail.com"), así el mismo
// email escrito distinto no termina siendo dos admins.
func NormalizarEmail(e string) string {
	return strings.ToLower(strings.TrimSpace(e))
}
