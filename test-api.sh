#!/usr/bin/env bash
set -e

API_URL="http://localhost:8080/api/productos"

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
    sleep 0.5
done
echo "Servidor listo."

echo "----------------------------------------"
echo "TEST 1: POST /api/productos (Crear producto)"
RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$API_URL" \
    -H "Content-Type: application/json" \
    -d '{"nombre":"Forest Doble","categoria":"hamburguesa","precio":9500.50,"descripcion":"Doble carne y cheddar"}')
BODY=$(echo "$RESPONSE" | head -n 1)
STATUS=$(echo "$RESPONSE" | tail -n 1)
echo "Respuesta ($STATUS): $BODY"
if [ "$STATUS" -ne 201 ]; then
    echo "ERROR: Se esperaba status 201 y se obtuvo $STATUS"
    exit 1
fi
# Extraer el ID generado (usando grep/sed sin depender de jq)
PROD_ID=$(echo "$BODY" | grep -o '"id":[0-9]*' | head -n 1 | cut -d':' -f2)
echo "-> Producto creado con ID=$PROD_ID (OK)"

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
    -d '{"nombre":"Forest Triple","categoria":"hamburguesa","precio":11500.00,"descripcion":"Triple carne, bacon y cheddar"}')
BODY=$(echo "$RESPONSE" | head -n 1)
STATUS=$(echo "$RESPONSE" | tail -n 1)
echo "Respuesta ($STATUS): $BODY"
if [ "$STATUS" -ne 200 ]; then
    echo "ERROR: Se esperaba status 200 y se obtuvo $STATUS"
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

echo "========================================"
echo "TODOS LOS TESTS DE LA API PASARON CON EXITO"
echo "========================================"
