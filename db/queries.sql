/* Producto */
-- name: CreateProducto :one
INSERT INTO producto (nombre, categoria, precio, descripcion)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetProducto :one
SELECT * FROM producto WHERE id = $1;

-- name: ListProductos :many
SELECT * FROM producto ORDER BY categoria, nombre;

-- name: UpdateProducto :one
UPDATE producto
SET nombre = $2, categoria = $3, precio = $4, descripcion = $5
WHERE id = $1
RETURNING *;

-- name: DeleteProducto :exec
DELETE FROM producto WHERE id = $1;

/* Pedido */

-- name: CreatePedido :one
INSERT INTO pedido (nombre_cliente, tipo_entrega, direccion, forma_pago, notas, total)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetPedido :one
SELECT * FROM pedido WHERE id = $1;

-- name: ListPedidos :many
SELECT * FROM pedido ORDER BY fecha DESC;

-- name: UpdateEstadoPedido :one
UPDATE pedido SET estado = $2 WHERE id = $1
RETURNING *;

-- name: UpdateTotalPedido :one
UPDATE pedido SET total = $2 WHERE id = $1
RETURNING *;

/* Extra*/

-- name: CreateExtra :one
INSERT INTO extra (nombre, precio)
VALUES ($1, $2)
RETURNING *;

-- name: GetExtra :one
SELECT * FROM extra WHERE id = $1;

-- name: ListExtras :many
SELECT * FROM extra ORDER BY nombre;

-- name: UpdateExtra :one
UPDATE extra
SET nombre = $2, precio = $3
WHERE id = $1
RETURNING *;   

-- name: DeleteExtra :exec
DELETE FROM extra WHERE id = $1;

/* ItemPedido e  */

-- name: CreateItemPedido :one
INSERT INTO item_pedido (id_pedido, id_producto, cantidad, precio_unitario, aclaraciones)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListItemsByPedido :many
SELECT ip.id, ip.id_producto, p.nombre AS nombre_producto,
       ip.cantidad, ip.precio_unitario, ip.aclaraciones
FROM item_pedido ip
JOIN producto p ON p.id = ip.id_producto
WHERE ip.id_pedido = $1
ORDER BY ip.id;


-- name: DeleteItemPedido :exec
DELETE FROM item_pedido WHERE id = $1;


/*ItemPedidoExtra*/

-- name: CreateItemPedidoExtra :one
INSERT INTO item_pedido_extra (id_item_pedido, id_extra, cantidad, precio_unitario)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListExtrasByItem :many
SELECT ipe.id_extra, e.nombre AS nombre_extra,
       ipe.cantidad, ipe.precio_unitario
FROM item_pedido_extra ipe
JOIN extra e ON e.id = ipe.id_extra
WHERE ipe.id_item_pedido = $1
ORDER BY ipe.id;

-- name: DeleteItemPedidoExtra :exec
DELETE FROM item_pedido_extra WHERE id = $1;