package auth

import "testing"

func TestHashearYComparar(t *testing.T) {
	hash, err := HashearPassword("hamburguesa123")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "hamburguesa123" {
		t.Error("el hash no puede ser la contraseña")
	}
	if !PasswordCorrecta(hash, "hamburguesa123") {
		t.Error("la contraseña correcta tendría que dar true")
	}
	if PasswordCorrecta(hash, "hamburguesa124") {
		t.Error("una contraseña incorrecta tendría que dar false")
	}
}

func TestNormalizarEmail(t *testing.T) {
	if got := NormalizarEmail("  Ana.Perez@ForestBurger.COM \n"); got != "ana.perez@forestburger.com" {
		t.Errorf("NormalizarEmail = %q", got)
	}
}

func TestEmailPermitido(t *testing.T) {
	t.Setenv("ADMIN_EMAILS", " Ana@ForestBurger.com, luis@forestburger.com ,")

	casos := map[string]bool{
		"ana@forestburger.com":     true,
		"  LUIS@forestburger.com ": true,
		"otro@forestburger.com":    false,
		"":                         false, // la coma final deja un elemento vacío: no tiene que valer
	}
	for email, quiero := range casos {
		if got := EmailPermitido(email); got != quiero {
			t.Errorf("EmailPermitido(%q) = %v, quiero %v", email, got, quiero)
		}
	}
}

func TestEmailPermitido_ListaVacia(t *testing.T) {
	t.Setenv("ADMIN_EMAILS", "")
	if EmailPermitido("ana@forestburger.com") {
		t.Error("con ADMIN_EMAILS vacío no tendría que haber nadie permitido")
	}
}
