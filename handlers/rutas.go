package handlers

import (
	"net/http"

	"forestburger/views"
)

// Rutas registra todo lo de la app que pasa por la API: el login (público) y el panel (protegido).
// La usan main.go y los tests, así los tests prueban exactamente lo mismo que corre.
func (a *API) Rutas(mux *http.ServeMux) {
	a.RutasAuth(mux)
	// Las fotos son públicas: las va a mostrar el menú para los clientes.
	mux.Handle("GET /uploads/", a.servirFotos())

	// El panel va en un mux propio y entra entero por RequiereAdmin: una ruta nueva de /admin
	// queda protegida sola, sin acordarse de envolverla.
	admin := http.NewServeMux()
	a.RutasAdmin(admin)
	protegido := a.RequiereAdmin(admin)
	mux.Handle("/admin", protegido)
	mux.Handle("/admin/", protegido)
}

// RutasAuth registra el login y el logout, que son públicos.
func (a *API) RutasAuth(mux *http.ServeMux) {
	mux.HandleFunc("GET /login", a.FormLogin)
	mux.HandleFunc("POST /login", a.Login)
	mux.HandleFunc("POST /logout", a.Logout)
}

// RutasAdmin registra las rutas del panel del local, sin protección: Rutas las monta detrás de
// RequiereAdmin.
func (a *API) RutasAdmin(mux *http.ServeMux) {
	mux.HandleFunc("GET /admin", a.Index)
	// "nuevo" es un tramo literal: el mux lo prefiere antes que {id}, así no choca con la fila.
	mux.HandleFunc("GET /admin/productos/nuevo", a.NuevoProducto)
	mux.HandleFunc("GET "+views.RutaCerrarModal, a.CerrarModal)
	mux.HandleFunc("POST /admin/productos", a.CrearProducto)
	mux.HandleFunc("POST /admin/productos/foto-previa", a.FotoPrevia)
	mux.HandleFunc("GET /admin/productos/{id}", a.FilaProducto)
	mux.HandleFunc("GET /admin/productos/{id}/editar", a.FormEditarProducto)
	mux.HandleFunc("GET /admin/productos/{id}/borrar", a.ConfirmarBorrado)
	mux.HandleFunc("PUT /admin/productos/{id}", a.ActualizarProducto)
	mux.HandleFunc("DELETE /admin/productos/{id}", a.BorrarProducto)

	mux.HandleFunc("GET /admin/extras", a.Extras)
	mux.HandleFunc("GET /admin/extras/nuevo", a.NuevoExtra)
	mux.HandleFunc("POST /admin/extras", a.CrearExtra)
	mux.HandleFunc("GET /admin/extras/{id}/editar", a.EditarExtra)
	mux.HandleFunc("POST /admin/extras/{id}", a.ActualizarExtra)
	mux.HandleFunc("PUT /admin/extras/{id}/disponible", a.CambiarDisponibleExtra)
	mux.HandleFunc("GET /admin/extras/{id}/borrar", a.ConfirmarBorrarExtra)
	mux.HandleFunc("DELETE /admin/extras/{id}", a.BorrarExtra)

	mux.HandleFunc("GET /admin/ajustes", a.Ajustes)
	mux.HandleFunc("POST /admin/ajustes", a.GuardarAjustes)
	mux.HandleFunc("GET /admin/ajustes/transferencia", a.VistaTransferencia)
	mux.HandleFunc("GET /admin/ajustes/cambios", a.AvisoCambios)
	mux.HandleFunc("PUT /admin/local/abierto", a.CambiarLocalAbierto)
}
