# ForestBurger

Aplicación web para una hamburguesería artesanal. Gestiona dos entidades
principales: los **productos** del menú (nombre, descripción, precio y
categoría) y los **pedidos** de los clientes (items seleccionados, cantidad,
extras, total, tipo de entrega, forma de pago y aclaraciones). Al finalizar
un pedido, la aplicación genera automáticamente un mensaje de WhatsApp con el
detalle y lo envía al local, para que la gestión de pedidos no dependa de ir
y venir con cada cliente.

Este repositorio (rama `tp3-extendido`) parte de la entrega del **TP3**
(API REST en Go conectada a PostgreSQL mediante **sqlc**, con pruebas de
endpoints usando `curl`) y la extiende con el resto del modelo de datos:
**extras** y **pedidos**.

## Qué agrega `tp3-extendido` respecto de `tp3`

En `tp3` la API solo manejaba el CRUD de `producto`. En esta rama se suma
todo lo necesario para registrar un pedido completo.

### Modelo de datos

Se agregaron cuatro tablas en `db/schema.sql`:

| Tabla | Para qué sirve |
| :--- | :--- |
| `extra` | Agregados con precio fijo (cheddar, bacon, carne extra) que el local administra igual que los productos. |
| `pedido` | Datos del pedido: cliente, tipo de entrega (`delivery` / `take_away`), dirección, forma de pago (`efectivo` / `transferencia`), notas, total, estado y fecha. |
| `item_pedido` | Cada producto del pedido, con su cantidad, sus aclaraciones sin precio ("sin cebolla") y el precio unitario al momento de la compra. |
| `item_pedido_extra` | Tabla intermedia entre `item_pedido` y `extra`: qué extras lleva cada item y cuántos (por ejemplo, doble cheddar). |

Algunas decisiones de diseño:

- **El precio se copia en el pedido** (`precio_unitario`): si después cambia el
  precio de un producto o de un extra, los pedidos viejos no se alteran.
- **El servidor calcula el precio y el total:** el cliente solo manda IDs y
  cantidades; los precios se leen de la base, así no se pueden falsear desde
  la API.
- **Estados del pedido:** `pendiente` (por defecto), `confirmado`,
  `cancelado` y `entregado`, controlados con un `CHECK` en la base.
- **`ON DELETE CASCADE`:** si se borra un pedido se borran sus items, y con
  ellos sus extras.

### API

- CRUD completo de **extras** en `/api/extras`.
- **Pedidos** en `/api/pedidos`: crear un pedido con sus items y extras,
  listarlos, ver el detalle de uno y cambiar su estado. Un pedido no se edita
  ni se borra una vez creado; solo cambia de estado.
- Validaciones de pedido: nombre obligatorio, tipo de entrega y forma de pago
  válidos, dirección obligatoria si es delivery, al menos un producto en el
  carrito y cantidades mayores a cero.

### Organización del código

El archivo `handlers.go` de la raíz pasó a ser el paquete `handlers/`, con un
archivo por entidad:

- `handlers/handlers.go`: el tipo `API`, el constructor `NewAPI` y los helpers
  compartidos (respuestas JSON, conversión de precios y de campos opcionales).
- `handlers/producto.go`, `handlers/extra.go` y `handlers/pedido.go`: DTOs,
  validaciones y handlers de cada entidad.

`main.go` crea la API con `handlers.NewAPI(queries)` y registra todas las rutas.

### Pruebas

`test-api.sh` pasó de 6 a 14 pruebas: además del CRUD de productos, ahora
prueba los extras y el ciclo completo de un pedido (ver más abajo).

## Tecnologías

- Go (`net/http` estándar con ruteo de Go 1.22+)
- [sqlc](https://sqlc.dev/) para generar código Go a partir de SQL
- PostgreSQL 16
- Docker y Docker Compose
- `curl` y Bash (`test-api.sh`) para las pruebas de integración de la API
- Paquete `testing` de Go para los tests de persistencia

## Requisitos previos

- Go 1.22+ instalado
- [sqlc](https://docs.sqlc.dev/en/latest/overview/install.html) instalado
- Docker y Docker Compose instalados y funcionando (el usuario debe poder
  ejecutar `docker` sin `sudo`)
- `make` y `curl`

## Cómo ejecutar

1. Clonar el repositorio y pararse en la rama `tp3-extendido`:

   ```bash
   git clone https://github.com/ulisesmatteoda/ForestBurger.git
   cd ForestBurger
   git checkout tp3-extendido
   ```

2. Correr las pruebas de la API:

   ```bash
   make test-api
   ```

   Este comando se encarga de todo el flujo:
   - **Tareas previas:** ejecuta `sqlc generate`, compila el proyecto, elimina
     contenedores y volúmenes previos, levanta PostgreSQL en Docker y espera a
     que acepte conexiones.
   - **Pruebas de la API:** levanta la aplicación en `:8080` y ejecuta
     `test-api.sh`, que prueba los endpoints con `curl`.
   - **Tareas posteriores:** detiene el servidor y elimina los contenedores y
     volúmenes creados (`docker compose down -v`), aunque alguna prueba falle.

   Las pruebas que corre `test-api.sh` son:

   | # | Prueba | Resultado esperado |
   | :--- | :--- | :--- |
   | 1 | `POST /api/productos` | `201` y devuelve el ID |
   | 2 | `GET /api/productos` | `200` |
   | 3 | `GET /api/productos/{id}` | `200` |
   | 4 | `PUT /api/productos/{id}` | `200` |
   | 5 | `DELETE /api/productos/{id}` | `204` |
   | 6 | `GET /api/productos/{id}` del producto borrado | `404` |
   | 7 | `POST /api/extras` (Cheddar y Bacon) | `201` |
   | 8 | `GET /api/extras` | `200` |
   | 9 | `POST /api/productos` (productos para el pedido) | `201` |
   | 10 | `POST /api/pedidos` con productos y extras | `201`, total `24100` y estado `pendiente` |
   | 11 | `GET /api/pedidos` | `200` |
   | 12 | `GET /api/pedidos/{id}` | `200` con los nombres de productos y extras |
   | 13 | `PUT /api/pedidos/{id}/estado` | `200` y estado `confirmado` |
   | 14 | `POST /api/pedidos` con el carrito vacío | `400` |

   Si todo sale bien, el script termina con
   `TODOS LOS TESTS DE LA API PASARON CON EXITO`. Si una prueba falla, muestra
   el status esperado y el recibido, y `make test-api` termina con error.

3. Otros comandos útiles:

   - Ejecutar los tests de la capa de base de datos (TP2):
     ```bash
     make test
     ```
   - Levantar el servidor de forma interactiva en `http://localhost:8080`:
     ```bash
     make run
     ```

## Endpoints de la API

Todos los endpoints reciben y devuelven JSON (`application/json`).

### Productos

| Método | Ruta | Descripción | Respuestas |
| :--- | :--- | :--- | :--- |
| `POST` | `/api/productos` | Crea un nuevo producto | `201 Created`, `400 Bad Request` |
| `GET` | `/api/productos` | Lista todos los productos | `200 OK` |
| `GET` | `/api/productos/{id}` | Obtiene un producto por ID | `200 OK`, `400 Bad Request`, `404 Not Found` |
| `PUT` | `/api/productos/{id}` | Actualiza un producto por ID | `200 OK`, `400 Bad Request`, `404 Not Found` |
| `DELETE` | `/api/productos/{id}` | Elimina un producto por ID | `204 No Content`, `400 Bad Request`, `404 Not Found` |

```json
{
  "nombre": "Forest Doble",
  "categoria": "hamburguesa",
  "precio": 9500.50,
  "descripcion": "Doble carne y cheddar"
}
```

### Extras (nuevo)

| Método | Ruta | Descripción | Respuestas |
| :--- | :--- | :--- | :--- |
| `POST` | `/api/extras` | Crea un nuevo extra | `201 Created`, `400 Bad Request` |
| `GET` | `/api/extras` | Lista todos los extras | `200 OK` |
| `GET` | `/api/extras/{id}` | Obtiene un extra por ID | `200 OK`, `400 Bad Request`, `404 Not Found` |
| `PUT` | `/api/extras/{id}` | Actualiza un extra por ID | `200 OK`, `400 Bad Request`, `404 Not Found` |
| `DELETE` | `/api/extras/{id}` | Elimina un extra por ID | `204 No Content`, `400 Bad Request`, `404 Not Found` |

```json
{
  "nombre": "Cheddar",
  "precio": 500
}
```

### Pedidos (nuevo)

| Método | Ruta | Descripción | Respuestas |
| :--- | :--- | :--- | :--- |
| `POST` | `/api/pedidos` | Crea un pedido con sus items y extras y calcula el total | `201 Created`, `400 Bad Request` |
| `GET` | `/api/pedidos` | Lista los pedidos, del más nuevo al más viejo | `200 OK` |
| `GET` | `/api/pedidos/{id}` | Obtiene un pedido con sus items y extras | `200 OK`, `400 Bad Request`, `404 Not Found` |
| `PUT` | `/api/pedidos/{id}/estado` | Cambia el estado de un pedido | `200 OK`, `400 Bad Request`, `404 Not Found` |

Ejemplo para crear un pedido (los precios no se mandan: los pone el servidor):

```json
{
  "nombre_cliente": "Ulises",
  "tipo_entrega": "delivery",
  "direccion": "Av. Colón 1234",
  "forma_pago": "efectivo",
  "notas": "Tocar timbre 2B",
  "items": [
    {
      "id_producto": 1,
      "cantidad": 2,
      "aclaraciones": "sin cebolla",
      "extras": [{ "id_extra": 1, "cantidad": 1 }]
    }
  ]
}
```

Ejemplo para cambiar el estado:

```json
{ "estado": "confirmado" }
```

## Estructura del proyecto

```text
ForestBurger/
├── db/
│   ├── sqlc/
│   │   ├── db.go                      # generado por sqlc
│   │   ├── models.go                  # generado por sqlc
│   │   └── queries.sql.go             # generado por sqlc
│   ├── product_repository_test.go     # tests de integración de BD
│   ├── schema.sql                     # definición de tablas
│   └── queries.sql                    # queries fuente para sqlc
├── handlers/
│   ├── handlers.go                    # tipo API, NewAPI y helpers compartidos
│   ├── producto.go                    # endpoints de productos
│   ├── extra.go                       # endpoints de extras
│   └── pedido.go                      # endpoints de pedidos
├── static/
│   └── index.html
├── main.go                            # punto de entrada y rutas de la API
├── test-api.sh                        # script de pruebas con curl
├── docker-compose.yml
├── sqlc.yaml
├── Makefile
├── .env.example
└── README.md
```

## Notas

- Si los puertos `5432` (PostgreSQL) u `8080` (API) están ocupados en tu
  máquina, liberalos antes de correr `make test-api` o `make test`.
