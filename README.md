# ForestBurger

Aplicación web para una hamburguesería artesanal. Gestiona dos entidades
principales: los **productos** del menú (nombre, descripción, precio simple/doble y
categoría) y los **pedidos** de los clientes (items seleccionados, cantidad,
total, tipo de entrega, forma de pago y aclaraciones). Al finalizar un
pedido, la aplicación genera automáticamente un mensaje de WhatsApp con el
detalle y lo envía al local, para que la gestión de pedidos no dependa de ir
y venir con cada cliente.

Este repositorio (rama `tp5`) corresponde a la entrega del **TP5**, que
implementa el frontend completo renderizado desde el servidor (SSR) utilizando
**Go + templ + HTMX** (sin archivos JavaScript propios), integrado con la API
y la capa de persistencia generada con **sqlc** sobre PostgreSQL.

## Tecnologías

- Go (`net/http` estándar con ruteo de Go 1.22+)
- [templ](https://templ.guide/) para la generación de vistas HTML tipadas en Go
- HTMX para interactividad dinámica en el cliente sin archivos `.js`
- [sqlc](https://sqlc.dev/) para generar código Go a partir de SQL
- PostgreSQL 16
- Docker y Docker Compose
- Paquete `testing` de Go y `curl` para pruebas de integración

## Requisitos previos

- Go 1.22+ instalado
- [sqlc](https://docs.sqlc.dev/en/latest/overview/install.html) instalado
- [templ](https://templ.guide/quick-start/installation) instalado (`go install github.com/a-h/templ/cmd/templ@latest`)
- Docker y Docker Compose instalados y funcionando (el usuario debe poder
  ejecutar `docker` sin `sudo`)
- `make` y `curl`

## Cómo ejecutar la aplicación (TP5)

1. Clonar el repositorio y pararse en la rama `tp5`:

   ```bash
   git clone [https://github.com/ulisesmatteoda/ForestBurger.git](https://github.com/ulisesmatteoda/ForestBurger.git)
   cd ForestBurger
   git checkout tp5
   ```

2. Ejecutar el flujo completo de la aplicación:

   ```bash
   make run-app
   ```

   Este comando realiza automáticamente todas las tareas requeridas por la cátedra:
   - **1) Tareas previas:**
     - Ejecuta `sqlc generate` y `templ generate`.
     - Compila el proyecto (`go build ./...`).
     - Crea el archivo `.env` a partir de `.env.example` (si no existe).
     - Elimina contenedores y volúmenes previos (`docker compose down -v`).
     - Levanta el contenedor de PostgreSQL (`docker compose up -d`) y espera
       activamente a que acepte conexiones (`pg_isready`).
   - **2) Ejecutar la aplicación:**
     - Inicia el servidor web en `http://localhost:8080`.
   - **3) Tareas posteriores:**
     - Al detener la aplicación con `Ctrl + C`, el `trap` configurado ejecuta
       automáticamente `make clean` (`docker compose down -v`), eliminando los
       contenedores y volúmenes creados.

3. Otros comandos disponibles en el `Makefile`:

   - **Ejecutar la suite de tests automatizados (`go test`):**
     ```bash
     make test
     ```
   - **Ejecutar las pruebas de endpoints con `curl`:**
     ```bash
     make test-api
     ```
   - **Limpiar contenedores y volúmenes manualmente:**
     ```bash
     make clean
     ```

## Notas

- La aplicación no utiliza archivos `.js` locales.
- Asegurarse de tener `$(go env GOPATH)/bin` en la variable de entorno `PATH`
  para que `make` pueda invocar tanto `sqlc` como `templ`.
- Si los puertos `5432` (PostgreSQL) u `8080` (Servidor Web) están ocupados en
  la máquina, liberarlos antes de ejecutar `make run-app` o `make test`.