package auth

import (
	"os"
	"strings"
)

// EmailPermitido indica si el email está en ADMIN_EMAILS (mails separados por coma). Se compara
// normalizado de los dos lados. Si la variable está vacía no hay nadie permitido.
func EmailPermitido(email string) bool {
	email = NormalizarEmail(email)
	if email == "" {
		return false
	}
	for _, permitido := range strings.Split(os.Getenv("ADMIN_EMAILS"), ",") {
		if NormalizarEmail(permitido) == email {
			return true
		}
	}
	return false
}
