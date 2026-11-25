package handlers

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Las fotos en Cloudflare R2 (compatible con S3), para staging y producción: el disco de un
// contenedor en Railway se borra en cada deploy. En local no se usa: sin las variables, las fotos
// van a uploads/ como siempre.

// variablesR2 son las que hacen falta para guardar las fotos en R2 (todas o ninguna).
var variablesR2 = []string{"S3_ENDPOINT", "S3_BUCKET", "S3_ACCESS_KEY_ID", "S3_SECRET_ACCESS_KEY", "FOTOS_URL_PUBLICA"}

// almacenR2 sube y borra las fotos de un bucket. urlPublica es desde donde las ve el navegador
// ("https://pub-xxxx.r2.dev", sin la barra final).
type almacenR2 struct {
	cliente    *minio.Client
	bucket     string
	urlPublica string
}

// ConfigurarFotos elige dónde van las fotos según las variables de entorno: sin ninguna, al disco
// (uploads/, como en local); con todas, a R2. Con algunas sí y otras no es un error de
// configuración y la app no arranca, así no se pierden fotos en un disco que se borra.
func (a *API) ConfigurarFotos() error {
	valores := map[string]string{}
	var faltan []string
	for _, v := range variablesR2 {
		valores[v] = strings.TrimSpace(os.Getenv(v))
		if valores[v] == "" {
			faltan = append(faltan, v)
		}
	}
	switch len(faltan) {
	case len(variablesR2):
		return nil // ninguna: disco
	case 0:
	default:
		return fmt.Errorf("faltan %s para guardar las fotos en R2 (o se ponen todas, o ninguna)", strings.Join(faltan, ", "))
	}
	r2, err := nuevoAlmacenR2(valores["S3_ENDPOINT"], valores["S3_BUCKET"], valores["S3_ACCESS_KEY_ID"], valores["S3_SECRET_ACCESS_KEY"], valores["FOTOS_URL_PUBLICA"])
	if err != nil {
		return err
	}
	a.r2 = r2
	log.Printf("Fotos en R2: bucket %s, se ven en %s", r2.bucket, r2.urlPublica)
	return nil
}

func nuevoAlmacenR2(endpoint, bucket, accessKey, secretKey, urlPublica string) (*almacenR2, error) {
	// minio quiere el host solo ("<cuenta>.r2.cloudflarestorage.com"); se acepta con https://.
	host := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://"), "/")
	cliente, err := minio.New(host, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: !strings.HasPrefix(endpoint, "http://"),
		Region: "auto", // R2 no usa regiones: "auto" es lo que pide
	})
	if err != nil {
		return nil, fmt.Errorf("S3_ENDPOINT inválido: %w", err)
	}
	return &almacenR2{cliente: cliente, bucket: bucket, urlPublica: strings.TrimSuffix(urlPublica, "/")}, nil
}

// guardar sube la foto y devuelve su URL pública. El nombre es al azar y nunca se reusa, así que
// el navegador la puede guardar en caché para siempre.
func (s *almacenR2) guardar(nombre string, datos []byte, tipo string) (string, error) {
	ctx, cancelar := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelar()
	_, err := s.cliente.PutObject(ctx, s.bucket, nombre, bytes.NewReader(datos), int64(len(datos)), minio.PutObjectOptions{
		ContentType:  tipo,
		CacheControl: "public, max-age=31536000, immutable",
	})
	if err != nil {
		return "", fmt.Errorf("no se pudo subir la foto: %w", err)
	}
	return s.urlPublica + "/" + nombre, nil
}

// esDeAca dice si la URL es de una foto de este bucket (las viejas, "/uploads/...", no).
func (s *almacenR2) esDeAca(url string) bool {
	return strings.HasPrefix(url, s.urlPublica+"/")
}

// borrar saca la foto del bucket. Si falla solo se avisa en el log: la foto queda huérfana, pero
// el producto se guarda o se borra igual.
func (s *almacenR2) borrar(url string) {
	if !s.esDeAca(url) {
		return
	}
	nombre := strings.TrimPrefix(url, s.urlPublica+"/")
	ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelar()
	if err := s.cliente.RemoveObject(ctx, s.bucket, nombre, minio.RemoveObjectOptions{}); err != nil {
		log.Printf("No se pudo borrar la foto %s de R2: %v", nombre, err)
	}
}
