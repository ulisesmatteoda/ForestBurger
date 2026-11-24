-- name: ListPromos :many
SELECT * FROM promo ORDER BY id;

-- name: GetPromo :one
SELECT * FROM promo WHERE id = $1;

-- name: ListProductosEnPromos :many
-- Los productos de todas las promos (para las tarjetas), con sus precios actuales.
SELECT pp.promo_id, p.id, p.nombre, p.precio, p.precio_doble
FROM promo_producto pp
JOIN producto p ON p.id = pp.producto_id
ORDER BY p.nombre;

-- name: ListProductosDePromo :many
SELECT producto_id FROM promo_producto WHERE promo_id = $1 ORDER BY producto_id;

-- name: CreatePromo :one
INSERT INTO promo (nombre, precio, cantidad, tamano, dias, imagen_url, activa)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: UpdatePromo :one
UPDATE promo
SET nombre = $2, precio = $3, cantidad = $4, tamano = $5, dias = $6
WHERE id = $1
RETURNING *;

-- name: UpdateImagenPromo :one
UPDATE promo SET imagen_url = $2 WHERE id = $1
RETURNING *;

-- name: UpdateActivaPromo :one
-- El switch de la tarjeta: activa (se ve en el menú) o pausada.
UPDATE promo SET activa = $2 WHERE id = $1
RETURNING *;

-- name: DeletePromo :exec
DELETE FROM promo WHERE id = $1;

-- name: DeleteProductosDePromo :exec
DELETE FROM promo_producto WHERE promo_id = $1;

-- name: AddProductoAPromo :exec
INSERT INTO promo_producto (promo_id, producto_id) VALUES ($1, $2);

-- name: ListPromosConProducto :many
-- Para avisar al borrar un producto: en qué promos activas está.
SELECT pr.nombre FROM promo pr
JOIN promo_producto pp ON pp.promo_id = pr.id
WHERE pp.producto_id = $1 AND pr.activa
ORDER BY pr.nombre;
