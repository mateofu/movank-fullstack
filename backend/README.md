# Backend Go

Un proceso Go con PostgreSQL. `cmd/api` arranca el servicio; `internal` contiene
la implementación y `migrations` los cambios de esquema.

Arranque: [guía principal](../README.md). Contrato: [OpenAPI](../docs/api/openapi.yaml).

## Rutas disponibles

- `GET /healthz`: proceso activo.
- `GET /readyz`: conexión a PostgreSQL; no comprueba migraciones.
- `GET /v1/me`: identidad autenticada.
- `POST /v1/session` y `DELETE /v1/session`: abrir y cerrar sesión del navegador.
- `POST /v1/products`: crear producto.
- `GET /v1/products`: catálogo paginado.
- `GET /v1/products/{id}`: consultar producto.
- `POST /v1/sales`: crear venta; requiere `Idempotency-Key`.
- `GET /v1/sales/{id}`: consultar venta.
- `POST /v1/sales/{id}/pay`: pagar con `method: CARD` y un `scenario`.
- `GET /v1/dashboard/today`: pagos aprobados del día UTC.
- `GET /v1/dashboard/stream`: agregado completo por SSE.

Las rutas de negocio requieren JWT por Bearer o cookie. El comercio viene del token verificado y las
consultas filtran por él; no hay RLS. Los importes son enteros en centavos de COP.
El SKU es único por comercio. La paginación no es una instantánea ante nuevas altas.

## Token local

Desde la raíz, con los servicios y migraciones preparados:

```powershell
$merchantId = [Guid]::NewGuid().ToString()
$userId = [Guid]::NewGuid().ToString()
docker compose exec -T postgres psql -U movank -d movank -v ON_ERROR_STOP=1 -c "INSERT INTO public.merchants (id, name) VALUES ('$merchantId', 'Demo');"
$token = docker compose exec -T backend /api token $merchantId $userId
Invoke-RestMethod http://127.0.0.1:8080/v1/me -Headers @{ Authorization = "Bearer $token" }
```

El token dura 15 minutos. Es una herramienta del operador: no hay login,
renovación ni revocación individual. No guardes tokens en Git ni localStorage.
El navegador lo intercambia por una cookie HttpOnly, SameSite=Strict y Secure en
HTTPS. Las escrituras con cookie validan Origin. `X-Workspace` solo comprueba
que la sesión no cambió; nunca selecciona el comercio. Detrás de un proxy HTTPS,
configura `PUBLIC_ORIGIN` con el origen público. En local puede quedar vacío.

## Ventas y migraciones

Envía `{"items":[{"product_id":"UUID","quantity":2}]}` con una clave estable.
Go obtiene precios del catálogo y guarda venta y detalle en una transacción.
Admite 1–100 productos distintos, cantidades 1–10000 y total hasta 1000000000000 centavos.

Un bloqueo transaccional por comercio y clave ordena los reintentos. El hash
SHA-256 del pedido normalizado detecta cambios: misma clave y pedido devuelve
la venta original con 201; otro pedido devuelve 409. Orden y mayúsculas de UUID
no importan. Las claves no caducan y sobreviven reinicios; tras timeout o 503,
repite la misma clave y pedido. Los errores de validación no reservan la clave.
Los precios históricos no cambian.

Las migraciones son transaccionales, serializadas y repetibles. No se editan una
vez aplicadas; las correcciones usan otra migración. No hay reversión automática.

## Pagos y dashboard

Un intento por venta: repetir método y escenario devuelve el estado actual;
cambiarlos devuelve 409. La venta actúa como clave de idempotencia del pago.
`APPROVED` y `DECLINED` son finales. `TIMEOUT` queda `UNKNOWN` y se consulta después.
El simulador confirma ese timeout como aprobado; no procesa dinero real.

El intento `PENDING` se guarda antes de llamar al proveedor. Si el proceso cae,
el worker puede reenviarlo con la misma referencia: el simulador la deduplica en
PostgreSQL. Para `UNKNOWN` solo consulta. Reintenta cada 5–60 segundos y conserva
las comprobaciones; una respuesta incierta nunca se convierte en rechazo.

La aprobación y el outbox comparten transacción. Un worker interno escucha avisos
y revisa pendientes cada 2 segundos. Reprocesar envía una instantánea, no suma otra vez.
Redis guarda el dashboard 30 segundos: una operación atómica impide retrocesos;
`singleflight` agrupa reconstrucciones simultáneas. Sin Redis se consulta PostgreSQL.
SSE comparte eventos por comercio, manda una instantánea al reconectar y termina
al vencer el JWT. Se consume con `fetch` y Bearer o cookie, sin tokens en la URL.

Límites: un proceso Go, días UTC, sin reembolso ni segundo intento tras rechazo.
Proveedor real, login y revisión operativa de pagos irresueltos quedan fuera.

## Pruebas

El build comprueba formato, ejecuta `go vet` y `go test -race`. Para repetirlo:

```powershell
docker compose build --no-cache backend
```

Pruebas con PostgreSQL real: [integración](../tests/integration/README.md).
