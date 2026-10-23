package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	sqlc "forestburger/db/sqlc"
)

// ---------- DTOs (lo que ve el cliente de la API) ----------

// No trae precio: lo saca el servidor de la tabla extra.
type extraItemRequest struct {
	IDExtra  int32 `json:"id_extra"`
	Cantidad int32 `json:"cantidad"`
}

// itemRequest es un producto del carrito, con su cantidad, sus aclaraciones y sus extras.
// No trae precio: lo saca el servidor de la tabla producto.
type itemRequest struct {
	IDProducto   int32              `json:"id_producto"`
	Cantidad     int32              `json:"cantidad"`
	Aclaraciones *string            `json:"aclaraciones"` // texto libre sin precio (ej: "sin cebolla"); puede ser null
	Extras       []extraItemRequest `json:"extras"`       // puede venir vacío
}

// extraItemResponse es un extra guardado en un item del pedido.
type extraItemResponse struct {
	IDExtra        int32   `json:"id_extra"`
	NombreExtra    string  `json:"nombre_extra"`
	Cantidad       int32   `json:"cantidad"`
	PrecioUnitario float64 `json:"precio_unitario"`
}

// itemResponse es un producto guardado en el pedido, con sus extras.
type itemResponse struct {
	IDProducto     int32               `json:"id_producto"`
	NombreProducto string              `json:"nombre_producto"`
	Cantidad       int32               `json:"cantidad"`
	PrecioUnitario float64             `json:"precio_unitario"`
	Aclaraciones   *string             `json:"aclaraciones"`
	Extras         []extraItemResponse `json:"extras"`
}

// pedidoDetalleResponse es el pedido completo: sus datos más el carrito guardado.
// pedidoResponse va embebido, así sus campos salen al mismo nivel en el JSON.
type pedidoDetalleResponse struct {
	pedidoResponse
	Items []itemResponse `json:"items"`
}

type estadoRequest struct {
	Estado string `json:"estado"`
}

type pedidoRequest struct {
	NombreCliente string        `json:"nombre_cliente"`
	TipoEntrega   string        `json:"tipo_entrega"`
	Direccion     *string       `json:"direccion"`
	FormaPago     string        `json:"forma_pago"`
	Notas         *string       `json:"notas"`
	Items         []itemRequest `json:"items"` // el carrito: al menos un producto
}

type pedidoResponse struct {
	ID            int32     `json:"id"`
	NombreCliente string    `json:"nombre_cliente"`
	TipoEntrega   string    `json:"tipo_entrega"`
	Direccion     *string   `json:"direccion"`
	FormaPago     string    `json:"forma_pago"`
	Notas         *string   `json:"notas"`
	Total         float64   `json:"total"`
	Estado        string    `json:"estado"`
	Fecha         time.Time `json:"fecha"`
}

func (r pedidoRequest) validate() error {
	switch {
	case strings.TrimSpace(r.NombreCliente) == "":
		return errors.New("Ingresá tu nombre")
	case r.TipoEntrega != "delivery" && r.TipoEntrega != "take_away":
		return errors.New("Elegí si querés delivery o retirar en el local")
	case r.TipoEntrega == "delivery" && (r.Direccion == nil || strings.TrimSpace(*r.Direccion) == ""):
		return errors.New("Ingresá la dirección de entrega")
	case r.FormaPago != "efectivo" && r.FormaPago != "transferencia":
		return errors.New("Forma de pago inválida")
	}

	// Valida que el carrito no este vacío y que las cantidades sean al menos 1.
	if len(r.Items) == 0 {
		return errors.New("Agregá al menos un producto")
	}
	for _, it := range r.Items {
		if it.Cantidad < 1 {
			return errors.New("La cantidad tiene que ser al menos 1")
		}
		for _, ex := range it.Extras {
			if ex.Cantidad < 1 {
				return errors.New("La cantidad de un extra tiene que ser al menos 1")
			}
		}
	}

	return nil
}

func (r estadoRequest) validate() error {
	switch r.Estado {
	case "pendiente", "confirmado", "cancelado", "entregado":
		return nil
	}
	return errors.New("Estado inválido")
}

func (a *API) toResponse(p sqlc.Pedido) pedidoResponse {
	total, _ := strconv.ParseFloat(p.Total, 64)
	return pedidoResponse{
		ID:            p.ID,
		NombreCliente: p.NombreCliente,
		TipoEntrega:   p.TipoEntrega,
		Direccion:     nullToPtr(p.Direccion),
		FormaPago:     p.FormaPago,
		Notas:         nullToPtr(p.Notas),
		Total:         total,
		Estado:        p.Estado,
		Fecha:         p.Fecha.Time,
	}
}

// decodePedido lee y valida el body; si hay error ya responde al cliente.
func decodePedido(w http.ResponseWriter, r *http.Request) (pedidoRequest, bool) {
	var req pedidoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "JSON inválido")
		return req, false
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return req, false
	}
	return req, true
}

// ---------- handlers ----------

// detallePedido busca los items y extras de un pedido y arma la respuesta completa.
func (a *API) detallePedido(ctx context.Context, p sqlc.Pedido) (pedidoDetalleResponse, error) {
	items, err := a.q.ListItemsByPedido(ctx, p.ID)
	if err != nil {
		return pedidoDetalleResponse{}, err
	}

	out := make([]itemResponse, 0, len(items))
	for _, it := range items {
		extras, err := a.q.ListExtrasByItem(ctx, it.ID)
		if err != nil {
			return pedidoDetalleResponse{}, err
		}

		extrasOut := make([]extraItemResponse, 0, len(extras)) // "[]" y no "null" si no tiene extras
		for _, ex := range extras {
			precioExtra, _ := strconv.ParseFloat(ex.PrecioUnitario, 64)
			extrasOut = append(extrasOut, extraItemResponse{
				IDExtra:        ex.IDExtra,
				NombreExtra:    ex.NombreExtra,
				Cantidad:       ex.Cantidad,
				PrecioUnitario: precioExtra,
			})
		}

		precio, _ := strconv.ParseFloat(it.PrecioUnitario, 64)
		out = append(out, itemResponse{
			IDProducto:     it.IDProducto,
			NombreProducto: it.NombreProducto,
			Cantidad:       it.Cantidad,
			PrecioUnitario: precio,
			Aclaraciones:   nullToPtr(it.Aclaraciones),
			Extras:         extrasOut,
		})
	}

	return pedidoDetalleResponse{
		pedidoResponse: a.toResponse(p),
		Items:          out,
	}, nil
}

func (a *API) CreatePedido(w http.ResponseWriter, r *http.Request) {
	req, ok := decodePedido(w, r)
	if !ok {
		return
	}
	ctx := r.Context()

	pedido, err := a.q.CreatePedido(ctx, sqlc.CreatePedidoParams{
		NombreCliente: req.NombreCliente,
		TipoEntrega:   req.TipoEntrega,
		Direccion:     toNullString(req.Direccion),
		FormaPago:     req.FormaPago,
		Notas:         toNullString(req.Notas),
		Total:         "0.00", // se calcula después
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Recorrer el carrito: guardar cada producto y sus extras, e ir sumando.
	var total float64

	for _, it := range req.Items {
		// El precio se saca de la base, nunca del cliente.
		producto, err := a.q.GetProducto(ctx, it.IDProducto)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Uno de los productos no existe")
			return
		}

		// Crear el item del pedido
		item, err := a.q.CreateItemPedido(ctx, sqlc.CreateItemPedidoParams{
			IDPedido:       pedido.ID,
			IDProducto:     it.IDProducto,
			Cantidad:       it.Cantidad,
			PrecioUnitario: producto.Precio,
			Aclaraciones:   toNullString(it.Aclaraciones),
		})

		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		// Calcular el total: precio del producto * cantidad + extras
		precio, _ := strconv.ParseFloat(producto.Precio, 64)
		total += precio * float64(it.Cantidad)

		for _, ex := range it.Extras {
			extra, err := a.q.GetExtra(ctx, ex.IDExtra)
			if err != nil {
				writeError(w, http.StatusBadRequest, "Uno de los extras no existe")
				return
			}

			_, err = a.q.CreateItemPedidoExtra(ctx, sqlc.CreateItemPedidoExtraParams{
				IDItemPedido:   item.ID,
				IDExtra:        ex.IDExtra,
				Cantidad:       ex.Cantidad,
				PrecioUnitario: extra.Precio,
			})
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}

			precioExtra, _ := strconv.ParseFloat(extra.Precio, 64)
			total += precioExtra * float64(ex.Cantidad)
		}
	}
	// 3. Guardar el total calculado.
	pedido, err = a.q.UpdateTotalPedido(ctx, sqlc.UpdateTotalPedidoParams{
		ID:    pedido.ID,
		Total: floatToStr(total),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	detalle, err := a.detallePedido(ctx, pedido)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, detalle)
}

func (a *API) ListPedidos(w http.ResponseWriter, r *http.Request) {
	pedidos, err := a.q.ListPedidos(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
	}

	out := make([]pedidoResponse, 0, len(pedidos))
	for _, p := range pedidos {
		out = append(out, a.toResponse(p))
	}

	writeJSON(w, http.StatusOK, out)
}

// GetPedido: GET /api/pedidos/{id}
// Devuelve el pedido con todo lo que se pidió (items y extras).
func (a *API) GetPedido(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id inválido")
		return
	}

	pedido, err := a.q.GetPedido(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "pedido no encontrado")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	detalle, err := a.detallePedido(r.Context(), pedido)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, detalle)
}

func (a *API) UpdateEstadoPedido(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id inválido")
		return
	}

	var req estadoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	pedido, err := a.q.UpdateEstadoPedido(r.Context(), sqlc.UpdateEstadoPedidoParams{
		ID:     id,
		Estado: req.Estado,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "pedido no encontrado")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, a.toResponse(pedido))
}
