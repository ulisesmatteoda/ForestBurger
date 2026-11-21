package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"forestburger/auth"
	"forestburger/views"
)

// Un solo mensaje para mail inexistente, no permitido o contraseña mala: así no se puede averiguar
// qué mails son de admins probando el login.
const errorLogin = "Mail o contraseña incorrectos."

// RequiereAdmin deja pasar solo con una sesión válida de un mail que sigue en ADMIN_EMAILS (sacar
// a alguien de la lista le corta el acceso aunque su cookie no haya vencido). Sin sesión, a htmx
// le pide que vaya al login (HX-Redirect, con 200 para que htmx 1.9 lo procese) y a un pedido
// común lo redirige al login recordando a dónde quería ir.
func RequiereAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		email, ok := auth.LeerSesion(r, time.Now())
		if !ok || !auth.EmailPermitido(email) {
			if esHTMX(r) {
				w.Header().Set("HX-Redirect", "/login")
				w.WriteHeader(http.StatusOK)
				return
			}
			http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.ConAdmin(r.Context(), email)))
	})
}

// FormLogin: GET /login
// Con una sesión válida no tiene sentido volver a entrar: va directo al panel.
func (a *API) FormLogin(w http.ResponseWriter, r *http.Request) {
	if email, ok := auth.LeerSesion(r, time.Now()); ok && auth.EmailPermitido(email) {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	renderLogin(w, r, views.FormLogin{Next: r.URL.Query().Get("next")}, http.StatusOK)
}

// Login: POST /login
// Form común, sin htmx. Si los datos están bien deja la cookie de sesión y vuelve a donde quería
// ir (next), solo si es una página del panel; si no, vuelve a mostrar el login con el error.
func (a *API) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido", http.StatusBadRequest)
		return
	}
	email := auth.NormalizarEmail(r.PostFormValue("email"))
	password := r.PostFormValue("password")
	next := r.PostFormValue("next")
	// El form vuelve con el mail escrito, nunca con la contraseña.
	fallo := views.FormLogin{Email: email, Next: next, Error: errorLogin}

	if !auth.EmailPermitido(email) {
		renderLogin(w, r, fallo, http.StatusUnauthorized)
		return
	}
	admin, err := a.q.AdminPorEmail(r.Context(), email)
	if errors.Is(err, sql.ErrNoRows) {
		renderLogin(w, r, fallo, http.StatusUnauthorized)
		return
	}
	if err != nil {
		http.Error(w, "No se pudo iniciar sesión", http.StatusInternalServerError)
		return
	}
	if !auth.PasswordCorrecta(admin.PasswordHash, password) {
		renderLogin(w, r, fallo, http.StatusUnauthorized)
		return
	}

	http.SetCookie(w, auth.CrearCookie(admin.Email, time.Now()))
	http.Redirect(w, r, destinoSeguro(next), http.StatusSeeOther)
}

// Logout: POST /logout
func (a *API) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, auth.BorrarCookie())
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// destinoSeguro devuelve next solo si es una página del panel. Cualquier otra cosa (otro sitio,
// "//otro.com", una ruta pública) va a /admin: así el login no sirve para mandar a alguien afuera.
func destinoSeguro(next string) string {
	if next == "/admin" || strings.HasPrefix(next, "/admin/") || strings.HasPrefix(next, "/admin?") {
		return next
	}
	return "/admin"
}

func renderLogin(w http.ResponseWriter, r *http.Request, f views.FormLogin, status int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	views.LoginPage(f).Render(r.Context(), w)
}
