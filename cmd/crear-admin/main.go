// Comando crear-admin: da de alta un admin del panel, o le cambia la contraseña si ya existe.
// Es la única forma de crear uno: no hay registro público. Se corre con `make crear-admin`.
package main

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"

	"forestburger/auth"
	sqlc "forestburger/db/sqlc"

	_ "github.com/lib/pq"
	"golang.org/x/term"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run() error {
	// La contraseña se lee sin eco, y eso solo se puede en una terminal.
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return errors.New("este comando se usa desde una terminal (make crear-admin)")
	}

	// Misma conexión que main.go.
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		connStr = "postgres://usuario:password@localhost:5432/forestburger?sslmode=disable"
	}
	conn, err := sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("abriendo conexión: %w", err)
	}
	defer conn.Close()
	if err := conn.Ping(); err != nil {
		return fmt.Errorf("no se pudo conectar a la base (¿está levantada?): %w", err)
	}

	lector := bufio.NewReader(os.Stdin)
	fmt.Print("Email: ")
	linea, err := lector.ReadString('\n')
	if err != nil {
		return errors.New("no se pudo leer el email")
	}
	email := auth.NormalizarEmail(linea)
	if !strings.Contains(email, "@") {
		return fmt.Errorf("%q no parece un email", email)
	}
	if !auth.EmailPermitido(email) {
		return fmt.Errorf("%s no está en ADMIN_EMAILS del .env: agregalo ahí (separado por coma) y volvé a correr el comando", email)
	}

	password, err := leerPassword("Contraseña: ")
	if err != nil {
		return err
	}
	if len(password) < auth.LargoMinimoPassword {
		return fmt.Errorf("la contraseña tiene que tener al menos %d caracteres", auth.LargoMinimoPassword)
	}
	repetida, err := leerPassword("Repetí la contraseña: ")
	if err != nil {
		return err
	}
	if password != repetida {
		return errors.New("las contraseñas no coinciden")
	}

	hash, err := auth.HashearPassword(password)
	if err != nil {
		return errors.New("no se pudo hashear la contraseña")
	}
	q := sqlc.New(conn)
	if _, err := q.GuardarAdmin(context.Background(), sqlc.GuardarAdminParams{Email: email, PasswordHash: hash}); err != nil {
		return fmt.Errorf("no se pudo guardar el admin: %w", err)
	}
	fmt.Printf("Admin %s guardado.\n", email)
	return nil
}

// leerPassword pide la contraseña sin mostrarla mientras se tipea.
func leerPassword(pregunta string) (string, error) {
	fmt.Print(pregunta)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println() // ReadPassword no deja el salto de línea del Enter
	if err != nil {
		return "", errors.New("no se pudo leer la contraseña")
	}
	return string(b), nil
}
