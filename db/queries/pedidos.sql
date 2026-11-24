-- name: CreatePedido :one
INSERT INTO pedido (
    creado_en, cliente, telefono, origen, entrega, direccion, pago, notas, estado, demora_min,
    subtotal, descuento_pct, descuento_motivo, descuento, costo_envio, total, token
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
RETURNING *;

-- name: CreateItemPedido :one
INSERT INTO item_pedido (pedido_id, producto_id, nombre, tamano, cantidad, precio_unitario, aclaraciones, promo_id, detalle)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: CreateItemPedidoExtra :exec
INSERT INTO item_pedido_extra (item_pedido_id, extra_id, nombre, cantidad, precio_unitario)
VALUES ($1, $2, $3, $4, $5);

-- name: CreateEventoPedido :exec
INSERT INTO pedido_evento (pedido_id, creado_en, estado, detalle)
VALUES ($1, $2, $3, $4);

-- name: GetPedido :one
SELECT * FROM pedido WHERE id = $1;

-- name: GetPedidoPorToken :one
-- La página de seguimiento del cliente (/pedido/{token}).
SELECT * FROM pedido WHERE token = $1;

-- name: ListItemsPedido :many
SELECT * FROM item_pedido WHERE pedido_id = $1 ORDER BY id;

-- name: ListExtrasItemsPedido :many
-- Los extras de todos los ítems de un pedido, en una sola consulta.
SELECT ipe.* FROM item_pedido_extra ipe
JOIN item_pedido ip ON ip.id = ipe.item_pedido_id
WHERE ip.pedido_id = $1
ORDER BY ipe.id;

-- name: ListEventosPedido :many
SELECT * FROM pedido_evento WHERE pedido_id = $1 ORDER BY creado_en, id;

-- name: CountPedidosEnCursoConProducto :one
-- Para avisar al borrar: en cuántos pedidos pendientes o confirmados aparece el producto.
SELECT count(DISTINCT p.id) FROM pedido p
JOIN item_pedido ip ON ip.pedido_id = p.id
WHERE ip.producto_id = $1 AND p.estado IN ('pendiente', 'confirmado');

-- name: CountPedidosEnCursoConExtra :one
SELECT count(DISTINCT p.id) FROM pedido p
JOIN item_pedido ip ON ip.pedido_id = p.id
JOIN item_pedido_extra ipe ON ipe.item_pedido_id = ip.id
WHERE ipe.extra_id = $1 AND p.estado IN ('pendiente', 'confirmado');

-- name: CountProductos :one
SELECT count(*) FROM producto;

-- name: ListPedidos :many
-- Los pedidos de un rango de fechas [desde, hasta), del más nuevo al más viejo. estado vacío trae
-- todos; "confirmado" incluye los en camino (siguen en curso).
SELECT * FROM pedido
WHERE creado_en >= sqlc.arg(desde) AND creado_en < sqlc.arg(hasta)
  AND (sqlc.arg(estado)::text = ''
       OR estado = sqlc.arg(estado)::text
       OR (sqlc.arg(estado)::text = 'confirmado' AND estado = 'en_camino'))
ORDER BY creado_en DESC;

-- name: ListResumenItems :many
-- Los ítems de varios pedidos a la vez (los de la lista), con los nombres de sus extras juntos:
-- una sola consulta en vez de una por pedido.
SELECT ip.pedido_id, ip.cantidad, ip.nombre, ip.tamano, ip.aclaraciones, ip.detalle, (ip.promo_id IS NOT NULL OR ip.detalle <> '')::bool AS es_promo,
       COALESCE((SELECT string_agg(e.nombre, ', ' ORDER BY e.id)
                 FROM item_pedido_extra e WHERE e.item_pedido_id = ip.id), '')::text AS extras
FROM item_pedido ip
WHERE ip.pedido_id = ANY(sqlc.arg(ids)::int[])
ORDER BY ip.pedido_id, ip.id;

-- name: CambiarEstadoPedido :one
-- Cambia el estado solo si sigue en el que se leyó (desde): si otro lo cambió en el medio, no
-- devuelve filas y no se pisa nada. La demora solo se toca si viene.
UPDATE pedido
SET estado = sqlc.arg(nuevo), demora_min = COALESCE(sqlc.narg(demora), demora_min)
WHERE id = sqlc.arg(id) AND estado = sqlc.arg(desde)
RETURNING *;

-- name: MarcarPedidoVisto :exec
UPDATE pedido SET visto_en = now() WHERE id = $1 AND visto_en IS NULL;

-- name: AvisoPedidos :one
-- El aviso de pedido nuevo: el pendiente sin ver más viejo y cuántos hay sin ver en total (la
-- ventana cuenta antes del LIMIT). Sin filas: no hay nada que avisar.
SELECT id, cliente, total, count(*) OVER () AS cantidad
FROM pedido
WHERE estado = 'pendiente' AND visto_en IS NULL
ORDER BY creado_en, id
LIMIT 1;

-- name: VentasPorDia :many
-- Ventas de un rango, por día (en la hora del local, la zona que se pasa) y por forma de pago,
-- sin los pedidos cancelados. De acá salen las métricas y el gráfico de Pedidos.
SELECT (creado_en AT TIME ZONE sqlc.arg(zona)::text)::date AS dia,
       pago,
       count(*) AS pedidos,
       sum(total)::text AS total
FROM pedido
WHERE creado_en >= sqlc.arg(desde) AND creado_en < sqlc.arg(hasta) AND estado <> 'cancelado'
GROUP BY 1, 2
ORDER BY 1, 2;

-- name: CountCanceladosRango :one
-- Para la tarjeta "Pedidos": cuántos se cancelaron o rechazaron en el período.
SELECT count(*) FROM pedido
WHERE creado_en >= sqlc.arg(desde) AND creado_en < sqlc.arg(hasta) AND estado = 'cancelado';

-- name: CountPendientesRango :one
-- Para el chip "Pendientes (2)": cuántos esperan confirmación en el período.
SELECT count(*) FROM pedido
WHERE creado_en >= sqlc.arg(desde) AND creado_en < sqlc.arg(hasta) AND estado = 'pendiente';
