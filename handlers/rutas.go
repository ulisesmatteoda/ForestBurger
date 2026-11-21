package handlers

import "net/http"

// RutasAdmin registra las rutas del panel del local. Las usan main.go y los tests, asi no se repiten
func (a *API) RutasAdmin(mux *http.ServeMux) {
	mux.HandleFunc("GET /admin", a.Index)
	mux.HandleFunc("POST /admin/productos", a.CrearProducto)
	mux.HandleFunc("GET /admin/productos/{id}", a.FilaProducto)
	mux.HandleFunc("GET /admin/productos/{id}/editar", a.FormEditarProducto)
	mux.HandleFunc("PUT /admin/productos/{id}", a.ActualizarProducto)
	mux.HandleFunc("DELETE /admin/productos/{id}", a.BorrarProducto)
}
