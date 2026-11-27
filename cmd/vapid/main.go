// Comando vapid: genera las claves para las notificaciones push (una vez por ambiente). Se corre
// con `make vapid` y la salida va a las variables del servidor (el .env o Railway).
package main

import (
	"fmt"
	"log"

	"github.com/SherClockHolmes/webpush-go"
)

func main() {
	privada, publica, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("# Claves para las notificaciones push. La privada no se comparte ni se sube al repo.")
	fmt.Println("VAPID_PUBLIC_KEY=" + publica)
	fmt.Println("VAPID_PRIVATE_KEY=" + privada)
	fmt.Println("# Un mail de contacto para los servicios de push (Google, Apple, Mozilla).")
	fmt.Println("VAPID_SUBJECT=mailto:tu-mail@ejemplo.com")
}
