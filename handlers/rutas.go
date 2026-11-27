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
	// La web del cliente: pública, sin sesión.
	mux.HandleFunc("GET /{$}", a.Inicio)
	mux.HandleFunc("GET /menu", a.Menu)
	mux.HandleFunc("GET /menu/{id}", a.Burger)
	mux.HandleFunc("POST /menu/{id}/armar", a.ArmarBurger)
	mux.HandleFunc("POST /carrito", a.AgregarAlCarrito)
	mux.HandleFunc("POST /carrito/rapido", a.CambiarRapido)
	mux.HandleFunc("GET /promo/{id}", a.PromoArmar)
	mux.HandleFunc("POST /promo/{id}/armar", a.ArmarPromo)
	mux.HandleFunc("POST /carrito/promo", a.AgregarPromo)
	mux.HandleFunc("GET /carrito", a.Carrito)
	mux.HandleFunc("POST /carrito/linea", a.CambiarLinea)
	mux.HandleFunc("POST /carrito/recalcular", a.RecalcularCarrito)
	mux.HandleFunc("POST /carrito/revisar", a.RevisarCarrito)
	mux.HandleFunc("GET /carrito/revisar", a.Revisar)
	mux.HandleFunc("POST /pedido", a.ConfirmarPedido)
	mux.HandleFunc("GET /pedido/{token}", a.Seguimiento)
	mux.HandleFunc("GET /pedido/{token}/estado", a.EstadoSeguimiento)

	// El service worker de las notificaciones push (público: el navegador lo pide sin sesión).
	mux.HandleFunc("GET /sw.js", ServiceWorker)

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
	mux.HandleFunc("PUT /admin/productos/{id}/disponible", a.CambiarDisponibleProducto)

	mux.HandleFunc("GET /admin/promos", a.Promos)
	mux.HandleFunc("GET /admin/promos/nueva", a.NuevaPromo)
	mux.HandleFunc("POST /admin/promos/vista", a.VistaPromo)
	mux.HandleFunc("POST /admin/promos", a.CrearPromo)
	mux.HandleFunc("GET /admin/promos/{id}/editar", a.EditarPromo)
	mux.HandleFunc("POST /admin/promos/{id}", a.ActualizarPromo)
	mux.HandleFunc("PUT /admin/promos/{id}/activa", a.CambiarActivaPromo)
	mux.HandleFunc("GET /admin/promos/{id}/borrar", a.ConfirmarBorrarPromo)
	mux.HandleFunc("DELETE /admin/promos/{id}", a.BorrarPromo)
	mux.HandleFunc("DELETE /admin/productos/{id}", a.BorrarProducto)

	mux.HandleFunc("GET /admin/extras", a.Extras)
	mux.HandleFunc("GET /admin/extras/nuevo", a.NuevoExtra)
	mux.HandleFunc("POST /admin/extras", a.CrearExtra)
	mux.HandleFunc("GET /admin/extras/{id}/editar", a.EditarExtra)
	mux.HandleFunc("POST /admin/extras/{id}", a.ActualizarExtra)
	mux.HandleFunc("PUT /admin/extras/{id}/disponible", a.CambiarDisponibleExtra)
	mux.HandleFunc("GET /admin/extras/{id}/borrar", a.ConfirmarBorrarExtra)
	mux.HandleFunc("DELETE /admin/extras/{id}", a.BorrarExtra)

	mux.HandleFunc("GET /admin/pedidos", a.Pedidos)
	// "cargar" es un tramo literal: el mux lo prefiere antes que {id}.
	mux.HandleFunc("GET /admin/pedidos/aviso", a.AvisoPedido)
	mux.HandleFunc("GET /admin/pedidos/cargar", a.CargarPedido)
	mux.HandleFunc("POST /admin/pedidos/cargar/armar", a.ArmarPedido)
	mux.HandleFunc("POST /admin/pedidos/cargar", a.GuardarPedido)
	mux.HandleFunc("GET /admin/pedidos/{id}", a.DetallePedido)
	mux.HandleFunc("GET /admin/pedidos/{id}/baja", a.ConfirmarBajaPedido)
	mux.HandleFunc("POST /admin/pedidos/{id}/{accion}", a.AccionPedido)

	mux.HandleFunc("GET /admin/ajustes", a.Ajustes)
	mux.HandleFunc("POST /admin/ajustes", a.GuardarAjustes)
	mux.HandleFunc("GET /admin/ajustes/transferencia", a.VistaTransferencia)
	mux.HandleFunc("GET /admin/ajustes/cambios", a.AvisoCambios)
	mux.HandleFunc("POST /admin/push/suscribir", a.SuscribirPush)
	mux.HandleFunc("POST /admin/push/desuscribir", a.DesuscribirPush)
	mux.HandleFunc("POST /admin/push/prueba", a.ProbarPush)
	mux.HandleFunc("PUT /admin/local/abierto", a.CambiarLocalAbierto)
}
