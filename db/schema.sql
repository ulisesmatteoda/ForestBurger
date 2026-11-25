CREATE TABLE IF NOT EXISTS producto (
    id SERIAL PRIMARY KEY,
    nombre VARCHAR(255) NOT NULL,
    categoria VARCHAR(50) NOT NULL,
    precio DECIMAL(10,2) NOT NULL,
    precio_doble DECIMAL(10,2),   -- NULL si el producto no tiene versión doble
    descripcion TEXT,
    imagen_url TEXT,              -- ruta de la foto ("/uploads/..."); NULL si no tiene. La imagen va a disco.
    disponible BOOLEAN NOT NULL DEFAULT true   -- false: agotado (no se puede pedir)
);

-- Para una base que ya estaba levantada antes de existir la columna (make crear-admin aplica este
-- schema sobre la base que haya): CREATE TABLE IF NOT EXISTS no la agrega, esto sí.
ALTER TABLE producto ADD COLUMN IF NOT EXISTS imagen_url TEXT;
ALTER TABLE producto ADD COLUMN IF NOT EXISTS disponible BOOLEAN NOT NULL DEFAULT true;

-- Las promos: el cliente elige `cantidad` hamburguesas entre los productos de promo_producto, del
-- tamaño que diga la promo ('cliente': elige él, simple o doble), por un precio fijo. dias son los
-- días de la semana en que está (0 = domingo … 6 = sábado, como time.Weekday de Go; ojo, horario.dia
-- arranca en lunes).
CREATE TABLE IF NOT EXISTS promo (
    id SERIAL PRIMARY KEY,
    nombre VARCHAR(100) NOT NULL,
    precio DECIMAL(10,2) NOT NULL CHECK (precio > 0),
    cantidad SMALLINT NOT NULL CHECK (cantidad BETWEEN 1 AND 10),
    tamano VARCHAR(10) NOT NULL CHECK (tamano IN ('simple', 'doble', 'cliente')),
    dias INTEGER[] NOT NULL,
    imagen_url TEXT,
    activa BOOLEAN NOT NULL DEFAULT true
);

-- Los productos que entran en cada promo (N a M). Si se borra el producto, sale de la promo.
CREATE TABLE IF NOT EXISTS promo_producto (
    promo_id INTEGER NOT NULL REFERENCES promo(id) ON DELETE CASCADE,
    producto_id INTEGER NOT NULL REFERENCES producto(id) ON DELETE CASCADE,
    PRIMARY KEY (promo_id, producto_id)
);

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

-- ---------- Pedidos ----------
-- Cada pedido guarda una copia del nombre y del precio de lo que se pidió: si después cambia o se
-- borra un producto o un extra, el pedido (en curso o viejo) se sigue viendo igual. Por eso las
-- referencias a producto y extra quedan en NULL al borrarlos (ON DELETE SET NULL).
CREATE TABLE IF NOT EXISTS pedido (
    id SERIAL PRIMARY KEY,                        -- el N° del pedido
    creado_en TIMESTAMPTZ NOT NULL DEFAULT now(),
    cliente VARCHAR(100) NOT NULL,
    telefono VARCHAR(20) NOT NULL DEFAULT '',
    origen VARCHAR(20) NOT NULL CHECK (origen IN ('web', 'whatsapp', 'telefono', 'local')),
    entrega VARCHAR(20) NOT NULL CHECK (entrega IN ('retiro', 'delivery')),
    direccion VARCHAR(200) NOT NULL DEFAULT '',
    pago VARCHAR(20) NOT NULL CHECK (pago IN ('efectivo', 'transferencia')),
    notas TEXT NOT NULL DEFAULT '',
    estado VARCHAR(20) NOT NULL DEFAULT 'pendiente',   -- valores posibles: ver pedido_estado_check
    demora_min SMALLINT CHECK (demora_min > 0),   -- la que se le avisa al cliente; NULL hasta confirmar
    subtotal DECIMAL(10,2) NOT NULL CHECK (subtotal >= 0),
    descuento_pct SMALLINT NOT NULL DEFAULT 0 CHECK (descuento_pct BETWEEN 0 AND 100),
    descuento_motivo VARCHAR(60) NOT NULL DEFAULT '',
    descuento DECIMAL(10,2) NOT NULL DEFAULT 0 CHECK (descuento >= 0),
    costo_envio DECIMAL(10,2) NOT NULL DEFAULT 0 CHECK (costo_envio >= 0),
    total DECIMAL(10,2) NOT NULL CHECK (total >= 0),
    token VARCHAR(16) NOT NULL UNIQUE,            -- el código del link de seguimiento
    visto_en TIMESTAMPTZ,                         -- NULL hasta que alguien lo abre en el panel
    CHECK (entrega = 'retiro' OR direccion <> '')
);
CREATE INDEX IF NOT EXISTS pedido_creado_en ON pedido (creado_en);
-- Los estados van en un CHECK con nombre, aparte, para poder cambiarlos en una base que ya existe
-- (CREATE TABLE IF NOT EXISTS no toca una tabla que ya está). en_camino: un delivery que ya salió;
-- listo: un retiro que ya se puede pasar a buscar.
ALTER TABLE pedido DROP CONSTRAINT IF EXISTS pedido_estado_check;
ALTER TABLE pedido ADD CONSTRAINT pedido_estado_check
    CHECK (estado IN ('pendiente', 'confirmado', 'en_camino', 'listo', 'entregado', 'cancelado'));

-- Lo que se pidió. Los extras van por cada unidad: "2x Classic + cheddar" son dos Classic, cada
-- una con cheddar. Una línea cuesta cantidad × (precio_unitario + extras).
CREATE TABLE IF NOT EXISTS item_pedido (
    id SERIAL PRIMARY KEY,
    pedido_id INTEGER NOT NULL REFERENCES pedido(id) ON DELETE CASCADE,
    producto_id INTEGER REFERENCES producto(id) ON DELETE SET NULL,
    nombre VARCHAR(255) NOT NULL,
    tamano VARCHAR(10) NOT NULL CHECK (tamano IN ('simple', 'doble')),
    cantidad SMALLINT NOT NULL CHECK (cantidad > 0),
    precio_unitario DECIMAL(10,2) NOT NULL CHECK (precio_unitario >= 0),
    aclaraciones TEXT NOT NULL DEFAULT ''   -- lo que pide el cliente para ese ítem ("sin cebolla")
);
ALTER TABLE item_pedido ADD COLUMN IF NOT EXISTS aclaraciones TEXT NOT NULL DEFAULT '';
-- Una promo del cliente ("Armá tu promo") es un ítem: su precio fijo, y en detalle lo que eligió
-- ("Classic (simple) + Cheddar · Big Forest (simple)"). Los extras van en item_pedido_extra.
ALTER TABLE item_pedido ADD COLUMN IF NOT EXISTS promo_id INTEGER REFERENCES promo(id) ON DELETE SET NULL;
ALTER TABLE item_pedido ADD COLUMN IF NOT EXISTS detalle TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS item_pedido_extra (
    id SERIAL PRIMARY KEY,
    item_pedido_id INTEGER NOT NULL REFERENCES item_pedido(id) ON DELETE CASCADE,
    extra_id INTEGER REFERENCES extra(id) ON DELETE SET NULL,
    nombre VARCHAR(100) NOT NULL,
    cantidad SMALLINT NOT NULL DEFAULT 1 CHECK (cantidad > 0),   -- por cada unidad del ítem (doble cheddar = 2)
    precio_unitario DECIMAL(10,2) NOT NULL CHECK (precio_unitario >= 0)
);

-- Historial de cada pedido: "21:05 Recibido", "21:07 Confirmado · 30 min".
CREATE TABLE IF NOT EXISTS pedido_evento (
    id SERIAL PRIMARY KEY,
    pedido_id INTEGER NOT NULL REFERENCES pedido(id) ON DELETE CASCADE,
    creado_en TIMESTAMPTZ NOT NULL DEFAULT now(),
    estado VARCHAR(20) NOT NULL,                 -- recibido, confirmado, entregado, cancelado
    detalle VARCHAR(100) NOT NULL DEFAULT ''
);
