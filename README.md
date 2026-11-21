# ForestBurger

Aplicación web para una hamburguesería artesanal. Gestiona dos entidades
principales: los **productos** del menú (nombre, descripción, precio simple/doble y
categoría) y los **pedidos** de los clientes (items seleccionados, cantidad,
total, tipo de entrega, forma de pago y aclaraciones). Al finalizar un
pedido, la aplicación genera automáticamente un mensaje de WhatsApp con el
detalle y lo envía al local, para que la gestión de pedidos no dependa de ir
y venir con cada cliente.

Este repositorio (rama `tp6`) corresponde a la entrega del **TP6**, que
evoluciona el frontend SSR del TP5 (**Go + templ**) incorporando **HTMX**: los
productos se crean y se borran sin recargar la página (reemplazo de fragmentos
HTML y *Out-of-Band Swap* para limpiar el formulario), sin archivos JavaScript
propios, sobre la capa de persistencia generada con **sqlc** y PostgreSQL.

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

## Cómo ejecutar la aplicación (TP6)

1. Clonar el repositorio y pararse en la rama `tp6`:

   ```bash
   git clone [https://github.com/ulisesmatteoda/ForestBurger.git](https://github.com/ulisesmatteoda/ForestBurger.git)
   cd ForestBurger
   git checkout tp6
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

## Rutas de la aplicación

La interfaz se actualiza con HTMX: el servidor responde fragmentos de HTML (no JSON)
y HTMX los coloca en la página sin recargarla.

El panel del local vive bajo `/admin`, así más adelante se puede proteger todo junto con un
login. La raíz `/` por ahora redirige al panel; más adelante va a ser el menú para los clientes.

| Método y ruta                      | Qué hace                                                                 |
|------------------------------------|--------------------------------------------------------------------------|
| `GET /`                            | Redirige (303) al panel en `/admin`                                      |
| `GET /admin`                       | Página completa: lista de productos con el modal de alta cerrado. `?categoria=<valor>` filtra la lista (una categoría desconocida muestra todos); con htmx devuelve solo el panel |
| `GET /admin/productos/nuevo`       | Abre el modal de alta: con htmx devuelve solo el modal; sin htmx, la página con el modal abierto |
| `GET /admin/productos/nuevo/cerrar`| Responde vacío; htmx lo pone en `#modal` y el modal se cierra             |
| `POST /admin/productos`            | Crea un producto; devuelve el panel con todos los productos (el nuevo resaltado) y cierra el modal (OOB). Con error, vuelve al modal con el mensaje |
| `GET /admin/productos/{id}/editar` | Devuelve la fila del producto en modo edición, con sus datos cargados    |
| `PUT /admin/productos/{id}`        | Actualiza el producto; devuelve la fila en modo lectura (o en edición con el error) |
| `GET /admin/productos/{id}`        | Devuelve la fila en modo lectura (botón "Cancelar" de la edición)        |
| `DELETE /admin/productos/{id}`     | Borra el producto; responde 200 vacío y HTMX quita la fila              |

### Alta de productos (modal)

**+ NUEVO PRODUCTO** pide el modal y lo pone en `<div id="modal">`, al final de la página: el
modal se ve cuando ese div tiene algo adentro. Se cierra pidiendo `/admin/productos/nuevo/cerrar`,
que responde vacío, con la ✕, con **Cancelar**, con Esc o con un clic en el fondo. Todo con
atributos de htmx, sin JavaScript propio.

- **Guardar bien:** el servidor devuelve el panel con todos los productos (sin filtro, y la URL
  vuelve a `/admin` con `HX-Push-Url`), la fila nueva resaltada, y un `<div id="modal">` vacío
  por OOB que cierra el modal.
- **Guardar con error:** responde 200 con `HX-Retarget: #modal` y `HX-Reswap: innerHTML`. Así
  htmx pone el modal con el mensaje y lo que se escribió en vez de tocar la tabla.
- **Sin JavaScript:** el link y el formulario funcionan solos. Entrar a `/admin/productos/nuevo`
  muestra la página con el modal abierto; guardar redirige a `/admin` (303), o muestra la
  página con el modal y el error (400).

### Filtrar por categoría

Arriba de la lista hay chips (Todos, Hamburguesa, Veggie, Acompañamiento, Bebida) y un contador.
Cada chip es un link a `/admin?categoria=<valor>`: con htmx pide solo el panel y actualiza la URL
(`hx-push-url`), así el filtro sobrevive a F5 y se puede compartir; sin JS funciona como un link
común y devuelve la página completa.

### Editar un producto

En cada fila de la tabla, el botón **Editar** convierte la fila en un formulario (nombre,
descripción, categoría, precio simple y precio doble). **Guardar** valida con las mismas
reglas que el alta; si hay un error se muestra en la misma fila y no se pierde lo escrito.
Dejar vacío el precio doble o la descripción los borra. **Cancelar** vuelve a la fila original.
Si el producto ya no existe (por ejemplo, lo borraron desde otra pestaña), la página se recarga.

## Notas

- La aplicación no utiliza archivos `.js` locales.
- `make test` ejecuta los paquetes de a uno (`go test -p 1`) porque los tests de `handlers`
  y de `db` comparten la misma base de datos.
- Asegurarse de tener `$(go env GOPATH)/bin` en la variable de entorno `PATH`
  para que `make` pueda invocar tanto `sqlc` como `templ`.
- Si los puertos `5432` (PostgreSQL) u `8080` (Servidor Web) están ocupados en
  la máquina, liberarlos antes de ejecutar `make run-app` o `make test`.