package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// La sesión es una cookie firmada, sin tabla de sesiones: el valor dice de quién es y hasta cuándo
// vale, y la firma HMAC prueba que la armó el servidor. Quien no tenga SESSION_SECRET no puede
// fabricar ni modificar una. Formato: base64url(email|vencimiento) + "." + base64url(firma).

const (
	NombreCookie    = "fb_sesion"
	duracionSesion  = 7 * 24 * time.Hour
	largoMinSecreto = 32
	separadorDatos  = "|"
	separadorFirma  = "."
)

var (
	secreto      []byte
	cookieSegura bool
)

// CargarConfig lee SESSION_SECRET (obligatorio, de al menos 32 caracteres) y COOKIE_SECURE
// ("true" en producción con HTTPS). Se llama una vez al arrancar: sin secreto la app no arranca.
func CargarConfig() error {
	s := os.Getenv("SESSION_SECRET")
	if len(s) < largoMinSecreto {
		return errors.New("falta SESSION_SECRET en el .env, o tiene menos de 32 caracteres. " +
			"Generá uno con: openssl rand -hex 32")
	}
	secreto = []byte(s)
	cookieSegura = os.Getenv("COOKIE_SECURE") == "true"
	return nil
}

func codificar(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func firmar(datos string) []byte {
	mac := hmac.New(sha256.New, secreto)
	mac.Write([]byte(datos))
	return mac.Sum(nil)
}

// CrearCookie arma la cookie de sesión del email, que vence en 7 días.
func CrearCookie(email string, ahora time.Time) *http.Cookie {
	vence := ahora.Add(duracionSesion)
	datos := codificar([]byte(email + separadorDatos + strconv.FormatInt(vence.Unix(), 10)))
	return &http.Cookie{
		Name:     NombreCookie,
		Value:    datos + separadorFirma + codificar(firmar(datos)),
		Path:     "/",
		Expires:  vence,
		MaxAge:   int(duracionSesion.Seconds()),
		HttpOnly: true, // el JS de la página no la puede leer
		SameSite: http.SameSiteLaxMode,
		Secure:   cookieSegura,
	}
}

// LeerSesion devuelve el email de la sesión si la cookie existe, la firma es del servidor, el
// formato es válido y no venció.
func LeerSesion(r *http.Request, ahora time.Time) (email string, ok bool) {
	if len(secreto) == 0 {
		return "", false
	}
	c, err := r.Cookie(NombreCookie)
	if err != nil {
		return "", false
	}
	datos, firma, ok := strings.Cut(c.Value, separadorFirma)
	if !ok {
		return "", false
	}
	firmaRecibida, err := base64.RawURLEncoding.DecodeString(firma)
	// hmac.Equal compara en tiempo constante: con == se podría adivinar la firma byte a byte.
	if err != nil || !hmac.Equal(firmaRecibida, firmar(datos)) {
		return "", false
	}
	plano, err := base64.RawURLEncoding.DecodeString(datos)
	if err != nil {
		return "", false
	}
	email, venceTxt, ok := strings.Cut(string(plano), separadorDatos)
	if !ok || email == "" {
		return "", false
	}
	vence, err := strconv.ParseInt(venceTxt, 10, 64)
	if err != nil || !ahora.Before(time.Unix(vence, 0)) {
		return "", false
	}
	return email, true
}

// BorrarCookie es la misma cookie vencida: el navegador la descarta.
func BorrarCookie() *http.Cookie {
	return &http.Cookie{
		Name:     NombreCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   cookieSegura,
	}
}

type claveAdmin struct{}

// ConAdmin guarda en el contexto el email del admin logueado (lo hace el middleware).
func ConAdmin(ctx context.Context, email string) context.Context {
	return context.WithValue(ctx, claveAdmin{}, email)
}

// AdminDesdeContexto devuelve el email del admin logueado, o "" si no hay.
func AdminDesdeContexto(ctx context.Context) string {
	email, _ := ctx.Value(claveAdmin{}).(string)
	return email
}
