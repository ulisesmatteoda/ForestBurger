# ForestBurger

Trabajo práctico de **Programación Web (UNICEN)**. Es una app web para una hamburguesería ficticia llamada ForestBurger.

## Concepto

El cliente mira el menú en la web, arma su pedido (productos, extras y aclaraciones) y lo confirma: entra al sistema como **pendiente** (con aviso y alarma en el panel) y el cliente lo sigue con el link `/pedido/{token}`. El carrito va en una cookie; los precios los pone siempre el servidor. El local lo atiende desde el **panel** (`/admin`): productos, promos, extras, pedidos y ajustes. El panel se puede instalar como app (PWA).

El diseño está en Figma: archivo `sU9jcobXIMVEt95QMQfrGI`, páginas Admin `15:324` (escritorio y frames "Celu ·") y Pantallas `0:1` (la web del cliente). Leelo **con la menor cantidad de llamadas posible**: una `get_metadata` de la página procesada localmente y las capturas justas.

## Stack

- Go (servidor HTTP, `net/http` con los patrones de Go 1.22) y una API REST en `/api/productos`.
- Vistas server-side con **templ** e interacción con **htmx**.
- PostgreSQL 16 en Docker Compose.
- sqlc para generar el código Go a partir de las queries SQL. Las queries están en `db/queries.sql` y en `db/queries/*.sql`.
- Makefile para orquestar:
  - `make test`: recrea la base de test (`forestburger_test`) y corre todo, sin tocar la de desarrollo.
  - `make run`: conserva la base de desarrollo y aplica `db/schema.sql`, que es idempotente.
  - `make run-app` y `make clean`: arrancan de cero (borran la base y `uploads/`).
  - `make test-api` y `make crear-admin`.

## Convenciones

- Cada entrega va en su propia branch, según lo que pide la cátedra.
- **El código generado no se sube:** `db/sqlc/*.go` y `views/*_templ.go` están en `.gitignore`. Se regeneran con `make generate` (sqlc + templ), y `build`, `test` y `run` lo hacen solos.
- Se prefieren soluciones **simples y acordes a lo visto en la cursada** antes que patrones de industria todavía no enseñados (por ejemplo, nada de healthchecks de Docker ni migraciones con herramientas externas).
- **Sin JavaScript propio.** Todo con htmx y CSS. La única excepción es `static/panel.js`, para lo que htmx no puede hacer, como copiar al portapapeles. La PWA va sin service worker.
- **El esquema es idempotente:**
  - las tablas van con `CREATE TABLE IF NOT EXISTS`;
  - las columnas nuevas, con `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`;
  - así `make run` no borra datos.
- **La plata va en centavos** (`pedidos.Centavos`). La base guarda `DECIMAL` y lo devuelve como texto ("17500.00"). Las cuentas siempre las hace el servidor con los precios de la base.
- **Con htmx:**
  - las respuestas de error van con 200 y `HX-Retarget`/`HX-Reswap`, porque htmx ignora los 4xx;
  - la sesión vencida responde `HX-Redirect`;
  - el filtro de la página se recupera con `HX-Current-URL`;
  - las actualizaciones fuera del target van con `hx-swap-oob`;
  - sin htmx, cada ruta devuelve la página completa (con el modal abierto si corresponde).
- **Tests:**
  - de handlers, con `setupAPI`/`setup`/`conSesion` en `handlers/`;
  - de base, con `internal/basetest.Abrir(t)`, que trunca las tablas.
  - Para lo que depende de htmx o del navegador, probar en un navegador de verdad (Chromium de Playwright).
  - Esas pruebas van con una copia del servidor en otro puerto, para no tocar el `make run` del usuario.
- Las credenciales tienen que coincidir en `.env` / `docker-compose.yml`, en el `DB_URL` del Makefile y en el connection string de Go y de los tests. **Nunca mostrar los valores de `.env`.**
- El usuario hace los commits y los push. En los commits no va ninguna referencia a Claude ni a Anthropic.

## Modelo de datos

La fuente de verdad es **`db/schema.sql`**. Resumen:

| Tabla | Para qué |
|---|---|
| `producto` | El catálogo. Tiene precio simple y doble (opcional), foto, y `disponible` (false = agotado) |
| `extra` | Los agregados con precio (cheddar, panceta…), con `disponible` |
| `promo` y `promo_producto` | Las promos: precio, cuántas elige el cliente, tamaño, días (0 = domingo, como `time.Weekday`), foto, activa, y los productos que entran (N a M) |
| `pedido` | Cliente, origen, entrega, pago, estado, cuentas (subtotal, descuento, envío, total), `token` de seguimiento y `visto_en` |
| `item_pedido` | Las líneas del pedido, con sus aclaraciones. Una promo armada por el cliente es una línea con `promo_id` y lo elegido en `detalle` |
| `item_pedido_extra` | Los extras de cada unidad de una línea |
| `pedido_evento` | El historial de estados |
| `admin` | El personal que entra al panel (bcrypt) |
| `ajustes` y `horario` | Los datos del local (fila única) y los horarios por día (0 = lunes) |

### Decisiones de diseño

- **`producto` fue la entidad principal del CRUD del TP** porque no depende de otra tabla y el dueño administra el catálogo. `pedido`, en cambio, es transaccional: una vez enviado no se edita ni se borra, solo cambia de estado.
- **Copias en `item_pedido` e `item_pedido_extra`:** guardan el nombre y el precio del momento. Así, si después cambia o se borra el producto, los pedidos viejos no se alteran: la referencia queda en `NULL` (`ON DELETE SET NULL`) y la copia se conserva.
- **Los extras van por cada unidad del ítem.** El doble cheddar es una fila con cantidad 2.
- **No se guarda el subtotal de cada línea:** se calcula. Las cuentas del pedido sí se guardan.
- **Los estados son** pendiente, confirmado, en_camino, entregado y cancelado. El cambio de estado es un `UPDATE ... WHERE estado = desde`, para que dos personas no pisen el cambio de la otra.
- **Un pedido se graba en una transacción** (`pedidos.Crear`): el pedido, sus ítems, sus extras y el primer evento. Las cuentas que se ven mientras se arma salen del mismo código (`pedidos.CalcularPresupuesto`).
- **Las fechas** se manejan con la zona de Argentina (`pedidos.Zona`).

## Problemas conocidos

- **Race condition al levantar Postgres:** en el primer arranque, la imagen levanta un servidor temporal que solo escucha por socket para correr `schema.sql`, después se reinicia y recién ahí acepta conexiones TCP. Si `pg_isready` chequea por socket, da OK antes de tiempo y Go falla con `connection reset by peer`. Por eso `wait-db` usa `pg_isready -h 127.0.0.1` para forzar el chequeo por TCP.
- `test-api.sh` corta con error si el servidor se muere antes de responder, en vez de quedarse esperando para siempre.
- **El audio de la alarma** no suena en una página recién cargada hasta que la persona toca algo, porque lo bloquea el navegador. Con el permiso de reproducción automática para el sitio, o con la PWA instalada, sí suena.
