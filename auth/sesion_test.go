package auth

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const secretoTest = "0123456789abcdef0123456789abcdef"

func configurar(t *testing.T) {
	t.Helper()
	t.Setenv("SESSION_SECRET", secretoTest)
	t.Setenv("COOKIE_SECURE", "")
	if err := CargarConfig(); err != nil {
		t.Fatal(err)
	}
}

// pedidoCon arma un request que trae la cookie con ese valor.
func pedidoCon(valor string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/admin", nil)
	r.AddCookie(&http.Cookie{Name: NombreCookie, Value: valor})
	return r
}

func TestCargarConfig_SinSecretoOCorto(t *testing.T) {
	for _, s := range []string{"", "corto"} {
		t.Setenv("SESSION_SECRET", s)
		if err := CargarConfig(); err == nil {
			t.Errorf("SESSION_SECRET=%q tendría que dar error", s)
		}
	}
}

func TestCookieValidaSeLee(t *testing.T) {
	configurar(t)
	ahora := time.Now()
	c := CrearCookie("ana@forestburger.com", ahora)

	email, ok := LeerSesion(pedidoCon(c.Value), ahora.Add(time.Hour))
	if !ok || email != "ana@forestburger.com" {
		t.Fatalf("LeerSesion = %q, %v; quiero el mail y true", email, ok)
	}
	if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" {
		t.Errorf("atributos de la cookie: HttpOnly=%v SameSite=%v Path=%q", c.HttpOnly, c.SameSite, c.Path)
	}
	if c.Secure {
		t.Error("sin COOKIE_SECURE=true no tendría que ser Secure")
	}
}

func TestCookieSecureEnProduccion(t *testing.T) {
	t.Setenv("SESSION_SECRET", secretoTest)
	t.Setenv("COOKIE_SECURE", "true")
	if err := CargarConfig(); err != nil {
		t.Fatal(err)
	}
	if !CrearCookie("ana@forestburger.com", time.Now()).Secure {
		t.Error("con COOKIE_SECURE=true tendría que ser Secure")
	}
}

func TestCookieAlteradaOVencidaSeRechaza(t *testing.T) {
	configurar(t)
	ahora := time.Now()
	valor := CrearCookie("ana@forestburger.com", ahora).Value
	datos, firma, _ := strings.Cut(valor, ".")

	// Otro mail con la firma del original: la firma ya no coincide.
	otroMail := codificar([]byte("intruso@gmail.com|" + strings.Split(string(mustDecode(t, datos)), "|")[1]))

	casos := map[string]struct {
		valor string
		ahora time.Time
	}{
		"firma alterada":       {datos + "." + firma[:len(firma)-2] + "AA", ahora},
		"datos cambiados":      {otroMail + "." + firma, ahora},
		"sin punto":            {datos + firma, ahora},
		"firma no es base64":   {datos + ".%%%", ahora},
		"vacía":                {"", ahora},
		"vencida":              {valor, ahora.Add(duracionSesion + time.Second)},
		"justo al vencimiento": {valor, ahora.Add(duracionSesion)},
	}
	for nombre, c := range casos {
		if email, ok := LeerSesion(pedidoCon(c.valor), c.ahora); ok {
			t.Errorf("%s: se aceptó la sesión de %q", nombre, email)
		}
	}
}

func TestOtroSecretoInvalidaLasSesiones(t *testing.T) {
	configurar(t)
	ahora := time.Now()
	valor := CrearCookie("ana@forestburger.com", ahora).Value

	t.Setenv("SESSION_SECRET", strings.Repeat("z", 32))
	if err := CargarConfig(); err != nil {
		t.Fatal(err)
	}
	if _, ok := LeerSesion(pedidoCon(valor), ahora); ok {
		t.Error("cambiar SESSION_SECRET tendría que cerrar las sesiones existentes")
	}
}

func TestBorrarCookie(t *testing.T) {
	c := BorrarCookie()
	if c.Name != NombreCookie || c.MaxAge != -1 || c.Path != "/" {
		t.Errorf("BorrarCookie = %+v", c)
	}
}

func mustDecode(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
