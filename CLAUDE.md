# ForestBurger

Trabajo práctico de **Programación Web (UNICEN)**. Es una app web para una hamburguesería ficticia llamada ForestBurger.

## Concepto

El cliente mira el menú en la web, arma su pedido (productos, extras y aclaraciones) y lo envía por **WhatsApp** al local con un link `wa.me`. El mensaje llega ya formateado, así no hay idas y vueltas por chat.

## Stack

- Go (servidor HTTP + API REST en `/api/productos`)
- PostgreSQL 16 en Docker Compose
- sqlc para generar el código Go a partir de las queries SQL (el código generado **se sube al repo**)
- Makefile para orquestar: `make test`, `make test-api` y `make run`

## Convenciones

- Cada entrega va en su propia branch (`tp1`, `tp2`, `tp3`, ...), según lo que pide la cátedra.
- Se prefieren soluciones **simples y acordes a lo visto en la cursada** antes que patrones de industria todavía no enseñados (por ejemplo, nada de healthchecks de Docker ni migraciones con herramientas externas).
- Las credenciales tienen que coincidir en `.env` / `docker-compose.yml`, en el `DB_URL` del Makefile y en el connection string de Go y de los tests.

## Modelo de datos

Hay cinco tablas. Por ahora en `db/schema.sql` **solo está implementada `producto`**, porque la consigna del TP de sqlc pedía el CRUD de una única entidad principal. Las demás se agregan en las próximas entregas.

```sql
CREATE TABLE producto (
    id SERIAL PRIMARY KEY,
    nombre VARCHAR(255) NOT NULL,
    categoria VARCHAR(50) NOT NULL,
    precio DECIMAL(10,2) NOT NULL,
    descripcion TEXT
);

CREATE TABLE pedido (
    id SERIAL PRIMARY KEY,
    nombre_cliente VARCHAR(255) NOT NULL,
    tipo_entrega VARCHAR(20) NOT NULL,     -- retiro / delivery
    direccion TEXT,                        -- solo si es delivery
    forma_pago VARCHAR(30) NOT NULL,
    notas TEXT,
    total DECIMAL(10,2) NOT NULL,
    fecha TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE item_pedido (
    id SERIAL PRIMARY KEY,
    id_pedido INTEGER NOT NULL REFERENCES pedido(id) ON DELETE CASCADE,
    id_producto INTEGER NOT NULL REFERENCES producto(id),
    cantidad INTEGER NOT NULL,
    precio_unitario DECIMAL(10,2) NOT NULL,
    aclaraciones TEXT
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
```

### Relaciones

| Relación | Cardinalidad |
|---|---|
| pedido → item_pedido | 1..1 — 1..N |
| producto → item_pedido | 1..1 — 0..N |
| item_pedido → item_pedido_extra | 1..1 — 0..N |
| extra → item_pedido_extra | 1..1 — 0..N |

`item_pedido_extra` es la tabla intermedia que resuelve la relación N a M entre `item_pedido` y `extra`.

### Decisiones de diseño

- **Por qué `producto` es la entidad principal del CRUD:** no depende de ninguna otra tabla y encaja bien con el create/read/update/delete, porque el dueño administra el catálogo. `pedido`, en cambio, es una entidad transaccional: no tiene sentido sin sus items y, una vez enviado, no se edita ni se borra.
- **Por qué existe `item_pedido`:** un pedido tiene varios productos, cada uno con su propia cantidad y sus aclaraciones.
- **Por qué existen `extra` e `item_pedido_extra`:** los agregados con precio fijo (cheddar, bacon, carne extra) tienen que sumar al total automáticamente y mostrarse como opciones con precio en el frontend. Se descartó guardarlos solo como texto libre porque el sistema no podría calcular el precio, y esta opción sirve más a largo plazo.
- **`aclaraciones` en `item_pedido` igual se mantiene:** cubre los pedidos sin precio ("sin cebolla", "bien cocida", "cortala al medio"). Los extras con precio van por la tabla `extra`.
- **`precio_unitario` copiado en `item_pedido` e `item_pedido_extra`:** guarda el precio del momento de la compra. Si después cambia el precio en `producto` o en `extra`, los pedidos viejos no se alteran.
- **`cantidad` en `item_pedido_extra`:** para casos como el doble cheddar (una fila con `cantidad = 2`). Si un item lleva varios extras distintos (cheddar + carne), se guarda una fila por extra.
- **No se guarda el subtotal:** se calcula siempre como `cantidad × precio_unitario`. El total del pedido es la suma de los items más la suma de sus extras.
- **`ON DELETE CASCADE`:** si se borra un pedido se borran sus items, y si se borra un item se borran sus extras.

### Cómo se graba un pedido completo

Son INSERTs en cadena, idealmente dentro de una transacción:

1. INSERT en `pedido` y obtener el `id` con `RETURNING id`.
2. INSERT de cada `item_pedido` con ese `id_pedido`, obteniendo el `id` de cada item.
3. INSERT de cada `item_pedido_extra` con el `id_item_pedido` que corresponde.

Cálculo del total:

```sql
SELECT
    (SELECT COALESCE(SUM(cantidad * precio_unitario), 0)
     FROM item_pedido WHERE id_pedido = $1)
  + (SELECT COALESCE(SUM(ipe.cantidad * ipe.precio_unitario), 0)
     FROM item_pedido_extra ipe
     JOIN item_pedido ip ON ip.id = ipe.id_item_pedido
     WHERE ip.id_pedido = $1) AS total;
```

## Problemas conocidos

- **Race condition al levantar Postgres:** en el primer arranque, la imagen levanta un servidor temporal que solo escucha por socket para correr `schema.sql`, después se reinicia y recién ahí acepta conexiones TCP. Si `pg_isready` chequea por socket, da OK antes de tiempo y Go falla con `connection reset by peer`. Por eso `wait-db` usa `pg_isready -h 127.0.0.1` para forzar el chequeo por TCP.
- `test-api.sh` corta con error si el servidor se muere antes de responder, en vez de quedarse esperando para siempre.
