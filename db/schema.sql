CREATE TABLE producto (
    id SERIAL PRIMARY KEY,
    nombre VARCHAR(255) NOT NULL,
    categoria VARCHAR(50) NOT NULL,
    precio DECIMAL(10,2) NOT NULL,
    precio_doble DECIMAL(10,2), 
    descripcion TEXT,
    imagen_url TEXT
);

CREATE TABLE pedido (
    id SERIAL PRIMARY KEY,
    nombre_cliente VARCHAR(255) NOT NULL,
    tipo_entrega VARCHAR(20) NOT NULL
        CHECK (tipo_entrega IN ('delivery', 'take_away')),
    direccion TEXT,
    forma_pago VARCHAR(30) NOT NULL
        CHECK (forma_pago IN ('efectivo', 'transferencia')),
    notas TEXT,
    total DECIMAL(10,2) NOT NULL,
    estado VARCHAR(20) NOT NULL DEFAULT 'pendiente'
        CHECK (estado IN ('pendiente', 'confirmado', 'cancelado', 'entregado')),
    fecha TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE item_pedido (
    id SERIAL PRIMARY KEY,
    id_pedido INTEGER NOT NULL REFERENCES pedido(id) ON DELETE CASCADE,
    id_producto INTEGER NOT NULL REFERENCES producto(id),
    cantidad INTEGER NOT NULL,
    precio_unitario DECIMAL(10,2) NOT NULL,
    aclaraciones TEXT,
    tamano VARCHAR(10)
        CHECK (tamano IN ('simple', 'doble'))
);

CREATE TABLE extra (
    id SERIAL PRIMARY KEY,
    nombre VARCHAR(255) NOT NULL,
    precio DECIMAL(10,2) NOT NULL
);

CREATE TABLE item_pedido_extra (
    id SERIAL PRIMARY KEY,
    id_item_pedido INTEGER NOT NULL REFERENCES item_pedido(id) ON DELETE CASCADE,
    id_extra INTEGER NOT NULL REFERENCES extra(id),
    cantidad INTEGER NOT NULL DEFAULT 1,
    precio_unitario DECIMAL(10,2) NOT NULL
);