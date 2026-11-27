-- name: GuardarSuscripcion :exec
-- Activar en un dispositivo: si el endpoint ya estaba (otra vez "Activar"), se actualiza.
INSERT INTO push_suscripcion (admin_email, endpoint, p256dh, auth)
VALUES ($1, $2, $3, $4)
ON CONFLICT (endpoint) DO UPDATE SET admin_email = EXCLUDED.admin_email, p256dh = EXCLUDED.p256dh, auth = EXCLUDED.auth;

-- name: ListSuscripciones :many
SELECT * FROM push_suscripcion ORDER BY id;

-- name: ListSuscripcionesDe :many
SELECT * FROM push_suscripcion WHERE admin_email = $1 ORDER BY id;

-- name: BorrarSuscripcion :exec
DELETE FROM push_suscripcion WHERE endpoint = $1;
