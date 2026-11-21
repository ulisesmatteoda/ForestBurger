package auth

import (
	"context"
	"testing"

	sqlc "forestburger/db/sqlc"
)

// guardadorFalso anota lo que le piden guardar, sin base.
type guardadorFalso struct{ guardados []sqlc.GuardarAdminParams }

func (g *guardadorFalso) GuardarAdmin(_ context.Context, arg sqlc.GuardarAdminParams) (sqlc.GuardarAdminRow, error) {
	g.guardados = append(g.guardados, arg)
	return sqlc.GuardarAdminRow{ID: 1, Email: arg.Email}, nil
}

func TestAdminDesarrollo_SoloConLasDosVariables(t *testing.T) {
	t.Setenv("ADMIN_EMAILS", "dev@forestburger.com")
	casos := map[string][2]string{
		"ninguna":       {"", ""},
		"solo el mail":  {"dev@forestburger.com", ""},
		"solo la clave": {"", "clave-de-desarrollo"},
	}
	for nombre, v := range casos {
		t.Setenv("DEV_ADMIN_EMAIL", v[0])
		t.Setenv("DEV_ADMIN_PASSWORD", v[1])
		g := &guardadorFalso{}
		email, err := AdminDesarrollo(context.Background(), g)
		if err != nil || email != "" || len(g.guardados) != 0 {
			t.Errorf("%s: email=%q err=%v guardados=%d; quiero que no haga nada", nombre, email, err, len(g.guardados))
		}
	}
}

func TestAdminDesarrollo_GuardaConHash(t *testing.T) {
	t.Setenv("ADMIN_EMAILS", "dev@forestburger.com")
	t.Setenv("DEV_ADMIN_EMAIL", "  Dev@ForestBurger.com ")
	t.Setenv("DEV_ADMIN_PASSWORD", "clave-de-desarrollo")
	g := &guardadorFalso{}

	email, err := AdminDesarrollo(context.Background(), g)
	if err != nil {
		t.Fatal(err)
	}
	if email != "dev@forestburger.com" || len(g.guardados) != 1 {
		t.Fatalf("email=%q guardados=%d; quiero el mail normalizado y un guardado", email, len(g.guardados))
	}
	guardado := g.guardados[0]
	if guardado.PasswordHash == "clave-de-desarrollo" || !PasswordCorrecta(guardado.PasswordHash, "clave-de-desarrollo") {
		t.Error("tiene que guardar el hash bcrypt de la contraseña, nunca la contraseña")
	}
}

func TestAdminDesarrollo_ErroresNoGuardan(t *testing.T) {
	t.Setenv("ADMIN_EMAILS", "dev@forestburger.com")
	casos := map[string][2]string{
		"mail fuera de ADMIN_EMAILS": {"otro@forestburger.com", "clave-de-desarrollo"},
		"clave corta":                {"dev@forestburger.com", "corta"},
	}
	for nombre, v := range casos {
		t.Setenv("DEV_ADMIN_EMAIL", v[0])
		t.Setenv("DEV_ADMIN_PASSWORD", v[1])
		g := &guardadorFalso{}
		if _, err := AdminDesarrollo(context.Background(), g); err == nil || len(g.guardados) != 0 {
			t.Errorf("%s: err=%v guardados=%d; quiero error y nada guardado", nombre, err, len(g.guardados))
		}
	}
}
