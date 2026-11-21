-- name: GuardarAdmin :one
-- Crea el admin, o le cambia la contraseña si el email ya existe (sirve para "me la olvidé").
INSERT INTO admin (email, password_hash)
VALUES ($1, $2)
ON CONFLICT (email) DO UPDATE SET password_hash = EXCLUDED.password_hash
RETURNING id, email, creado_en;

-- name: AdminPorEmail :one
SELECT * FROM admin WHERE email = $1;
