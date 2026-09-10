# ForestBurger

Aplicación web para una hamburguesería artesanal. Gestiona dos entidades
principales: los **productos** del menú (nombre, descripción, precio y
categoría) y los **pedidos** de los clientes (items seleccionados, cantidad,
total, tipo de entrega, forma de pago y aclaraciones). Al finalizar un
pedido, la aplicación genera automáticamente un mensaje de WhatsApp con el
detalle y lo envía al local, para que la gestión de pedidos no dependa de ir
y venir con cada cliente.

Este repositorio corresponde a la entrega del **TP2**, centrada en la capa
de persistencia del proyecto usando **sqlc** sobre PostgreSQL.

## Tecnologías

- Go
- [sqlc](https://sqlc.dev/) para generar código Go a partir de SQL
- PostgreSQL 16
- Docker y Docker Compose
- Paquete `testing` de Go para los tests de integración

## Requisitos previos

- Go instalado
- [sqlc](https://docs.sqlc.dev/en/latest/overview/install.html) instalado
- Docker y Docker Compose instalados y funcionando (el usuario debe poder
  ejecutar `docker` sin `sudo`)
- `make`

## Cómo ejecutar

1. Clonar el repositorio y pararse en la rama `tp2`:

   ```bash
   git clone https://github.com/ulisesmatteoda/ForestBurger.git
   cd ForestBurger
   git checkout tp2
   ```

2. Ejecutar el pipeline completo de tests:

   ```bash
   make test
   ```

   Este comando se encarga de todo el flujo necesario:
   - Genera el código Go a partir de las queries (`sqlc generate`)
   - Compila el proyecto
   - Elimina contenedores y volúmenes previos, y levanta la base de datos
     en Docker
   - Espera a que PostgreSQL esté aceptando conexiones
   - Corre los tests de integración contra la base real
   - Al finalizar (haya pasado o fallado el test), elimina los
     contenedores y volúmenes creados

## Estructura del proyecto

```
ForestBurger/
├── db/
│   ├── sqlc/
│   │   ├── db.go              # generado por sqlc
│   │   ├── models.go          # generado por sqlc
│   │   └── queries.sql.go     # generado por sqlc
│   ├── producto_repository_test.go   # tests de integración
│   ├── schema.sql         # definición de tablas
│   └── queries.sql        # queries fuente para sqlc
├── static/
│   └── index.html
├── docker-compose.yml
├── sqlc.yaml
├── Makefile
├── .env.example
└── README.md
```

> Los archivos `.go` dentro de `db/sqlc/` son generados automáticamente por
> `sqlc generate` como parte de `make test`, por eso no se versionan en el
> repositorio.

## Tests

El archivo `db/producto_repository_test.go` cubre el flujo CRUD completo
sobre la entidad `Producto`, contra una instancia real de PostgreSQL
levantada en Docker:

- Creación de un producto
- Obtención de un producto por ID
- Listado de productos
- Actualización de un producto
- Eliminación de un producto

## Notas

- Si el puerto `5432` ya está ocupado en tu máquina por otra instancia de
  PostgreSQL (local o de otro proyecto en Docker), liberalo antes de correr
  `make test`, o ajustá el mapeo de puertos en `docker-compose.yml` (por
  ejemplo a `5433:5432`) y actualizá la cadena de conexión usada en los
  tests en consecuencia.
