package handlers

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	sqlc "forestburger/db/sqlc"
)

// setupFotos es setup (con sesión de admin) más la carpeta temporal donde quedan las fotos.
func setupFotos(t *testing.T) (http.Handler, *sqlc.Queries, string) {
	t.Helper()
	mux, q, api := setupAPI(t)
	guardarAdminTest(t, q)
	return conSesion(mux, adminTest), q, api.dirFotos
}

// pngDePrueba es un PNG chico de verdad (DetectContentType lo reconoce por el contenido).
func pngDePrueba(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// multipartCon arma un pedido multipart con los campos y, si archivo no es nil, la foto.
func multipartCon(t *testing.T, metodo, ruta string, campos map[string]string, archivo []byte) *http.Request {
	t.Helper()
	var cuerpo bytes.Buffer
	mw := multipart.NewWriter(&cuerpo)
	for k, v := range campos {
		mw.WriteField(k, v)
	}
	if archivo != nil {
		fw, err := mw.CreateFormFile("foto", "foto.png")
		if err != nil {
			t.Fatal(err)
		}
		fw.Write(archivo)
	}
	mw.Close()
	req := httptest.NewRequest(metodo, ruta, &cuerpo)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("HX-Request", "true")
	return req
}

func servir(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// archivosEn cuenta los archivos de la carpeta de fotos.
func archivosEn(t *testing.T, dir string) []string {
	t.Helper()
	entradas, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var nombres []string
	for _, e := range entradas {
		nombres = append(nombres, e.Name())
	}
	return nombres
}

var camposValidos = map[string]string{"nombre": "Forest Clásica", "categoria": "hamburguesa", "precio": "9500"}

func TestAltaConFoto_GuardaArchivoYRuta(t *testing.T) {
	mux, q, dir := setupFotos(t)

	rec := servir(mux, multipartCon(t, http.MethodPost, "/admin/productos", camposValidos, pngDePrueba(t)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `id="panel-productos"`) {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	ps, _ := q.ListProductos(context.Background())
	if len(ps) != 1 || !strings.HasPrefix(ps[0].ImagenUrl.String, "/uploads/") || !strings.HasSuffix(ps[0].ImagenUrl.String, ".png") {
		t.Fatalf("imagen_url = %+v, quiero /uploads/<nombre>.png", ps[0].ImagenUrl)
	}
	if archivos := archivosEn(t, dir); len(archivos) != 1 || "/uploads/"+archivos[0] != ps[0].ImagenUrl.String {
		t.Errorf("archivos en la carpeta = %v, quiero solo el de %s", archivos, ps[0].ImagenUrl.String)
	}
	if !strings.Contains(rec.Body.String(), `<img class="miniatura" src="`+ps[0].ImagenUrl.String+`"`) {
		t.Error("la fila no muestra la miniatura")
	}

	// Y se sirve públicamente.
	req := httptest.NewRequest(http.MethodGet, ps[0].ImagenUrl.String, nil)
	if foto := servir(mux, req); foto.Code != http.StatusOK || foto.Header().Get("Content-Type") != "image/png" {
		t.Errorf("GET %s: status %d, tipo %q", ps[0].ImagenUrl.String, foto.Code, foto.Header().Get("Content-Type"))
	}
}

func TestAltaSinFoto_MuestraRecuadroVacio(t *testing.T) {
	mux, q, dir := setupFotos(t)

	rec := servir(mux, multipartCon(t, http.MethodPost, "/admin/productos", camposValidos, nil))
	ps, _ := q.ListProductos(context.Background())
	if len(ps) != 1 || ps[0].ImagenUrl.Valid {
		t.Fatalf("quiero un producto sin foto, hay %+v", ps)
	}
	if !strings.Contains(rec.Body.String(), `class="miniatura vacia"`) {
		t.Error("sin foto tendría que mostrar el recuadro vacío")
	}
	if archivos := archivosEn(t, dir); len(archivos) != 0 {
		t.Errorf("no tendría que haber archivos, hay %v", archivos)
	}
}

func TestAltaConFotoInvalida_NoGuardaNada(t *testing.T) {
	casos := map[string]struct {
		archivo []byte
		mensaje string
	}{
		"no es imagen":  {[]byte("esto es texto, no una foto"), "La foto tiene que ser JPG, PNG o WEBP"},
		"pasa los 5 MB": {append(pngDePrueba(t), make([]byte, maxFoto)...), "La foto no puede pesar más de 5 MB"},
	}
	for nombre, c := range casos {
		t.Run(nombre, func(t *testing.T) {
			mux, q, dir := setupFotos(t)

			rec := servir(mux, multipartCon(t, http.MethodPost, "/admin/productos", camposValidos, c.archivo))
			body := rec.Body.String()
			if !strings.Contains(body, c.mensaje) || !strings.Contains(body, `role="dialog"`) {
				t.Errorf("el modal no muestra %q: %s", c.mensaje, body)
			}
			if ps, _ := q.ListProductos(context.Background()); len(ps) != 0 {
				t.Errorf("no tendría que guardar el producto, hay %d", len(ps))
			}
			if archivos := archivosEn(t, dir); len(archivos) != 0 {
				t.Errorf("no tendría que guardar archivos, hay %v", archivos)
			}
		})
	}
}

// crearConFoto da de alta un producto con foto y devuelve su id y la ruta de la foto.
func crearConFoto(t *testing.T, mux http.Handler, q *sqlc.Queries) (string, string) {
	t.Helper()
	servir(mux, multipartCon(t, http.MethodPost, "/admin/productos", camposValidos, pngDePrueba(t)))
	ps, _ := q.ListProductos(context.Background())
	if len(ps) != 1 || !ps[0].ImagenUrl.Valid {
		t.Fatalf("no se creó el producto con foto: %+v", ps)
	}
	return strconv.Itoa(int(ps[0].ID)), ps[0].ImagenUrl.String
}

func TestEditar_FotoNuevaReemplazaYBorraLaVieja(t *testing.T) {
	mux, q, dir := setupFotos(t)
	id, vieja := crearConFoto(t, mux, q)

	rec := servir(mux, multipartCon(t, http.MethodPut, "/admin/productos/"+id, camposValidos, pngDePrueba(t)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	p, _ := q.GetProducto(context.Background(), mustAtoi(t, id))
	if !p.ImagenUrl.Valid || p.ImagenUrl.String == vieja {
		t.Fatalf("la foto no cambió: %+v", p.ImagenUrl)
	}
	archivos := archivosEn(t, dir)
	if len(archivos) != 1 || "/uploads/"+archivos[0] != p.ImagenUrl.String {
		t.Errorf("tendría que quedar solo la foto nueva, hay %v", archivos)
	}
}

func TestEditar_QuitarFoto(t *testing.T) {
	mux, q, dir := setupFotos(t)
	id, _ := crearConFoto(t, mux, q)

	campos := map[string]string{"quitar_foto": "1"}
	for k, v := range camposValidos {
		campos[k] = v
	}
	rec := servir(mux, multipartCon(t, http.MethodPut, "/admin/productos/"+id, campos, nil))
	if !strings.Contains(rec.Body.String(), `class="miniatura vacia"`) {
		t.Error("la fila tendría que volver sin foto")
	}
	p, _ := q.GetProducto(context.Background(), mustAtoi(t, id))
	if p.ImagenUrl.Valid {
		t.Errorf("imagen_url tendría que ser NULL, es %q", p.ImagenUrl.String)
	}
	if archivos := archivosEn(t, dir); len(archivos) != 0 {
		t.Errorf("tendría que borrar el archivo, hay %v", archivos)
	}
}

func TestEditar_SinFotoNuevaLaConserva(t *testing.T) {
	mux, q, dir := setupFotos(t)
	id, vieja := crearConFoto(t, mux, q)

	servir(mux, multipartCon(t, http.MethodPut, "/admin/productos/"+id, camposValidos, nil))
	p, _ := q.GetProducto(context.Background(), mustAtoi(t, id))
	if p.ImagenUrl.String != vieja || len(archivosEn(t, dir)) != 1 {
		t.Errorf("editar sin foto nueva no tendría que tocar la foto: %+v", p.ImagenUrl)
	}
}

func TestBorrarProducto_BorraSuFoto(t *testing.T) {
	mux, q, dir := setupFotos(t)
	id, _ := crearConFoto(t, mux, q)

	req := httptest.NewRequest(http.MethodDelete, "/admin/productos/"+id, nil)
	req.Header.Set("HX-Request", "true")
	servir(mux, req)
	if archivos := archivosEn(t, dir); len(archivos) != 0 {
		t.Errorf("tendría que borrar la foto del producto, hay %v", archivos)
	}
}

func TestUploads_NoListaLaCarpeta(t *testing.T) {
	mux, q, dir := setupFotos(t)
	crearConFoto(t, mux, q)
	if len(archivosEn(t, dir)) != 1 {
		t.Fatal("falta la foto de prueba")
	}
	if rec := servir(mux, httptest.NewRequest(http.MethodGet, "/uploads/", nil)); rec.Code != http.StatusNotFound {
		t.Errorf("GET /uploads/: status %d, quiero 404 (no listar)", rec.Code)
	}
	if rec := servir(mux, httptest.NewRequest(http.MethodGet, "/uploads/no-existe.png", nil)); rec.Code != http.StatusNotFound {
		t.Errorf("foto inexistente: status %d, quiero 404", rec.Code)
	}
}

func mustAtoi(t *testing.T, s string) int32 {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatal(err)
	}
	return int32(n)
}

// ---------- vista previa al elegir la foto ----------

func TestFotoPrevia_MuestraLaFotoSinGuardarla(t *testing.T) {
	mux, _, dir := setupFotos(t)

	rec := servir(mux, multipartCon(t, http.MethodPost, "/admin/productos/foto-previa", nil, pngDePrueba(t)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quiero 200", rec.Code)
	}
	body := rec.Body.String()
	for _, esperado := range []string{`src="data:image/png;base64,`, "✓ foto.png", "Tocá para cambiarla", "KB"} {
		if !strings.Contains(body, esperado) {
			t.Errorf("la vista previa no contiene %q: %s", esperado, body)
		}
	}
	if archivos := archivosEn(t, dir); len(archivos) != 0 {
		t.Errorf("la vista previa no tiene que guardar nada en disco, hay %v", archivos)
	}
}

func TestFotoPrevia_AvisaSiNoSirve(t *testing.T) {
	mux, _, _ := setupFotos(t)

	rec := servir(mux, multipartCon(t, http.MethodPost, "/admin/productos/foto-previa", nil, []byte("no soy una foto")))
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, `class="foto-error"`) || !strings.Contains(body, "La foto tiene que ser JPG, PNG o WEBP") {
		t.Errorf("status = %d, body = %s; quiero 200 con el error", rec.Code, body)
	}
}

func TestFotoPrevia_SinArchivoNoMuestraNada(t *testing.T) {
	mux, _, _ := setupFotos(t)

	rec := servir(mux, multipartCon(t, http.MethodPost, "/admin/productos/foto-previa", nil, nil))
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Errorf("status = %d, body = %q; quiero 200 vacío", rec.Code, rec.Body.String())
	}
}

func TestFotoPrevia_PideSesion(t *testing.T) {
	mux, _ := setupSinSesion(t)

	rec := servir(mux, multipartCon(t, http.MethodPost, "/admin/productos/foto-previa", nil, pngDePrueba(t)))
	if got := rec.Header().Get("HX-Redirect"); got != "/login" {
		t.Errorf("sin sesión: HX-Redirect = %q, quiero /login", got)
	}
}

func TestModal_ZonaDeFotoPideLaVistaPrevia(t *testing.T) {
	mux, _, _ := setupFotos(t)

	body := pedirPanel(mux, "/admin/productos/nuevo", true).Body.String()
	for _, esperado := range []string{`hx-post="/admin/productos/foto-previa"`, `hx-trigger="change"`, `hx-encoding="multipart/form-data"`, `hx-target="next .foto-previa"`, `class="foto-previa"`} {
		if !strings.Contains(body, esperado) {
			t.Errorf("el modal no contiene %q", esperado)
		}
	}
}
