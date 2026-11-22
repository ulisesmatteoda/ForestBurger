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

-- Agregados que el cliente le suma a una hamburguesa (cheddar, panceta, medallón...). disponible
-- en false lo saca del menú sin borrarlo: sirve para un día que no hay.
CREATE TABLE IF NOT EXISTS extra (
    id SERIAL PRIMARY KEY,
    nombre VARCHAR(100) NOT NULL,
    precio DECIMAL(10,2) NOT NULL CHECK (precio >= 0),
    disponible BOOLEAN NOT NULL DEFAULT true,
    creado_en TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Datos del local que se cargan en Ajustes. Una sola fila (id = 1): no hay "varios ajustes".
CREATE TABLE IF NOT EXISTS ajustes (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    local_abierto BOOLEAN NOT NULL DEFAULT true,       -- el switch de la barra lateral: si la web toma pedidos
    whatsapp VARCHAR(20) NOT NULL DEFAULT '',          -- solo números (con código de área)
    direccion_retiro VARCHAR(200) NOT NULL DEFAULT '',
    alias_cbu VARCHAR(60) NOT NULL DEFAULT '',
    titular VARCHAR(100) NOT NULL DEFAULT '',
    delivery_activo BOOLEAN NOT NULL DEFAULT true,
    costo_envio DECIMAL(10,2) NOT NULL DEFAULT 0 CHECK (costo_envio >= 0),
    pedido_minimo DECIMAL(10,2) CHECK (pedido_minimo >= 0)  -- NULL: sin mínimo
);
INSERT INTO ajustes (id) VALUES (1) ON CONFLICT DO NOTHING;

-- Horario de cada día (0 = lunes ... 6 = domingo). Las horas van como "HH:MM": si hasta es menor
-- que desde, cierra al día siguiente (de 20:00 a 01:00).
CREATE TABLE IF NOT EXISTS horario (
    dia SMALLINT PRIMARY KEY CHECK (dia BETWEEN 0 AND 6),
    abierto BOOLEAN NOT NULL DEFAULT true,
    desde VARCHAR(5) NOT NULL DEFAULT '20:00' CHECK (desde ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$'),
    hasta VARCHAR(5) NOT NULL DEFAULT '00:00' CHECK (hasta ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$')
);
INSERT INTO horario (dia) SELECT generate_series(0, 6) ON CONFLICT DO NOTHING;
