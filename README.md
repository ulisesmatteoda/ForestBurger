# ForestBurger

Aplicación web para una hamburguesería artesanal. Gestiona dos entidades
principales: los **productos** del menú (nombre, descripción, precio simple/doble y
categoría) y los **pedidos** de los clientes (items seleccionados, cantidad,
total, tipo de entrega, forma de pago y aclaraciones). El cliente arma el
pedido en la web y lo confirma: entra directo al panel del local (con un aviso
y una alarma), y el cliente lo sigue en vivo con un link, sin ir y venir por
chat.

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

   - **Desarrollar con la base que guarda los datos:**
     ```bash
     make run
     ```
     A diferencia de `make run-app`, **no borra la base**: la levanta si no está, le aplica el
     schema (es seguro: todo es `IF NOT EXISTS`, así aparecen las tablas nuevas sin perder nada)
     y arranca. Los productos, extras, ajustes y pedidos quedan entre corridas.
   - **Ejecutar la suite de tests automatizados (`go test`):**
     ```bash
     make test
     ```
     Usa su propia base, `forestburger_test` (en el mismo contenedor), que se recrea en cada
     corrida: **nunca toca los datos de desarrollo**.
   - **Ejecutar las pruebas de endpoints con `curl`:**
     ```bash
     make test-api
     ```
   - **Borrar todo y empezar de cero** (la base, los datos y las fotos):
     ```bash
     make clean
     ```

   | Comando | La base de desarrollo |
   |---|---|
   | `make run` | la conserva |
   | `make run-app` | la borra al empezar y al terminar (el flujo del TP) |
   | `make test` | no la toca (usa `forestburger_test`) |
   | `make crear-admin` | la conserva |
   | `make clean` | la borra, con las fotos |

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
| `GET /admin/modal/cerrar`          | Responde vacío; htmx lo pone en `#modal` y se cierra cualquier modal del panel |
| `POST /admin/productos`            | Crea un producto; devuelve el panel con todos los productos (el nuevo resaltado) y cierra el modal (OOB). Con error, vuelve al modal con el mensaje |
| `GET /admin/productos/{id}/editar` | Devuelve la fila del producto en modo edición, con sus datos cargados    |
| `PUT /admin/productos/{id}`        | Actualiza el producto; devuelve la fila en modo lectura (o en edición con el error) |
| `PUT /admin/productos/{id}/disponible` | El switch de agotado de la lista; devuelve la fila              |
| `GET /admin/promos`                | Sección Promos: las tarjetas                                             |
| `GET /admin/promos/nueva`          | Modal "Nueva promo" (sin htmx, la página con el modal abierto)           |
| `GET /admin/promos/{id}/editar`    | El mismo modal, con la promo guardada                                     |
| `POST /admin/promos/vista`         | Recalcula el modal mientras se arma (vista previa y cuentas). No guarda nada |
| `POST /admin/promos`               | Guarda una promo nueva (multipart, con la foto opcional); devuelve las tarjetas |
| `POST /admin/promos/{id}`          | Guarda la edición                                                         |
| `PUT /admin/promos/{id}/activa`    | El switch Activa / Pausada de la tarjeta; devuelve la tarjeta             |
| `GET /admin/promos/{id}/borrar`    | Confirmación de borrado                                                   |
| `DELETE /admin/promos/{id}`        | Borra la promo y su foto; devuelve las tarjetas y cierra el modal         |
| `GET /admin/productos/{id}`        | Devuelve la fila en modo lectura (botón "Cancelar" de la edición)        |
| `GET /admin/productos/{id}/borrar` | Abre el modal que pide confirmar el borrado (`?categoria=` es el filtro activo) |
| `GET /admin/extras`                | Sección Extras: la tabla con el switch Disponible y el modal de alta/edición |
| `GET /admin/extras/nuevo`          | Abre el modal de alta (con htmx solo el modal; sin htmx, la página con el modal abierto) |
| `POST /admin/extras`               | Crea un extra; devuelve el panel con el nuevo resaltado y cierra el modal (OOB) |
| `GET /admin/extras/{id}/editar`    | Abre el mismo modal con los datos del extra                              |
| `POST /admin/extras/{id}`          | Guarda la edición (va por POST para que el form funcione sin JS)         |
| `PUT /admin/extras/{id}/disponible`| El switch de la tabla: guarda apenas se toca y devuelve la fila          |
| `GET /admin/extras/{id}/borrar`    | Modal de confirmación del borrado                                        |
| `DELETE /admin/extras/{id}`        | Borra el extra; devuelve el panel y cierra el modal                      |
| `GET /admin/ajustes`               | Sección Ajustes: horarios, contacto, transferencia y delivery en un solo form |
| `POST /admin/ajustes`              | Guarda todo en una transacción; con htmx devuelve el form con el aviso o el error |
| `GET /admin/ajustes/transferencia` | Vista previa "Así lo ve el cliente" mientras se escribe el alias y el titular |
| `GET /admin/ajustes/cambios`       | Aviso "Tenés cambios sin guardar · DESCARTAR" (lo pide htmx con el primer cambio) |
| `PUT /admin/local/abierto`         | El switch "Local abierto" de la barra lateral: guarda apenas se toca     |
| `POST /admin/productos/foto-previa` | Valida la foto elegida y devuelve su vista previa (o el error). No guarda nada |
| `GET /admin/pedidos`               | Sección Pedidos: `?rango=hoy\|ayer\|semana\|mes` o `?desde=…&hasta=…`, y `?estado=…`; con htmx solo el panel |
| `GET /admin/pedidos/{id}`          | Detalle de un pedido (modal); lo marca visto                             |
| `GET /admin/pedidos/{id}/baja`     | Confirmación de `?accion=rechazar\|cancelar`                            |
| `POST /admin/pedidos/{id}/{accion}`| `confirmar` (con `demora`), `rechazar`, `en-camino`, `entregado`, `cancelar`; devuelve el detalle y la etiqueta de la lista (OOB) |
| `GET /admin/pedidos/aviso`         | Aviso de pedido nuevo (lo consulta el panel cada 10 s con `?actual=`): 204 si no cambió, si no el aviso nuevo |
| `GET /admin/pedidos/cargar`        | Modal "Cargar pedido" vacío (sin htmx, la página de Pedidos con el modal abierto) |
| `POST /admin/pedidos/cargar/armar` | Recalcula el modal después de cada clic (agregar, + / −, extra, categoría, entrega, descuento). No guarda nada |
| `POST /admin/pedidos/cargar`       | Guarda el pedido confirmado; devuelve "PEDIDO #N CARGADO" y la lista y las métricas (OOB). Con error, el modal con el mensaje |
| `GET /uploads/<archivo>`           | Sirve las fotos de los productos (público: las va a usar el menú). No lista la carpeta |
| `DELETE /admin/productos/{id}`     | Borra el producto. Con htmx devuelve el panel con el mismo filtro (`?categoria=`) y cierra el modal (OOB); sin htmx, 303 a `/admin` |

### Alta de productos (modal)

**+ NUEVO PRODUCTO** pide el modal y lo pone en `<div id="modal">`, al final de la página: el
modal se ve cuando ese div tiene algo adentro. Se cierra pidiendo `/admin/modal/cerrar`,
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

### Extras

Los agregados que el cliente le suma a una hamburguesa (cheddar, panceta, medallón...), en la
tabla `extra`. Se ven en `/admin/extras` con su precio como "+ $1.000" (se suma por cada unidad).

- **Disponible:** si un día no hay algo, el switch de la fila lo apaga y deja de aparecer en el
  menú sin borrarlo. Se guarda apenas se toca (`hx-put`), y la fila vuelve atenuada con "No".
- **Alta y edición** en el mismo modal (nombre, precio y disponible). **Borrar** pide confirmación
  y recuerda que, si solo falta por hoy, alcanza con apagarlo.

### Ajustes

Los datos del local que va a usar la web, en `/admin/ajustes`. Viven en la tabla `ajustes`
(una sola fila) y en `horario` (una fila por día, 0 = lunes).

- **Horarios:** cada día tiene un switch (apagado = "Cerrado") y las horas en formato 20:00. Si
  la hora de cierre es menor que la de apertura, se entiende que cierra al día siguiente (de 20:00
  a 01:00). Las horas de un día cerrado se conservan para cuando se vuelva a abrir.
- **Contacto:** el WhatsApp del local (se guarda solo con números) y la dirección para retirar.
- **Pago por transferencia:** alias o CBU y titular, con la vista previa "Así lo ve el cliente"
  que se actualiza mientras se escribe (htmx, sin JS propio).
- **Delivery:** si está apagado la web solo ofrece retiro; costo de envío (0 = gratis) y pedido
  mínimo opcional.
- Se guarda todo junto con **GUARDAR CAMBIOS**, en una transacción. Con el primer cambio aparece
  "Tenés cambios sin guardar · DESCARTAR" (DESCARTAR vuelve a cargar lo guardado).
- **Local abierto:** el switch de abajo de la barra lateral, en todas las pantallas del panel.
  Apagado, la web no toma pedidos aunque esté en horario. Se guarda apenas se toca.

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

### Promos

Una promo es "elegí **N** hamburguesas entre estas, de este tamaño, por **$X**": el cliente arma
la suya. Se guarda en `promo` (nombre, precio, cantidad, tamaño, días, foto y si está activa) y
en `promo_producto`, la tabla intermedia (N a M) con los productos que entran.

- **Las tarjetas** muestran la foto, el nombre, el precio, qué se elige ("Elegí 2 hamburguesas
  simples entre:" y los productos), los días ("Martes a domingo", "Solo martes", "Lun, Mié y
  Vie") y cuánto ahorra el cliente. El switch la **pausa** sin borrarla: deja de verse en el
  menú.
- **El modal "Nueva promo"** tiene el formulario a la izquierda:
  - foto opcional;
  - nombre y precio;
  - cuántas elige el cliente (− n +);
  - tamaño: Simples, Dobles o El cliente elige;
  - los productos que entran (hamburguesas y veggies, con "Marcar todas");
  - los días.

  A la derecha, **"Así lo ve el cliente"** y **las cuentas**.
- **Las cuentas comparan la promo con pedir lo mismo por separado**, a precio de menú. Como el
  cliente puede repetir, se calcula así:
  - lo más barato es N veces la hamburguesa más barata, y lo más caro, N veces la más cara;
  - con "El cliente elige", las baratas se cuentan en simple y las caras en doble.
  - El "ahorro hasta" es lo más caro menos el precio de la promo.
  - Si la promo sale más cara que todo eso, avisa.
  - Las hace el paquete `promos`, con los precios de la base. Los de las tarjetas son los de
    hoy: si cambia un precio del menú, cambia el ahorro.
- **En vivo sin JS**, igual que en Cargar pedido:
  - cada cambio pide `/admin/promos/vista`;
  - htmx reemplaza solo la parte que cambió (`hx-select`/`hx-select-oob`): la vista previa,
    el stepper, la lista o el contador;
  - así no se pierden el foco, el scroll ni la foto elegida.
  - Esos pedidos van urlencoded y la foto viaja recién al guardar (multipart).
- **Con "Dobles"**, los productos sin precio doble aparecen como "sin doble" y no se pueden
  marcar.
- **Borrar un producto** que está en una promo activa avisa ("Está en la promo 2 Smash"). Al
  borrarlo sale de la promo (`ON DELETE CASCADE`).
- Por ahora es solo el ABM del panel. Pedir una promo llega con el menú público.

### Producto agotado

La columna **DISPONIBLE** tiene un switch por producto: apagado es **agotado**. Se guarda apenas
se toca (`producto.disponible`), igual que el de Extras, y la fila queda atenuada.

- En **Cargar pedido** el agotado se ve, pero con la etiqueta "Agotado" en lugar de los botones.
- `pedidos.Crear` rechaza un pedido con un agotado ("Cheese Bacon está agotado"), por si llega
  igual desde un form viejo.
- El menú público lo va a usar después.

## La web del cliente

Lo que ve el cliente, del Figma (página "Pantallas"): pensado para el celu y centrado en la
compu. Todo es público (sin cuenta), **sin JavaScript propio**: htmx para lo que se recalcula y
`panel.js` solo para COPIAR. Usa su propio documento y su propio CSS (`views/cliente.templ`,
`static/cliente.css`).

| Ruta | Pantalla |
|---|---|
| `GET /` | **Inicio:** "VER EL MENÚ". Si el local está cerrado, el cartel con los horarios |
| `GET /menu` | **Menú:** promos de hoy, hamburguesas (el "+" abre su pantalla), veggies, acompañamientos y bebidas (− n + en la fila), y abajo "HACER PEDIDO · N productos · $total" |
| `GET /menu/{id}` | **Burger:** tamaño, extras (− n +), aclaraciones y "AGREGAR $15.500". Se recalcula en vivo (`POST /menu/{id}/armar`) |
| `GET /carrito` | **Tu pedido:** las líneas con − n +, el envío, el total y tus datos (entrega, dirección, pago, notas) |
| `GET /carrito/revisar` | **Revisá tu pedido,** y CONFIRMAR (`POST /pedido`) |
| `GET /pedido/{token}` | **Seguimiento:** el estado, los pasos con la hora, "Llega aprox. a las 21:50", el alias para transferir y MANDAR COMPROBANTE por WhatsApp, y el link para guardar |

- **El carrito va en una cookie** (`fb_carrito`). Guarda solo lo que se eligió: producto,
  tamaño, cantidad, extras y aclaración.
  - Los precios los pone siempre el servidor con los de la base (`pedidos.CalcularPresupuesto`),
    así que tocar la cookie no cambia lo que se cobra.
  - Si algo se agotó o se borró, la línea se saca con un aviso.
  - Los datos del cliente van en otra cookie (`fb_datos`), para no escribirlos de nuevo en el
    próximo pedido.
- **¿Toma pedidos?** Con el switch "Local abierto" **y** dentro del horario de Ajustes
  (`pedidos.AbiertoAhora`).
  - Cuenta los que cierran pasada la medianoche: de 20:00 a 01:00, a las 00:30 sigue abierto.
  - Cerrado, el menú se ve pero no deja agregar, y el carrito no deja mandar.
- **Al confirmar**, `pedidos.Crear` guarda el pedido **pendiente** con origen web. En el panel
  saltan el aviso de pedido nuevo y la alarma, y desde ahí se confirma con la demora.
- **El seguimiento se actualiza solo:** la tarjeta del estado y los pasos se piden cada 15
  segundos (`GET /pedido/{token}/estado`). Cuando el pedido se entrega o se cancela, deja de
  pedir.
- **Validaciones al revisar:**
  - local abierto y algo en el pedido;
  - nombre y teléfono con código de área;
  - la dirección si es delivery, y delivery solo si está activo;
  - el pedido mínimo de Ajustes.
- Las **aclaraciones** ("sin cebolla") se guardan por ítem (`item_pedido.aclaraciones`) y el
  panel las muestra en la lista y en el detalle.
- **Armá tu promo** (`GET /promo/{id}`, Figma 2b y 2c).
  - Desde el menú, ARMAR PROMO abre la promo: el cliente suma las hamburguesas con − n + (puede
    repetir) y ve el avance ("Te falta 1", "2 de 2").
  - En "Tu promo" elige el tamaño de cada una (si la promo es a elección: mismo precio) y sus
    extras.
  - AGREGAR AL PEDIDO se habilita cuando están todas, con "Promo $29.000 + extras $1.000".
  - Todo se recalcula sin JS (`POST /promo/{id}/armar`). Lo elegido viaja oculto en el form.
- **Cómo se guarda una promo en el pedido:** como **una línea** de `item_pedido`, con
  `promo_id`, el nombre y el precio de la promo, y en `detalle` lo elegido ("Classic (doble) +
  Cheddar · Big Forest (simple)").
  - Los extras van en `item_pedido_extra` ("Cheddar (Classic)"), así suman al total como
    cualquier extra.
  - `pedidos.Crear` valida que la promo esté activa y sea de hoy, la cantidad justa, que todas
    entren en la promo y estén disponibles, y el tamaño (el de la promo, salvo "a elección").
  - En el panel se ve "1x 2 Smash (promo): Classic (doble) + Cheddar · Big Forest (simple)".

## El panel en el celu

Con menos de 800 px de ancho, el panel se acomoda como en los frames "Celu ·" del Figma. Es **el
mismo HTML** que en la compu: lo cambia el CSS (`@media (max-width: 800px)` al final de
`static/styles.css`), así que los swaps de htmx (una fila, el panel, el OOB) andan igual.

- **Barras:** la barra lateral se oculta. Arriba queda la marca con el switch "Local abierto", y
  abajo las pestañas Pedidos, Productos, Promos, Extras y Ajustes. "Salir" va al final de
  Ajustes.
- **Las tablas pasan a ser tarjetas.** Cada celda tiene una clase `col-*` y se ubica con
  `grid-template-areas`. En Pedidos, toda la tarjeta abre el detalle: la flecha se estira,
  invisible, sobre la tarjeta.
- **Los modales pasan a ser hojas** a pantalla completa, con el pie (los botones) siempre a la
  vista. En Nueva promo, los productos van como chips.
- **Pedidos** muestra lo del Figma: las ventas y la cantidad de hoy, los chips de estado
  (con "Pendientes (2)") y las tarjetas. El gráfico y el filtro de fechas quedan para la compu.
- Los chips se deslizan de costado. Los botones dicen "+ NUEVO" o "+ CARGAR".
- **Nada puede ser más ancho que la pantalla**, porque si no el celu achica toda la página. Los
  horarios de Ajustes se compactan y las filas de chips no hacen `wrap`.

Se probó con el Chromium de Playwright en 390×844, pantalla por pantalla, contra el Figma.

## Instalar el panel como app (PWA)

El panel se puede instalar en el celu (y en la compu) y abre como una app, a pantalla completa y
sin la barra del navegador. **No usa service worker** (sería JavaScript): alcanza con el
`static/manifest.json`, los íconos de `static/icons/` y las metas del `<head>` de `Documento`.

| Dónde | Cómo |
|---|---|
| Android (Chrome) | Menú ⋮ → **Instalar app** |
| iPhone (Safari) | Compartir → **Agregar a inicio** |
| Compu (Chrome o Edge) | El ícono de instalar en la barra de direcciones |

- **Hace falta HTTPS.** En la compu, `localhost` cuenta como seguro. Desde el celu, por la IP
  de la red (`http://192.168…`), no se instala: queda como un acceso directo común. Para
  probarlo antes de tenerlo en producción hay que usar un túnel HTTPS, por ejemplo
  `cloudflared tunnel --url http://localhost:8080`.
- **Sin internet no abre.** Sin service worker no hay nada guardado. Igual, el panel no sirve
  sin el servidor.
- La sesión dura 7 días, como en el navegador. Cuando vence, la app abre en el login.
- **Los íconos** son los pinos del logo (`static/img/logoForestburger.svg`) sobre el verde del
  panel. Los hice una vez con el Chromium de Playwright, porque no hay ImageMagick:
  - `icon-192` e `icon-512` son los comunes;
  - `maskable-512` tiene más margen, porque Android lo recorta en círculo;
  - `apple-touch-icon` es el del iPhone.
- **Verificado** con la herramienta de Chrome que decide si ofrece "Instalar"
  (`Page.getInstallabilityErrors`): no da ningún error.

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
  base. `make run` conserva la base (y los admins); `make run-app` y `make clean` la borran, y
  después hay que volver a crear los admins (o usar el admin de desarrollo, abajo).
- **Admin de desarrollo:** para no correr `make crear-admin` después de cada `make run`, en el
  `.env` de tu máquina podés poner `DEV_ADMIN_EMAIL` y `DEV_ADMIN_PASSWORD`. Si están las dos, al
  arrancar la app crea (o actualiza) ese admin y avisa "Admin de desarrollo listo: <email>" (la
  contraseña nunca se muestra). El mail tiene que estar en `ADMIN_EMAILS` y la contraseña tener al
  menos 8 caracteres; si no, avisa el motivo y arranca igual. **Son solo para desarrollo: en
  producción no se ponen**, y sin ellas no hace nada.

## Pedidos (modelo de datos)

Los pedidos llegan de la web o se cargan en el panel (próximas fases). Las tablas:

- **`pedido`:** el cliente, de dónde vino (web, WhatsApp, teléfono o local), la entrega (retiro o
  delivery con dirección), el pago (efectivo o transferencia), el estado (pendiente, confirmado,
  entregado o cancelado), la demora avisada, las cuentas (subtotal, descuento en % con su motivo,
  envío y total) y el `token` del link de seguimiento.
- **`item_pedido`** e **`item_pedido_extra`:** lo que se pidió. Guardan **una copia del nombre y
  del precio** del momento: si después cambia o se borra un producto o un extra, el pedido (en
  curso o viejo) se sigue viendo igual (la referencia queda en `NULL`). Por eso se puede borrar
  un producto que está en pedidos; el modal de borrado avisa si está en pedidos en curso.
- Los extras van **por cada unidad**: "2x Classic + cheddar" son dos Classic, cada una con
  cheddar. Una línea cuesta `cantidad × (precio + extras)`.
- **`pedido_evento`:** el historial ("21:05 Recibido", "21:07 Confirmado · 30 min").

Los pedidos se crean con `pedidos.Crear` (paquete `pedidos/`), que toma los precios de la base y
hace las cuentas en el servidor (en centavos, para no perder ni uno por redondeo): quien crea el
pedido solo manda qué productos y extras, y cuántos.

### Atender pedidos (`/admin/pedidos`)

- **Arriba, las ventas del período que se está mirando**, el mismo de la lista (Hoy, Ayer, Esta
  semana, Este mes o Desde/Hasta). No las cambia el filtro de estado. Hay tres tarjetas:
  - **Ventas:** el total, cuánto entró en efectivo y cuánto por transferencia, y el % contra el
    período anterior;
  - **Pedidos:** la cantidad, con los cancelados aparte;
  - **Ticket promedio.**
- **La comparación es justa con los períodos en curso:**
  - hoy contra ayer;
  - esta semana, del lunes a hoy contra del lunes al mismo día de la semana pasada;
  - este mes, del 1 a hoy contra del 1 al mismo día del mes pasado;
  - un rango a mano, contra el tramo anterior del mismo largo.
- **Debajo, el gráfico** (barras hechas con CSS, sin librerías):
  - la semana de lunes a domingo si se mira un solo día o una semana;
  - un día por barra en el mes o en un rango de hasta 31 días;
  - una barra por semana en un rango más largo.
  - Con muchas barras, el monto se ve al pasar el mouse.
- Al cambiar el filtro, htmx trae la lista y, por OOB, las tarjetas y el gráfico.
- No cuentan los pedidos cancelados. Sale de una sola consulta (`VentasPorDia`, que cubre el
  período, el anterior y el gráfico) más el conteo de cancelados. Las cuentas las hace
  `pedidos.Metricas`.
- **La lista** se filtra por fecha (Hoy, Ayer, Esta semana de lunes a domingo, Este mes, o un
  rango Desde/Hasta) y por estado. Todo va en la URL, como los filtros de Productos. Cada fila
  muestra el resumen ("2x Cheese Bacon (doble) + cheddar, 1x Gaseosa lata") y la etiqueta de
  color del estado. Las fechas se cuentan con la hora de la Argentina.
- **El detalle** (la flecha ">") muestra los datos, las líneas con sus cuentas, el link de
  seguimiento del cliente y el historial. Con las acciones de cada estado:

  | Estado | Acciones |
  |---|---|
  | pendiente | elegir la demora (30, 45 o 60 min, la ve el cliente) y CONFIRMAR, o RECHAZAR |
  | confirmado | delivery: MARCAR EN CAMINO; retiro: MARCAR ENTREGADO; CANCELAR PEDIDO |
  | en camino | MARCAR ENTREGADO; CANCELAR PEDIDO |

  Rechazar y cancelar piden confirmación. Si dos personas tocan el mismo pedido a la vez, la
  segunda ve "Este pedido ya cambió de estado" con el estado real (el cambio va condicionado al
  estado que se leyó). Cada cambio queda en el historial.
- **El link de seguimiento** se puede **copiar** (COPIAR) o **mandar por WhatsApp** (un link
  `wa.me` con el mensaje ya escrito). La página que abre ese link es de una etapa posterior.
- **COPIAR es el único JavaScript del panel** (`static/panel.js`): copiar al portapapeles no se
  puede hacer de otra forma. Todo lo demás es htmx.
- Pendiente para más adelante: IMPRIMIR COMANDA.

### Aviso de pedido nuevo

Cuando hay un pedido **pendiente que nadie abrió**, arriba de todas las pantallas del panel (no
solo en Pedidos) aparece el banner amarillo "Nuevo pedido #17 · Martina · $17.000" (y "y 2 más"
si hay varios), suena una alarma y la pestaña dice "(3) Pedido nuevo".

- **Sin JS propio:** el lugar del aviso se consulta solo cada 10 segundos (`hx-trigger="every
  10s"`) y le dice al servidor qué está mostrando (`?actual=17-3`). Si no cambió nada, el
  servidor contesta **204** y htmx no toca la página, así la alarma no se corta ni vuelve a
  empezar. Si cambió, manda el aviso nuevo.
- **VER** es un link común al pedido. Al abrir el detalle (desde ahí o desde la lista) queda
  visto, y como mucho 10 segundos después el aviso se va de todas las pantallas abiertas.
- Cuentan solo los **pendientes sin abrir**. Los que se cargan a mano ya entran confirmados, y un
  pedido rechazado sin abrir deja de avisar.
- **La alarma** es `static/alarma.wav` (un "din-don" de 1 s, hecho con el módulo `wave` de
  Python), con `<audio autoplay loop>` dentro del banner.

**Para que la alarma suene siempre**, en la compu del local hay que darle permiso de
reproducción automática al sitio **una sola vez**. Los navegadores no dejan que una página haga
ruido hasta que la persona tocó algo en ella:

| Navegador | Cómo |
|---|---|
| Firefox | Candado de la barra de direcciones → "Reproducción automática" → **Permitir audio y video** |
| Edge | Candado → Permisos de este sitio → "Reproducción automática de medios" → **Permitir** |
| Chrome | No tiene ese permiso por sitio: conviene usar Firefox o Edge en la compu del local |

Sin ese permiso suena igual en la mayoría de los casos: después de cualquier clic en el panel
(por ejemplo, pasar de una sección a otra), cuando llega un pedido nuevo. Lo que no suena es
**recargar la página con el aviso ya puesto y no tocar nada**: el banner se ve y la pestaña
avisa, pero sin sonido hasta que llegue otro pedido o se cambie de sección.

### Cargar pedido (`+ CARGAR PEDIDO`)

Para los pedidos que entran por WhatsApp, por teléfono o en el mostrador. El pedido entra
**confirmado** (con el historial "Recibido" y "Confirmado").

- **A la izquierda**, los productos por categoría, con un botón por tamaño ("Simple $14.500",
  "Doble $16.500"). **A la derecha**, el cliente (nombre, teléfono opcional, por dónde vino), la
  entrega (la dirección solo aparece con delivery), el pago, el pedido armado y las cuentas.
- **Sin JavaScript propio:** todo el modal es un solo form, y el pedido que se va armando viaja
  en inputs ocultos (`item_0_producto`, `item_0_tamano`, `item_0_cantidad`, `item_0_extras`).
  Cada botón manda el form entero con su acción a `/admin/pedidos/cargar/armar` y htmx reemplaza
  el modal por el recalculado. El servidor no guarda nada entre clics y lo que ya se escribió
  (nombre, dirección, motivo) no se pierde.
- **Reglas:** tocar un tamaño que ya está (sin extras) suma uno a ese renglón; el "−" en 1 lo
  saca. Los extras disponibles se prenden y apagan en cada hamburguesa o veggie, y van por
  unidad.
- **Descuento** en % (redondeado al peso) o en $ fijo (nunca más que el subtotal), con un motivo
  opcional. Se aplica sobre el subtotal, no sobre el envío.
- **Las cuentas las hace siempre el servidor** con los precios de la base:
  `pedidos.CalcularPresupuesto` para lo que se ve mientras se arma y `pedidos.Crear` para guardar
  usan el mismo código, así que el total que se ve es el que se guarda.
- Al guardar aparece **"PEDIDO #N CARGADO"** con el link de seguimiento, COPIAR y MANDAR POR
  WHATSAPP (si se cargó el teléfono). La lista y las métricas de atrás se actualizan solas.

### Datos de prueba

Con `DEV_DATOS_PRUEBA=1` en el `.env`, al arrancar con la base sin productos se cargan solos:
9 productos, 4 extras, ajustes de ejemplo y unos 50 pedidos de los últimos 10 días (con estados,
pagos y entregas variados, para ver el panel de pedidos). Se cargan **una sola vez**: como
`make run` conserva la base, las corridas siguientes no tocan nada. Para empezar de cero,
`make clean`. **Solo para desarrollo:** en producción no se pone y la base es otra.

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

- El único archivo `.js` local es `static/panel.js` (el botón COPIAR del detalle de un pedido); todo lo demás es htmx.
- `make test` ejecuta los paquetes de a uno (`go test -p 1`) porque los tests de `handlers`,
  `db`, `pedidos` y `seed` comparten la base de test (`forestburger_test`).
- Asegurarse de tener `$(go env GOPATH)/bin` en la variable de entorno `PATH`
  para que `make` pueda invocar tanto `sqlc` como `templ`.
- Si los puertos `5432` (PostgreSQL) u `8080` (Servidor Web) están ocupados en
  la máquina, liberarlos antes de ejecutar `make run-app` o `make test`.