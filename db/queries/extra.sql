-- name: CreateExtra :one
INSERT INTO extra (nombre, precio, disponible)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListExtras :many
SELECT * FROM extra ORDER BY nombre;

-- name: GetExtra :one
SELECT * FROM extra WHERE id = $1;

-- name: UpdateExtra :one
UPDATE extra SET nombre = $2, precio = $3, disponible = $4
WHERE id = $1
RETURNING *;

-- name: UpdateDisponibleExtra :one
-- El switch de la tabla: prende o apaga sin tocar lo demás.
UPDATE extra SET disponible = $2 WHERE id = $1
RETURNING *;

-- name: DeleteExtra :exec
DELETE FROM extra WHERE id = $1;
