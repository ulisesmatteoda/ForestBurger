package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"testing"

	sqlc "forestburger/db/sqlc"
)

// sinVariablesR2 deja las 5 variables vacías durante el test (por si la máquina las tiene).
func sinVariablesR2(t *testing.T) {
	for _, v := range variablesR2 {
		t.Setenv(v, "")
	}
}

func TestConfigurarFotos_SinVariablesUsaDisco(t *testing.T) {
	sinVariablesR2(t)
	a := &API{dirFotos: "uploads"}
	if err := a.ConfigurarFotos(); err != nil || a.r2 != nil {
		t.Errorf("sin variables tendría que quedar en disco: r2 = %v, err = %v", a.r2, err)
	}
}

func TestConfigurarFotos_ConTodasUsaR2(t *testing.T) {
	sinVariablesR2(t)
	t.Setenv("S3_ENDPOINT", "https://cuenta.r2.cloudflarestorage.com")
	t.Setenv("S3_BUCKET", "fotos")
	t.Setenv("S3_ACCESS_KEY_ID", "clave")
	t.Setenv("S3_SECRET_ACCESS_KEY", "secreto")
	t.Setenv("FOTOS_URL_PUBLICA", "https://pub-x.r2.dev/")
	a := &API{dirFotos: "uploads"}
	if err := a.ConfigurarFotos(); err != nil || a.r2 == nil {
		t.Fatalf("con todas tendría que usar R2: err = %v", err)
	}
	if a.r2.bucket != "fotos" || a.r2.urlPublica != "https://pub-x.r2.dev" || a.r2.cliente.EndpointURL().Host != "cuenta.r2.cloudflarestorage.com" {
		t.Errorf("configuración: bucket %q, url %q, host %q", a.r2.bucket, a.r2.urlPublica, a.r2.cliente.EndpointURL().Host)
	}
	// Solo se borran las fotos de este bucket: las viejas del disco o de otro lado, no.
	for url, quiero := range map[string]bool{
		"https://pub-x.r2.dev/abc.png":   true,
		"https://pub-x.r2.dev.mal.com/a": false,
		"/uploads/abc.png":               false,
		"https://otro.r2.dev/abc.png":    false,
	} {
		if a.r2.esDeAca(url) != quiero {
			t.Errorf("esDeAca(%q) = %v", url, !quiero)
		}
	}
}

func TestConfigurarFotos_AMediasNoArranca(t *testing.T) {
	sinVariablesR2(t)
	t.Setenv("S3_BUCKET", "fotos")
	t.Setenv("S3_ENDPOINT", "https://cuenta.r2.cloudflarestorage.com")
	err := (&API{}).ConfigurarFotos()
	if err == nil || !strings.Contains(err.Error(), "faltan S3_ACCESS_KEY_ID, S3_SECRET_ACCESS_KEY, FOTOS_URL_PUBLICA") {
		t.Errorf("a medias tendría que decir qué falta: %v", err)
	}
}

// Con R2, la base guarda la URL completa: las vistas la usan tal cual (panel y web del cliente).
func TestFotos_URLCompletaSeVeTalCual(t *testing.T) {
	tt := setupCliente(t)
	foto := "https://pub-x.r2.dev/abc123.png"
	p, err := tt.q.CreateProducto(context.Background(), sqlc.CreateProductoParams{
		Nombre: "Con foto", Categoria: "hamburguesa", Precio: "14500", ImagenUrl: sql.NullString{String: foto, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	guardarAdminTest(t, tt.q)
	if body := pedirPanel(conSesion(tt.nav.mux, adminTest), "/admin", false).Body.String(); !strings.Contains(body, `src="`+foto+`"`) {
		t.Error("la tabla de productos tendría que mostrar la foto de R2")
	}
	if body := tt.nav.pedir(http.MethodGet, "/menu/"+idTexto(p.ID), nil, false).Body.String(); !strings.Contains(body, `src="`+foto+`"`) {
		t.Error("la pantalla de la burger tendría que mostrar la foto de R2")
	}
}
