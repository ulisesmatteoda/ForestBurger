CREATE TABLE IF NOT EXISTS producto (
    id SERIAL PRIMARY KEY,
    nombre VARCHAR(255) NOT NULL,
    categoria VARCHAR(50) NOT NULL,
    precio DECIMAL(10,2) NOT NULL,
    precio_doble DECIMAL(10,2),   -- NULL si el producto no tiene versión doble
    descripcion TEXT,
    imagen_url TEXT               -- ruta de la foto ("/uploads/..."); NULL si no tiene. La imagen va a disco.
);

-- Para una base que ya estaba levantada antes de existir la columna (make crear-admin aplica este
-- schema sobre la base que haya): CREATE TABLE IF NOT EXISTS no la agrega, esto sí.
ALTER TABLE producto ADD COLUMN IF NOT EXISTS imagen_url TEXT;

-- Personal del local que entra a /admin. No hay registro público: se crean con make crear-admin.
-- password_hash es bcrypt; la contraseña nunca se guarda.
CREATE TABLE IF NOT EXISTS admin (
    id SERIAL PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    creado_en TIMESTAMPTZ NOT NULL DEFAULT now()
);
