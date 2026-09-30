# Backend Go

Un proceso Go con PostgreSQL. `cmd/api` arranca el servicio; `internal` contiene
la implementación y `migrations` los cambios de esquema.

Arranque: [guía principal](../README.md). Contrato: [OpenAPI](../docs/api/openapi.yaml).

## Rutas disponibles

- `GET /healthz`: proceso activo.
- `GET /readyz`: conexión a PostgreSQL; no comprueba migraciones.
- `GET /v1/me`: identidad autenticada.
- `POST /v1/products`: crear producto.
- `GET /v1/products`: catálogo paginado.
- `GET /v1/products/{id}`: consultar producto.

Las rutas `/v1` requieren JWT. El comercio viene del token verificado y las
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

## Ventas y migraciones

El esquema guarda clave única por comercio, hash de 32 bytes y detalle con nombre
y precio históricos. Las relaciones impiden mezclar comercios. Aún faltan la API,
la comparación del hash y los pagos; Go todavía no tiene permisos sobre ventas.

PostgreSQL calcula subtotales. La API deberá guardar todo en una transacción y
validar que haya líneas y que su suma coincida con el total; el esquema no lo exige.
Cantidad: 1–10000; importes: 1–1000000000000 centavos. Las claves no caducan.

Las migraciones son transaccionales, serializadas y repetibles. No se editan una
vez aplicadas; las correcciones usan otra migración. No hay reversión automática.

## Pruebas

El build comprueba formato, ejecuta `go vet` y `go test -race`. Para repetirlo:

```powershell
docker compose build --no-cache backend
```

Pruebas con PostgreSQL real: [integración](../tests/integration/README.md).
