package main

import (
	"net/http"
	"net/http/httptest"
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
