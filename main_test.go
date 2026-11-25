package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNoIndexar(t *testing.T) {
	h := noIndexar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/styles.css", nil))
	if rec.Header().Get("X-Robots-Tag") != "noindex" {
		t.Error("en staging toda respuesta lleva X-Robots-Tag: noindex")
	}
}

func TestURLPublica(t *testing.T) {
	t.Setenv("RAILWAY_PUBLIC_DOMAIN", "")
	if got := urlPublica("8091"); got != "http://localhost:8091" {
		t.Errorf("en local: %q", got)
	}
	t.Setenv("RAILWAY_PUBLIC_DOMAIN", "forestburger-staging.up.railway.app")
	if got := urlPublica("8080"); got != "https://forestburger-staging.up.railway.app" {
		t.Errorf("en Railway: %q", got)
	}
}

// El schema embebido es el de db/schema.sql.
func TestSchemaEmbebido(t *testing.T) {
	if len(schema) < 100 {
		t.Error("db/schema.sql no quedó dentro del binario")
	}
}

func TestMedirPedidos_ServerTiming(t *testing.T) {
	h := medirPedidos(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/pedidos/aviso", nil))
	if rec.Code != http.StatusNoContent || !strings.HasPrefix(rec.Header().Get("Server-Timing"), "app;dur=") {
		t.Errorf("status %d, Server-Timing %q", rec.Code, rec.Header().Get("Server-Timing"))
	}
	// También cuando el handler escribe sin WriteHeader.
	h = medirPedidos(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("hola")) }))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Header().Get("Server-Timing") == "" || rec.Body.String() != "hola" {
		t.Error("Server-Timing tendría que ir también sin WriteHeader explícito")
	}
}

func TestCacheEstaticos(t *testing.T) {
	h := cacheEstaticos(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	for ruta, quiero := range map[string]string{
		"/static/styles.css?v=abc123":  "public, max-age=31536000, immutable",
		"/static/img/pinos.svg":        "no-cache",
		"/static/manifest.json?otro=1": "no-cache",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, ruta, nil))
		if got := rec.Header().Get("Cache-Control"); got != quiero {
			t.Errorf("%s: Cache-Control %q, quiero %q", ruta, got, quiero)
		}
	}
}

// En staging van los dos: la medición y el noindex (uno no puede pisar al otro).
func TestArmarHandler(t *testing.T) {
	mux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	for _, staging := range []bool{true, false} {
		rec := httptest.NewRecorder()
		armarHandler(mux, staging).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/pedidos/aviso", nil))
		if rec.Header().Get("Server-Timing") == "" {
			t.Errorf("staging=%v: falta Server-Timing", staging)
		}
		if (rec.Header().Get("X-Robots-Tag") == "noindex") != staging {
			t.Errorf("staging=%v: X-Robots-Tag = %q", staging, rec.Header().Get("X-Robots-Tag"))
		}
	}
}
