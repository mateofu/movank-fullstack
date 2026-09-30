# Movank — prueba técnica full stack

Un servicio Go y un frontend SvelteKit. PostgreSQL será la fuente de verdad;
Redis será una caché reconstruible. El backend alojará el worker de outbox y
distribuirá actualizaciones del dashboard por SSE. La PWA persistirá en IndexedDB
y sincronizará mediante un Web Worker.

## Estado actual

El entorno local incluye PostgreSQL, Redis y el arranque mínimo del servicio Go.
Consulta [infraestructura](infra/README.md) para configurar `.env`, arrancar y
verificar persistencia. El backend expone `GET /healthz`; todavía no hay
migraciones ni endpoints de negocio. Consulta [backend](backend/README.md)
para ejecución, configuración y pruebas.

```text
backend/
  cmd/api/         Entrada del único proceso Go
  internal/        Implementación privada del servicio
  migrations/      Evolución del esquema PostgreSQL
frontend/
  src/             Aplicación SvelteKit y workers
  static/          Recursos públicos e instalación PWA
infra/             Configuración de contenedores y desarrollo
docs/
  api/             Contrato OpenAPI, a incorporar con la API
tests/integration/ Pruebas entre componentes reales
```

## Ejecución

Prepara `.env` siguiendo la [guía de infraestructura](infra/README.md).
Después, desde la raíz:

```powershell
docker compose up -d --build --wait
Invoke-RestMethod http://127.0.0.1:8080/healthz
```

La respuesta esperada es `status: ok`. La construcción ejecuta las pruebas de Go.
`infra/verify.ps1` comprueba persistencia de PostgreSQL y caché descartable en Redis.

## Decisiones para las siguientes funcionalidades

Estas reglas guiarán la implementación; todavía no hay lógica de negocio:

- PostgreSQL guardará ventas, pagos y claves de idempotencia. Una clave repetida
  con distinto contenido será un conflicto. Las restricciones y transacciones
  resolverán solicitudes concurrentes. Redis será solo una caché reconstruible.
- El comercio saldrá del token verificado. Las consultas, la caché, el outbox y
  SSE mantendrán ese aislamiento, aunque alguien conozca el UUID de otra venta.
- `TIMEOUT` dejará el pago en `UNKNOWN`, nunca en `DECLINED`. El intento y su
  referencia se guardarán antes del cobro. La reconciliación consultará esa misma
  operación; si sigue siendo incierta, conservará `UNKNOWN` y un registro auditable.
  Reintentar no iniciará otro cobro ni cambiará el escenario del intento existente.
- El proveedor simulado deberá admitir idempotencia y consulta por referencia.
  Sin esas garantías, un cobro incierto requiere revisión, no otro cobro automático.
- Pago confirmado y outbox se guardarán en la misma transacción. Un worker dentro
  de Go usará `LISTEN/NOTIFY` como aviso y recuperará pendientes desde la tabla.
  Repetir un evento no duplicará agregados ni reemplazará datos nuevos por antiguos.
- IndexedDB conservará catálogo, carrito, cola y dashboard por comercio y usuario.
  Un Web Worker sincronizará con claves estables: podrá repetir solicitudes tras
  un cierre, pero el backend impedirá repetir sus efectos. El pago offline quedará
  en `PENDING_SYNC` hasta sincronizarse.
- La PWA necesitará una instalación inicial con red para descargar el shell y el
  catálogo. Después deberá abrir y operar offline, sin depender de SSR.

Las pruebas acompañarán cada funcionalidad: concurrencia, reintentos, recuperación
tras fallos y aislamiento entre comercios. El contrato OpenAPI se añadirá con la API.
