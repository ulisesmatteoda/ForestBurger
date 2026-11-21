package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"forestburger/auth"
	sqlc "forestburger/db/sqlc"
)

const claveTest = "clave-correcta-123"

// hashTest es el hash bcrypt de claveTest. Se calcula una sola vez: bcrypt es lento a propósito y
// casi todos los tests guardan el admin.
var hashTest = sync.OnceValue(func() string {
	hash, err := auth.HashearPassword(claveTest)
	if err != nil {
		panic(err)
	}
	return hash
})

// guardarAdminTest deja en la base el admin de los tests, con la contraseña claveTest.
func guardarAdminTest(t *testing.T, q *sqlc.Queries) {
	t.Helper()
	if _, err := q.GuardarAdmin(context.Background(), sqlc.GuardarAdminParams{Email: adminTest, PasswordHash: hashTest()}); err != nil {
		t.Fatal(err)
	}
}

// conAdminGuardado arma el mux sin sesión y guarda el admin de los tests en la base.
func conAdminGuardado(t *testing.T) http.Handler {
	t.Helper()
	mux, q := setupSinSesion(t)
	guardarAdminTest(t, q)
	return mux
}

func login(mux http.Handler, datos url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(datos.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// cookieSesion devuelve la cookie fb_sesion que dejó la respuesta, o nil.
func cookieSesion(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.NombreCookie {
			return c
		}
	}
	return nil
}

// ---------- middleware ----------

func TestAdmin_SinSesionRedirigeAlLogin(t *testing.T) {
	mux, _ := setupSinSesion(t)

	rec := pedirPanel(mux, "/admin", false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, quiero 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login?next=%2Fadmin" {
		t.Errorf("Location = %q, quiero /login?next=%%2Fadmin", loc)
	}

	// Las rutas de abajo también están protegidas, y next conserva la query.
	rec = pedirPanel(mux, "/admin?categoria=bebida", false)
	if loc := rec.Header().Get("Location"); loc != "/login?next="+url.QueryEscape("/admin?categoria=bebida") {
		t.Errorf("Location = %q", loc)
	}
	rec = pedir(mux, http.MethodDelete, "/admin/productos/1", nil)
	if rec.Code != http.StatusSeeOther {
		t.Errorf("DELETE sin sesión: status = %d, quiero 303", rec.Code)
	}
}

func TestAdmin_SinSesionConHTMXPideIrAlLogin(t *testing.T) {
	mux, _ := setupSinSesion(t)

	rec := pedirPanel(mux, "/admin?categoria=bebida", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quiero 200 (htmx 1.9 no procesa HX-Redirect en un 3xx)", rec.Code)
	}
	if got := rec.Header().Get("HX-Redirect"); got != "/login" {
		t.Errorf("HX-Redirect = %q, quiero /login", got)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("no tendría que mandar el panel: %q", rec.Body.String())
	}
}

func TestAdmin_ConSesionMuestraElMailYSalir(t *testing.T) {
	mux, _ := setup(t)

	rec := pedirPanel(mux, "/admin", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quiero 200", rec.Code)
	}
	body := rec.Body.String()
	for _, esperado := range []string{adminTest, `action="/logout"`, "Salir"} {
		if !strings.Contains(body, esperado) {
			t.Errorf("el panel no contiene %q", esperado)
		}
	}
}

func TestAdmin_MailSacadoDeLaListaSeRechaza(t *testing.T) {
	mux, _ := setup(t) // la cookie es válida y está firmada
	t.Setenv("ADMIN_EMAILS", "otro@forestburger.com")

	rec := pedirPanel(mux, "/admin", false)
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/login") {
		t.Errorf("status = %d, Location = %q; quiero 303 al login", rec.Code, rec.Header().Get("Location"))
	}
}

// ---------- login ----------

func TestLogin_FormularioPublico(t *testing.T) {
	mux, _ := setupSinSesion(t)

	rec := pedirPanel(mux, "/login?next=%2Fadmin%3Fcategoria%3Dbebida", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quiero 200", rec.Code)
	}
	body := rec.Body.String()
	for _, esperado := range []string{`action="/login"`, `type="email"`, `autocomplete="current-password"`, `name="next" value="/admin?categoria=bebida"`, "ENTRAR"} {
		if !strings.Contains(body, esperado) {
			t.Errorf("el login no contiene %q", esperado)
		}
	}
	if strings.Contains(body, `class="sidebar"`) {
		t.Error("el login no lleva la barra lateral del panel")
	}
}

func TestLogin_ConSesionVaAlPanel(t *testing.T) {
	mux, _ := setup(t)

	rec := pedirPanel(mux, "/login", false)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/admin" {
		t.Errorf("status = %d, Location = %q; quiero 303 a /admin", rec.Code, rec.Header().Get("Location"))
	}
}

func TestLogin_DatosMalosDanElMismoError(t *testing.T) {
	casos := map[string]url.Values{
		"contraseña mala":        {"email": {adminTest}, "password": {"otra-clave-999"}},
		"mail que no existe":     {"email": {"nuevo@forestburger.com"}, "password": {claveTest}},
		"mail fuera de la lista": {"email": {"intruso@gmail.com"}, "password": {claveTest}},
		"mail vacío":             {"email": {""}, "password": {claveTest}},
	}
	for nombre, datos := range casos {
		t.Run(nombre, func(t *testing.T) {
			mux := conAdminGuardado(t)
			if nombre == "mail que no existe" {
				// Está permitido pero nunca se creó con make crear-admin.
				t.Setenv("ADMIN_EMAILS", adminTest+",nuevo@forestburger.com")
			}

			rec := login(mux, datos)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, quiero 401", rec.Code)
			}
			body := rec.Body.String()
			if !strings.Contains(body, "Mail o contraseña incorrectos.") {
				t.Error("falta el mensaje genérico")
			}
			if c := cookieSesion(rec); c != nil {
				t.Errorf("no tendría que dejar cookie, dejó %q", c.Value)
			}
			if pw := datos.Get("password"); strings.Contains(body, pw) {
				t.Error("la página nunca tiene que devolver la contraseña escrita")
			}
			if email := datos.Get("email"); email != "" && !strings.Contains(body, `value="`+email+`"`) {
				t.Error("tendría que conservar el mail escrito")
			}
		})
	}
}

func TestLogin_BienDejaCookieYVaAlPanel(t *testing.T) {
	casos := []struct{ next, destino string }{
		{"", "/admin"},
		{"/admin?categoria=bebida", "/admin?categoria=bebida"},
		{"https://otro.com", "/admin"},
		{"//otro.com/admin", "/admin"},
		{"/login", "/admin"},
	}
	for _, c := range casos {
		t.Run("next="+c.next, func(t *testing.T) {
			mux := conAdminGuardado(t)

			rec := login(mux, url.Values{"email": {"  Admin@ForestBurger.com "}, "password": {claveTest}, "next": {c.next}})
			if rec.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, quiero 303", rec.Code)
			}
			if loc := rec.Header().Get("Location"); loc != c.destino {
				t.Errorf("Location = %q, quiero %q", loc, c.destino)
			}
			cookie := cookieSesion(rec)
			if cookie == nil {
				t.Fatal("no dejó la cookie fb_sesion")
			}
			if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
				t.Errorf("cookie sin HttpOnly/SameSite=Lax: %+v", cookie)
			}

			// Con esa cookie, el panel abre.
			req := httptest.NewRequest(http.MethodGet, "/admin", nil)
			req.AddCookie(cookie)
			panel := httptest.NewRecorder()
			mux.ServeHTTP(panel, req)
			if panel.Code != http.StatusOK {
				t.Errorf("con la cookie del login, /admin dio %d", panel.Code)
			}
		})
	}
}

// ---------- logout ----------

func TestLogout_BorraLaCookie(t *testing.T) {
	mux, _ := setup(t)

	rec := pedir(mux, http.MethodPost, "/logout", nil)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login" {
		t.Errorf("status = %d, Location = %q; quiero 303 a /login", rec.Code, rec.Header().Get("Location"))
	}
	c := cookieSesion(rec)
	if c == nil || c.MaxAge >= 0 {
		t.Errorf("tendría que borrar la cookie (MaxAge < 0), vino %+v", c)
	}
}

// ---------- admin que ya no existe ----------

func TestAdmin_CookieDeAdminInexistenteSeBorra(t *testing.T) {
	mux, _ := setupSinSesion(t) // no guarda ningún admin
	conCookie := conSesion(mux, adminTest)

	rec := pedirPanel(conCookie, "/admin", false)
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/login") {
		t.Errorf("status = %d, Location = %q; quiero 303 al login", rec.Code, rec.Header().Get("Location"))
	}
	if c := cookieSesion(rec); c == nil || c.MaxAge >= 0 {
		t.Errorf("tendría que borrar la cookie del admin inexistente, vino %+v", c)
	}

	rec = pedirPanel(conCookie, "/admin", true)
	if got := rec.Header().Get("HX-Redirect"); got != "/login" {
		t.Errorf("con htmx: HX-Redirect = %q, quiero /login", got)
	}
	if c := cookieSesion(rec); c == nil || c.MaxAge >= 0 {
		t.Error("con htmx también tendría que borrar la cookie")
	}
}

func TestLogin_CookieDeAdminInexistenteMuestraElLogin(t *testing.T) {
	mux, _ := setupSinSesion(t)

	rec := pedirPanel(conSesion(mux, adminTest), "/login", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quiero 200 con el login (no redirigir a /admin)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `action="/login"`) {
		t.Error("tendría que mostrar el formulario de login")
	}
	if c := cookieSesion(rec); c == nil || c.MaxAge >= 0 {
		t.Error("tendría que borrar la cookie del admin inexistente")
	}
}
