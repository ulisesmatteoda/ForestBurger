package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Las fotos de los productos van a disco (a.dirFotos, "uploads" en la app) y la base guarda solo
// su ruta pública, "/uploads/<nombre>". Misma idea que la API JSON de tp3-extendido.

const (
	maxFoto     = 5 << 20         // 5 MB por foto
	maxPedido   = maxFoto + 1<<20 // la foto más el resto del formulario
	rutaFotos   = "/uploads/"
	campoFoto   = "foto"
	errorTipo   = "La foto tiene que ser JPG, PNG o WEBP"
	errorTamano = "La foto no puede pesar más de 5 MB"
)

// Tipos aceptados, por contenido y no por extensión (la extensión se puede falsear).
var extensionesFoto = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

// fotoSubida es una foto ya validada, lista para guardar.
type fotoSubida struct {
	datos  []byte
	ext    string
	tipo   string // "image/png", etc.: el detectado por el contenido
	nombre string // el nombre que tenía en la compu de quien la subió, solo para mostrarlo
}

// parsearFormulario lee un formulario que puede traer una foto (multipart) o no (urlencoded),
// con un tope de tamaño para que nadie mande un archivo gigante.
func parsearFormulario(w http.ResponseWriter, r *http.Request) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxPedido)
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		return r.ParseMultipartForm(maxPedido)
	}
	return r.ParseForm()
}

// leerFoto devuelve la foto del formulario si vino una (es opcional: nil sin error si no vino).
// Si vino algo que no sirve, devuelve el mensaje para mostrarle al usuario.
func leerFoto(r *http.Request) (*fotoSubida, string) {
	archivo, cabecera, err := r.FormFile(campoFoto)
	if errors.Is(err, http.ErrMissingFile) || errors.Is(err, http.ErrNotMultipart) {
		return nil, ""
	}
	if err != nil {
		return nil, errorTipo
	}
	defer archivo.Close()

	// Se lee un byte de más para saber si se pasó del límite.
	datos, err := io.ReadAll(io.LimitReader(archivo, maxFoto+1))
	if err != nil {
		return nil, errorTipo
	}
	if len(datos) == 0 {
		return nil, "" // el input de archivo vacío llega como un archivo sin contenido
	}
	if len(datos) > maxFoto {
		return nil, errorTamano
	}
	tipo := http.DetectContentType(datos)
	ext, ok := extensionesFoto[tipo]
	if !ok {
		return nil, errorTipo
	}
	return &fotoSubida{datos: datos, ext: ext, tipo: tipo, nombre: filepath.Base(cabecera.Filename)}, ""
}

// errorFormulario traduce el error de parsearFormulario a un mensaje: casi siempre es que el
// pedido pasó el tope de tamaño (por la foto).
func errorFormulario(err error) string {
	var demasiado *http.MaxBytesError
	if errors.As(err, &demasiado) {
		return errorTamano
	}
	return "Formulario inválido"
}

// guardarFoto la escribe con un nombre al azar y devuelve su ruta pública. Un nombre nuevo en cada
// subida evita que el navegador muestre una foto vieja que tenía guardada en caché.
func (a *API) guardarFoto(f *fotoSubida) (string, error) {
	azar := make([]byte, 8)
	if _, err := rand.Read(azar); err != nil {
		return "", err
	}
	nombre := hex.EncodeToString(azar) + f.ext
	if err := os.MkdirAll(a.dirFotos, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(a.dirFotos, nombre), f.datos, 0o644); err != nil {
		return "", err
	}
	return rutaFotos + nombre, nil
}

// borrarFoto borra el archivo de una ruta "/uploads/<nombre>". filepath.Base se queda con el
// nombre solo, así una ruta rara no puede borrar nada fuera de la carpeta. Si no existe, no pasa nada.
func (a *API) borrarFoto(ruta string) {
	if !strings.HasPrefix(ruta, rutaFotos) {
		return
	}
	os.Remove(filepath.Join(a.dirFotos, filepath.Base(ruta)))
}

// servirFotos sirve la carpeta de fotos sin listarla: /uploads/ a secas da 404.
func (a *API) servirFotos() http.Handler {
	archivos := http.StripPrefix(rutaFotos, http.FileServer(http.Dir(a.dirFotos)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		archivos.ServeHTTP(w, r)
	})
}
