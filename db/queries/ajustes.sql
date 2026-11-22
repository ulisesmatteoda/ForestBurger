-- name: GetAjustes :one
SELECT * FROM ajustes WHERE id = 1;

-- name: UpdateAjustes :exec
-- Todo lo de la pantalla Ajustes menos "local abierto", que tiene su propio switch.
UPDATE ajustes
SET whatsapp = $1, direccion_retiro = $2, alias_cbu = $3, titular = $4,
    delivery_activo = $5, costo_envio = $6, pedido_minimo = $7
WHERE id = 1;

-- name: SetLocalAbierto :one
UPDATE ajustes SET local_abierto = $1 WHERE id = 1
RETURNING local_abierto;

-- name: ListHorarios :many
SELECT * FROM horario ORDER BY dia;

-- name: UpdateHorario :exec
UPDATE horario SET abierto = $2, desde = $3, hasta = $4 WHERE dia = $1;
