#!/usr/bin/env bash
set -e

API_URL="http://localhost:8080/api/productos"
BASE_URL="http://localhost:8080/api"

echo "==> 1. Compilando y levantando el servidor de la API..."
go build -o forestburger-server .
./forestburger-server &
SERVER_PID=$!

# Asegurar que el servidor se apague siempre al terminar el script (aunque falle un test)
cleanup() {
    echo "==> Deteniendo servidor (PID $SERVER_PID)..."
    kill $SERVER_PID 2>/dev/null || true
    rm -f forestburger-server
}
trap cleanup EXIT

echo "==> 2. Esperando a que el servidor esté disponible en :8080..."
until curl -s -o /dev/null "http://localhost:8080/"; do
    if ! kill -0 $SERVER_PID 2>/dev/null; then
        echo "ERROR: el servidor terminó antes de estar disponible"
        exit 1
    fi
    sleep 0.5
done
echo "Servidor listo."

echo "----------------------------------------"
echo "TEST 1: POST /api/productos (Crear producto con precio doble)"
RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$API_URL" \
    -H "Content-Type: application/json" \
    -d '{"nombre":"Forest Doble","categoria":"hamburguesa","precio":9500.50,"precio_doble":11500,"descripcion":"Doble carne y cheddar"}')
BODY=$(echo "$RESPONSE" | head -n 1)
STATUS=$(echo "$RESPONSE" | tail -n 1)
echo "Respuesta ($STATUS): $BODY"
if [ "$STATUS" -ne 201 ]; then
    echo "ERROR: Se esperaba status 201 y se obtuvo $STATUS"
    exit 1
fi
if ! echo "$BODY" | grep -q '"precio_doble":11500'; then
    echo "ERROR: Se esperaba precio_doble 11500"
    exit 1
fi
# Extraer el ID generado (usando grep/sed sin depender de jq)
PROD_ID=$(echo "$BODY" | grep -o '"id":[0-9]*' | head -n 1 | cut -d':' -f2)
echo "-> Producto creado con ID=$PROD_ID y precio doble (OK)"

echo "----------------------------------------"
echo "TEST 2: GET /api/productos (Listar productos)"
RESPONSE=$(curl -s -w "\n%{http_code}" -X GET "$API_URL")
BODY=$(echo "$RESPONSE" | head -n 1)
STATUS=$(echo "$RESPONSE" | tail -n 1)
echo "Respuesta ($STATUS): $BODY"
if [ "$STATUS" -ne 200 ]; then
    echo "ERROR: Se esperaba status 200 y se obtuvo $STATUS"
    exit 1
fi
echo "-> Listado obtenido correctamente (OK)"

echo "----------------------------------------"
echo "TEST 3: GET /api/productos/$PROD_ID (Obtener producto por ID)"
RESPONSE=$(curl -s -w "\n%{http_code}" -X GET "$API_URL/$PROD_ID")
BODY=$(echo "$RESPONSE" | head -n 1)
STATUS=$(echo "$RESPONSE" | tail -n 1)
echo "Respuesta ($STATUS): $BODY"
if [ "$STATUS" -ne 200 ]; then
    echo "ERROR: Se esperaba status 200 y se obtuvo $STATUS"
    exit 1
fi
echo "-> Producto obtenido por ID correctamente (OK)"

echo "----------------------------------------"
echo "TEST 4: PUT /api/productos/$PROD_ID (Actualizar producto)"
RESPONSE=$(curl -s -w "\n%{http_code}" -X PUT "$API_URL/$PROD_ID" \
    -H "Content-Type: application/json" \
    -d '{"nombre":"Forest Triple","categoria":"hamburguesa","precio":11500.00,"precio_doble":13500,"descripcion":"Triple carne, bacon y cheddar"}')
BODY=$(echo "$RESPONSE" | head -n 1)
STATUS=$(echo "$RESPONSE" | tail -n 1)
echo "Respuesta ($STATUS): $BODY"
if [ "$STATUS" -ne 200 ]; then
    echo "ERROR: Se esperaba status 200 y se obtuvo $STATUS"
    exit 1
fi
if ! echo "$BODY" | grep -q '"precio_doble":13500'; then
    echo "ERROR: Se esperaba precio_doble 13500 tras actualizar"
    exit 1
fi
echo "-> Producto actualizado correctamente (OK)"

echo "----------------------------------------"
echo "TEST 5: DELETE /api/productos/$PROD_ID (Eliminar producto)"
STATUS=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "$API_URL/$PROD_ID")
echo "Status recibido: $STATUS"
if [ "$STATUS" -ne 204 ]; then
    echo "ERROR: Se esperaba status 204 y se obtuvo $STATUS"
    exit 1
fi
echo "-> Producto eliminado correctamente (OK)"

echo "----------------------------------------"
echo "TEST 6: GET /api/productos/$PROD_ID (Verificar 404 tras eliminar)"
STATUS=$(curl -s -o /dev/null -w "%{http_code}" -X GET "$API_URL/$PROD_ID")
echo "Status recibido: $STATUS"
if [ "$STATUS" -ne 404 ]; then
    echo "ERROR: Se esperaba status 404 y se obtuvo $STATUS"
    exit 1
fi
echo "-> Verificación 404 exitosa (OK)"

echo "----------------------------------------"
echo "TEST 7: POST /api/extras (Crear extras)"
RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/extras" \
    -H "Content-Type: application/json" \
    -d '{"nombre":"Cheddar","precio":500}')
BODY=$(echo "$RESPONSE" | head -n 1)
STATUS=$(echo "$RESPONSE" | tail -n 1)
echo "Respuesta ($STATUS): $BODY"
if [ "$STATUS" -ne 201 ]; then
    echo "ERROR: Se esperaba status 201 y se obtuvo $STATUS"
    exit 1
fi
CHEDDAR_ID=$(echo "$BODY" | grep -o '"id":[0-9]*' | head -n 1 | cut -d':' -f2)

RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/extras" \
    -H "Content-Type: application/json" \
    -d '{"nombre":"Bacon","precio":800}')
BODY=$(echo "$RESPONSE" | head -n 1)
STATUS=$(echo "$RESPONSE" | tail -n 1)
echo "Respuesta ($STATUS): $BODY"
if [ "$STATUS" -ne 201 ]; then
    echo "ERROR: Se esperaba status 201 y se obtuvo $STATUS"
    exit 1
fi
BACON_ID=$(echo "$BODY" | grep -o '"id":[0-9]*' | head -n 1 | cut -d':' -f2)
echo "-> Extras creados: Cheddar ID=$CHEDDAR_ID, Bacon ID=$BACON_ID (OK)"

echo "----------------------------------------"
echo "TEST 8: GET /api/extras (Listar extras)"
RESPONSE=$(curl -s -w "\n%{http_code}" -X GET "$BASE_URL/extras")
BODY=$(echo "$RESPONSE" | head -n 1)
STATUS=$(echo "$RESPONSE" | tail -n 1)
echo "Respuesta ($STATUS): $BODY"
if [ "$STATUS" -ne 200 ]; then
    echo "ERROR: Se esperaba status 200 y se obtuvo $STATUS"
    exit 1
fi
echo "-> Listado de extras obtenido correctamente (OK)"

echo "----------------------------------------"
echo "TEST 9: POST /api/productos (Crear productos para el pedido)"
RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$API_URL" \
    -H "Content-Type: application/json" \
    -d '{"nombre":"Forest Doble","categoria":"hamburguesa","precio":9500,"precio_doble":11500,"descripcion":"Doble carne y cheddar"}')
BODY=$(echo "$RESPONSE" | head -n 1)
STATUS=$(echo "$RESPONSE" | tail -n 1)
if [ "$STATUS" -ne 201 ]; then
    echo "ERROR: Se esperaba status 201 y se obtuvo $STATUS"
    exit 1
fi
HAMB_ID=$(echo "$BODY" | grep -o '"id":[0-9]*' | head -n 1 | cut -d':' -f2)

# Las papas no tienen versión doble: precio_doble queda null
RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$API_URL" \
    -H "Content-Type: application/json" \
    -d '{"nombre":"Papas","categoria":"acompañamiento","precio":3000}')
BODY=$(echo "$RESPONSE" | head -n 1)
STATUS=$(echo "$RESPONSE" | tail -n 1)
if [ "$STATUS" -ne 201 ]; then
    echo "ERROR: Se esperaba status 201 y se obtuvo $STATUS"
    exit 1
fi
if ! echo "$BODY" | grep -q '"precio_doble":null'; then
    echo "ERROR: Las papas deberían tener precio_doble null"
    exit 1
fi
PAPAS_ID=$(echo "$BODY" | grep -o '"id":[0-9]*' | head -n 1 | cut -d':' -f2)
echo "-> Productos creados: Forest Doble ID=$HAMB_ID, Papas ID=$PAPAS_ID (OK)"

echo "----------------------------------------"
echo "TEST 10: POST /api/pedidos (Crear pedido con tamaño doble y extras)"
# Total esperado: 2 x 11500 (doble) + 1 x 500 (cheddar) + 2 x 800 (bacon) + 1 x 3000 (papas) = 28100
RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/pedidos" \
    -H "Content-Type: application/json" \
    -d '{
      "nombre_cliente": "Ulises",
      "tipo_entrega": "delivery",
      "direccion": "Av. Colón 1234",
      "forma_pago": "efectivo",
      "notas": "Tocar timbre 2B",
      "items": [
        {"id_producto": '"$HAMB_ID"', "cantidad": 2, "tamano": "doble", "aclaraciones": "sin cebolla",
         "extras": [{"id_extra": '"$CHEDDAR_ID"', "cantidad": 1}, {"id_extra": '"$BACON_ID"', "cantidad": 2}]},
        {"id_producto": '"$PAPAS_ID"', "cantidad": 1, "extras": []}
      ]
    }')
BODY=$(echo "$RESPONSE" | head -n 1)
STATUS=$(echo "$RESPONSE" | tail -n 1)
echo "Respuesta ($STATUS): $BODY"
if [ "$STATUS" -ne 201 ]; then
    echo "ERROR: Se esperaba status 201 y se obtuvo $STATUS"
    exit 1
fi
if ! echo "$BODY" | grep -q '"total":28100'; then
    echo "ERROR: Se esperaba total 28100"
    exit 1
fi
if ! echo "$BODY" | grep -q '"estado":"pendiente"'; then
    echo "ERROR: Se esperaba estado pendiente"
    exit 1
fi
PEDIDO_ID=$(echo "$BODY" | grep -o '"id":[0-9]*' | head -n 1 | cut -d':' -f2)
echo "-> Pedido creado con ID=$PEDIDO_ID, total 28100 y estado pendiente (OK)"

echo "----------------------------------------"
echo "TEST 11: GET /api/pedidos (Listar pedidos)"
RESPONSE=$(curl -s -w "\n%{http_code}" -X GET "$BASE_URL/pedidos")
BODY=$(echo "$RESPONSE" | head -n 1)
STATUS=$(echo "$RESPONSE" | tail -n 1)
echo "Respuesta ($STATUS): $BODY"
if [ "$STATUS" -ne 200 ]; then
    echo "ERROR: Se esperaba status 200 y se obtuvo $STATUS"
    exit 1
fi
echo "-> Listado de pedidos obtenido correctamente (OK)"

echo "----------------------------------------"
echo "TEST 12: GET /api/pedidos/$PEDIDO_ID (Ver pedido con su detalle)"
RESPONSE=$(curl -s -w "\n%{http_code}" -X GET "$BASE_URL/pedidos/$PEDIDO_ID")
BODY=$(echo "$RESPONSE" | head -n 1)
STATUS=$(echo "$RESPONSE" | tail -n 1)
echo "Respuesta ($STATUS): $BODY"
if [ "$STATUS" -ne 200 ]; then
    echo "ERROR: Se esperaba status 200 y se obtuvo $STATUS"
    exit 1
fi
if ! echo "$BODY" | grep -q '"nombre_producto":"Forest Doble"' || ! echo "$BODY" | grep -q '"nombre_extra":"Cheddar"'; then
    echo "ERROR: El detalle no trae los productos y extras con nombre"
    exit 1
fi
if ! echo "$BODY" | grep -q '"tamano":"doble"' || ! echo "$BODY" | grep -q '"precio_unitario":11500'; then
    echo "ERROR: El item doble no se guardó con su tamaño y precio doble"
    exit 1
fi
if ! echo "$BODY" | grep -q '"tamano":null'; then
    echo "ERROR: Las papas deberían tener tamano null"
    exit 1
fi
echo "-> Detalle del pedido con items, tamaños y extras correcto (OK)"

echo "----------------------------------------"
echo "TEST 13: PUT /api/pedidos/$PEDIDO_ID/estado (Cambiar estado)"
RESPONSE=$(curl -s -w "\n%{http_code}" -X PUT "$BASE_URL/pedidos/$PEDIDO_ID/estado" \
    -H "Content-Type: application/json" \
    -d '{"estado":"confirmado"}')
BODY=$(echo "$RESPONSE" | head -n 1)
STATUS=$(echo "$RESPONSE" | tail -n 1)
echo "Respuesta ($STATUS): $BODY"
if [ "$STATUS" -ne 200 ] || ! echo "$BODY" | grep -q '"estado":"confirmado"'; then
    echo "ERROR: No se pudo cambiar el estado a confirmado"
    exit 1
fi
echo "-> Estado actualizado a confirmado (OK)"

echo "----------------------------------------"
echo "TEST 14: POST /api/pedidos con carrito vacío (Verificar 400)"
STATUS=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE_URL/pedidos" \
    -H "Content-Type: application/json" \
    -d '{"nombre_cliente":"Ulises","tipo_entrega":"take_away","forma_pago":"efectivo","items":[]}')
echo "Status recibido: $STATUS"
if [ "$STATUS" -ne 400 ]; then
    echo "ERROR: Se esperaba status 400 y se obtuvo $STATUS"
    exit 1
fi
echo "-> Pedido inválido rechazado correctamente (OK)"

echo "----------------------------------------"
echo "TEST 15: POST /api/pedidos con hamburguesa sin tamaño (Verificar 400)"
STATUS=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE_URL/pedidos" \
    -H "Content-Type: application/json" \
    -d '{"nombre_cliente":"Ulises","tipo_entrega":"take_away","forma_pago":"efectivo","items":[{"id_producto":'"$HAMB_ID"',"cantidad":1}]}')
echo "Status recibido: $STATUS"
if [ "$STATUS" -ne 400 ]; then
    echo "ERROR: Se esperaba status 400 y se obtuvo $STATUS"
    exit 1
fi
echo "-> Hamburguesa sin tamaño rechazada correctamente (OK)"

echo "----------------------------------------"
echo "TEST 16: POST /api/pedidos con papas con tamaño (Verificar 400)"
STATUS=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE_URL/pedidos" \
    -H "Content-Type: application/json" \
    -d '{"nombre_cliente":"Ulises","tipo_entrega":"take_away","forma_pago":"efectivo","items":[{"id_producto":'"$PAPAS_ID"',"cantidad":1,"tamano":"doble"}]}')
echo "Status recibido: $STATUS"
if [ "$STATUS" -ne 400 ]; then
    echo "ERROR: Se esperaba status 400 y se obtuvo $STATUS"
    exit 1
fi
echo "-> Producto sin tamaño pedido como doble rechazado correctamente (OK)"

echo "----------------------------------------"
echo "TEST 17: POST /api/pedidos con tamaño inválido (Verificar 400)"
STATUS=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE_URL/pedidos" \
    -H "Content-Type: application/json" \
    -d '{"nombre_cliente":"Ulises","tipo_entrega":"take_away","forma_pago":"efectivo","items":[{"id_producto":'"$HAMB_ID"',"cantidad":1,"tamano":"triple"}]}')
echo "Status recibido: $STATUS"
if [ "$STATUS" -ne 400 ]; then
    echo "ERROR: Se esperaba status 400 y se obtuvo $STATUS"
    exit 1
fi
echo "-> Tamaño inválido rechazado correctamente (OK)"

echo "========================================"
echo "TODOS LOS TESTS DE LA API PASARON CON EXITO"
echo "========================================"