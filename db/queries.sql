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