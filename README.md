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
| `GET /login`                       | Formulario de ingreso (público). Con una sesión válida redirige a `/admin` |
| `POST /login`                      | Valida mail y contraseña; si están bien deja la cookie de sesión y redirige (303) |
| `POST /logout`                     | Borra la cookie de sesión y redirige (303) a `/login`                    |
| `GET /admin`                       | Página completa: lista de productos con el modal de alta cerrado. `?categoria=<valor>` filtra la lista (una categoría desconocida muestra todos); con htmx devuelve solo el panel |
| `GET /admin/productos/nuevo`       | Abre el modal de alta: con htmx devuelve solo el modal; sin htmx, la página con el modal abierto |
| `GET /admin/productos/nuevo/cerrar`| Responde vacío; htmx lo pone en `#modal` y el modal se cierra             |
| `POST /admin/productos`            | Crea un producto; devuelve el panel con todos los productos (el nuevo resaltado) y cierra el modal (OOB). Con error, vuelve al modal con el mensaje |
| `GET /admin/productos/{id}/editar` | Devuelve la fila del producto en modo edición, con sus datos cargados    |
| `PUT /admin/productos/{id}`        | Actualiza el producto; devuelve la fila en modo lectura (o en edición con el error) |
| `GET /admin/productos/{id}`        | Devuelve la fila en modo lectura (botón "Cancelar" de la edición)        |
| `GET /admin/productos/{id}/borrar` | Abre el modal que pide confirmar el borrado (`?categoria=` es el filtro activo) |
| `POST /admin/productos/foto-previa` | Valida la foto elegida y devuelve su vista previa (o el error). No guarda nada |
| `GET /uploads/<archivo>`           | Sirve las fotos de los productos (público: las va a usar el menú). No lista la carpeta |
| `DELETE /admin/productos/{id}`     | Borra el producto. Con htmx devuelve el panel con el mismo filtro (`?categoria=`) y cierra el modal (OOB); sin htmx, 303 a `/admin` |

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

### Fotos de los productos

Cada producto puede tener una foto (es opcional).

- **Cargarla:** el modal de alta tiene un campo "Foto". En la edición en línea de cada fila se
  puede elegir otra (reemplaza a la anterior) o marcar "Quitar foto". Por eso esos formularios
  viajan como `multipart/form-data` (`hx-encoding`).
- **Vista previa:** apenas se elige la foto (tocando la zona o arrastrándola encima), htmx la
  manda sola a `POST /admin/productos/foto-previa`, que la valida y devuelve la miniatura con el
  nombre y el peso, o el error en el momento (formato o tamaño). Esa ruta no guarda nada: la
  foto se guarda recién con el producto. Es sin JavaScript propio, solo atributos de htmx.
- **Qué se acepta:** JPG, PNG o WEBP, hasta 5 MB. El tipo se mira por el contenido del archivo,
  no por la extensión (que se puede falsear).
- **Dónde queda:** el archivo va a la carpeta `uploads/` con un nombre al azar, y la base guarda
  solo su ruta en `producto.imagen_url` (por ejemplo `/uploads/f2057e8bc36969de.png`); NULL si no
  tiene. La app la sirve en `/uploads/`.
- **Limpieza:** al cambiar o quitar la foto, y al borrar el producto, se borra el archivo viejo.
  `make clean` (y todo lo que arranca con la base vacía) vacía `uploads/`, porque sin la base
  esas fotos no las usa nadie. La carpeta está en `.gitignore`: las fotos no se suben al repo.
- **En la tabla:** cada fila muestra una miniatura; sin foto, un recuadro vacío.
- Si al guardar hay un error, el navegador no deja volver a completar el campo de archivo: el
  mensaje avisa "Volvé a elegir la foto.".

### Borrar un producto

El tacho de cada fila no usa el `confirm()` del navegador (que no se puede estilizar): abre un
modal chico en el mismo `<div id="modal">` que el alta, con el nombre del producto y los botones
CANCELAR y BORRAR. BORRAR hace el `DELETE` y el servidor devuelve el panel entero con el filtro
que estaba activo, así el contador y el mensaje de lista vacía quedan al día, y cierra el modal.
El filtro viaja en `?categoria=`; en las filas que devuelve la edición en línea, el servidor lo
saca de la URL de la página (header `HX-Current-URL` que manda htmx).

### Editar un producto

En cada fila de la tabla, el botón **Editar** convierte la fila en un formulario (nombre,
descripción, categoría, precio simple y precio doble). **Guardar** valida con las mismas
reglas que el alta; si hay un error se muestra en la misma fila y no se pierde lo escrito.
Dejar vacío el precio doble o la descripción los borra. **Cancelar** vuelve a la fila original.
Si el producto ya no existe (por ejemplo, lo borraron desde otra pestaña), la página se recarga.

## Admins

Solo el personal del local va a poder entrar a `/admin`. **No hay registro público**, a
propósito: si lo hubiera, cualquiera se podría crear una cuenta. Los admins se crean desde la
terminal y quedan en la tabla `admin`, con la contraseña hasheada con bcrypt (la contraseña en sí
nunca se guarda ni se muestra).

1. En el `.env`, poné en `ADMIN_EMAILS` los mails que pueden ser admin, separados por coma:
   ```
   ADMIN_EMAILS=ana@forestburger.com,luis@forestburger.com
   ```
2. Corré:
   ```bash
   make crear-admin
   ```
   Pide el email y la contraseña dos veces (no se ve al tipear). El email tiene que estar en
   `ADMIN_EMAILS`, y la contraseña tiene que tener al menos 8 caracteres.

- **Cambiar una contraseña olvidada:** volver a correr `make crear-admin` con el mismo email.
  No crea otro admin: le cambia la contraseña.
- **La base:** `make crear-admin` levanta Postgres si no está corriendo, pero **sin borrarlo**, y
  crea la tabla `admin` si todavía no existe. Si tenés `make run` en otra terminal, usa esa misma
  base. Ojo: `make run`, `make run-app` y `make test` arrancan con la base vacía, así que después
  de correrlos hay que volver a crear los admins (o usar el admin de desarrollo, abajo).
- **Admin de desarrollo:** para no correr `make crear-admin` después de cada `make run`, en el
  `.env` de tu máquina podés poner `DEV_ADMIN_EMAIL` y `DEV_ADMIN_PASSWORD`. Si están las dos, al
  arrancar la app crea (o actualiza) ese admin y avisa "Admin de desarrollo listo: <email>" (la
  contraseña nunca se muestra). El mail tiene que estar en `ADMIN_EMAILS` y la contraseña tener al
  menos 8 caracteres; si no, avisa el motivo y arranca igual. **Son solo para desarrollo: en
  producción no se ponen**, y sin ellas no hace nada.

## Login

Todo lo que está bajo `/admin` pide una sesión; `/`, `/login` y `/static/` son públicos.

- **Ingresar:** `/login` pide mail y contraseña. El mail tiene que estar en `ADMIN_EMAILS` y
  haberse creado con `make crear-admin`. Si algo no coincide, el error es siempre el mismo ("Mail o
  contraseña incorrectos."), para no revelar qué mails son de admins. Si se entró desde un link a
  una página del panel, después del login vuelve ahí; a cualquier otro destino no redirige.
- **La sesión** es una cookie `fb_sesion` firmada con HMAC-SHA256, sin tabla de sesiones: dice el
  mail y hasta cuándo vale (7 días), y la firma prueba que la armó el servidor. Es `HttpOnly` (el
  JavaScript de la página no la lee) y `SameSite=Lax`.
- **El middleware** `RequiereAdmin` envuelve todo `/admin`. Sin sesión válida, redirige al login
  (con htmx, manda `HX-Redirect: /login`). En cada pedido vuelve a mirar `ADMIN_EMAILS` y que el
  admin **siga existiendo en la base**: sacar un mail de la lista, o borrar el admin (en desarrollo
  pasa en cada `make run`), le corta el acceso aunque su cookie no haya vencido. Si la cookie es de
  un admin que ya no existe, además la borra del navegador; `/login` hace lo mismo y muestra el
  formulario en vez de mandar al panel.
- **Salir:** el botón "Salir" de la barra lateral borra la cookie.

Variables del `.env` (el Makefile se las pasa a la app):

| Variable         | Para qué |
|------------------|----------|
| `SESSION_SECRET` | Clave para firmar las cookies. **Obligatoria**, de al menos 32 caracteres; sin ella la app no arranca. Generala con `openssl rand -hex 32`. **Cambiarla cierra la sesión de todos.** |
| `COOKIE_SECURE`  | `true` en producción (HTTPS), así la cookie solo viaja cifrada. En localhost, `false`. |
| `ADMIN_EMAILS`   | Mails que pueden ser admin, separados por coma (ver "Admins"). |
| `DEV_ADMIN_EMAIL`, `DEV_ADMIN_PASSWORD` | **Solo desarrollo.** Admin que se crea solo al arrancar (ver "Admins"). |

## Notas

- La aplicación no utiliza archivos `.js` locales.
- `make test` ejecuta los paquetes de a uno (`go test -p 1`) porque los tests de `handlers`
  y de `db` comparten la misma base de datos.
- Asegurarse de tener `$(go env GOPATH)/bin` en la variable de entorno `PATH`
  para que `make` pueda invocar tanto `sqlc` como `templ`.
- Si los puertos `5432` (PostgreSQL) u `8080` (Servidor Web) están ocupados en
  la máquina, liberarlos antes de ejecutar `make run-app` o `make test`.