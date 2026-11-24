-- name: CreateProducto :one
INSERT INTO producto (nombre, categoria, precio, precio_doble, descripcion, imagen_url)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetProducto :one
SELECT * FROM producto WHERE id = $1;

-- name: ListProductos :many
SELECT * FROM producto ORDER BY categoria, nombre;

-- name: UpdateProducto :one
UPDATE producto
SET nombre = $2, categoria = $3, precio = $4, precio_doble = $5, descripcion = $6
WHERE id = $1
RETURNING *;

-- name: UpdateImagenProducto :one
-- Cambia o saca (NULL) la foto. Va aparte de UpdateProducto: la foto no viaja en cada edición.
UPDATE producto SET imagen_url = $2 WHERE id = $1
RETURNING *;

-- name: UpdateDisponibleProducto :one
-- El switch de agotado de la lista: prende o apaga sin tocar lo demás.
UPDATE producto SET disponible = $2 WHERE id = $1
RETURNING *;

-- name: DeleteProducto :exec
DELETE FROM producto WHERE id = $1;
-- name: ListProductosPorCategoria :many
SELECT * FROM producto WHERE categoria = $1 ORDER BY nombre;
