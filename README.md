# ForestBurger

Aplicación web para una hamburguesería artesanal. Gestiona dos entidades
principales: los **productos** del menú (nombre, descripción, precio y
categoría) y los **pedidos** de los clientes (items seleccionados, cantidad,
total, tipo de entrega, forma de pago y aclaraciones). Al finalizar un
pedido, la aplicación genera automáticamente un mensaje de WhatsApp con el
detalle y lo envía al local, para que la gestión de pedidos no dependa de ir
y venir con cada cliente.

Este repositorio (rama `tp3`) corresponde a la entrega del **TP3**, que suma
una **API REST** en Go (`net/http`) conectada a la capa de persistencia
generada con **sqlc** sobre PostgreSQL, junto con pruebas automatizadas de
endpoints mediante `curl`.

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

1. Clonar el repositorio y pararse en la rama `tp3`:

   ```bash
   git clone https://github.com/ulisesmatteoda/ForestBurger.git
   cd ForestBurger
   git checkout tp3
   ```

2. Ejecutar el pipeline completo de pruebas de la API (TP3):

   ```bash
   make test-api
   ```

   Este comando se encarga automáticamente de todo el flujo requerido:
   - **Tareas previas:** ejecuta `sqlc generate`, compila el proyecto, elimina
     contenedores y volúmenes previos, levanta PostgreSQL en Docker y espera a
     que acepte conexiones.
   - **Pruebas de la API:** levanta la aplicación en `:8080` y ejecuta el script
     `test-api.sh`, que prueba cada uno de los endpoints con `curl`.
   - **Tareas posteriores:** detiene el servidor y elimina los contenedores y
     volúmenes creados (`docker compose down -v`).

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

| Método | Ruta | Descripción | Respuestas |
| :--- | :--- | :--- | :--- |
| `POST` | `/api/productos` | Crea un nuevo producto | `201 Created`, `400 Bad Request` |
| `GET` | `/api/productos` | Lista todos los productos | `200 OK` |
| `GET` | `/api/productos/{id}` | Obtiene un producto por ID | `200 OK`, `400 Bad Request`, `404 Not Found` |
| `PUT` | `/api/productos/{id}` | Actualiza un producto por ID | `200 OK`, `400 Bad Request`, `404 Not Found` |
| `DELETE` | `/api/productos/{id}` | Elimina un producto por ID | `204 No Content`, `400 Bad Request`, `404 Not Found` |

### Ejemplo de JSON (`POST` y `PUT`)

```json
{
  "nombre": "Forest Doble",
  "categoria": "hamburguesa",
  "precio": 9500.50,
  "descripcion": "Doble carne y cheddar"
}
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
├── static/
│   └── index.html
├── main.go                            # punto de entrada y rutas de la API
├── handlers.go                        # handlers HTTP, DTOs y validaciones
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