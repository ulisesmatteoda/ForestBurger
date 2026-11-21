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

// sesionAdmin revisa la cookie de sesión y devuelve el email si es de un admin que puede entrar:
// firma y vencimiento bien, el mail sigue en ADMIN_EMAILS y el admin sigue existiendo en la base
// (en desarrollo la base se borra en cada make run, y una cookie vieja seguía abriendo el panel).
// borrar avisa que la cookie es válida pero su admin ya no existe: hay que sacarla del navegador.
func (a *API) sesionAdmin(r *http.Request) (email string, borrar bool, err error) {
	email, ok := auth.LeerSesion(r, time.Now())
	if !ok || !auth.EmailPermitido(email) {
		return "", false, nil
	}
	if _, err := a.q.AdminPorEmail(r.Context(), email); errors.Is(err, sql.ErrNoRows) {
		return "", true, nil
	} else if err != nil {
		return "", false, err
	}
	return email, false, nil
}

// RequiereAdmin deja pasar solo a un admin con sesión válida (ver sesionAdmin): sacar a alguien de
// ADMIN_EMAILS o borrarlo de la base le corta el acceso aunque su cookie no haya vencido. Sin
// sesión, a htmx le pide que vaya al login (HX-Redirect, con 200 para que htmx 1.9 lo procese) y a
// un pedido común lo redirige al login recordando a dónde quería ir.
func (a *API) RequiereAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		email, borrar, err := a.sesionAdmin(r)
		if err != nil {
			http.Error(w, "No se pudo verificar la sesión", http.StatusInternalServerError)
			return
		}
		if email == "" {
			if borrar {
				http.SetCookie(w, auth.BorrarCookie())
			}
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
// Con una sesión válida no tiene sentido volver a entrar: va directo al panel. Si la cookie era de
// un admin que ya no existe, la borra y muestra el login (si no, el login y el panel se mandarían
// el uno al otro).
func (a *API) FormLogin(w http.ResponseWriter, r *http.Request) {
	email, borrar, err := a.sesionAdmin(r)
	if err != nil {
		http.Error(w, "No se pudo verificar la sesión", http.StatusInternalServerError)
		return
	}
	if email != "" {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	if borrar {
		http.SetCookie(w, auth.BorrarCookie())
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
