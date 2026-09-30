# Movank

Prueba full stack con Go, PostgreSQL, Redis y SvelteKit.

Implementado: autenticación JWT, productos por comercio y esquema de ventas.
Pendiente: API de ventas, pagos, outbox, dashboard SSE y PWA offline.

## Ejecutar

Configura `.env` siguiendo [infraestructura](infra/README.md). Desde la raíz:

```powershell
docker compose up -d --build --wait
powershell -NoProfile -ExecutionPolicy Bypass -File infra/migrate.ps1
Invoke-RestMethod http://127.0.0.1:8080/readyz
```

## Decisiones

- PostgreSQL será la fuente de verdad y guardará la idempotencia; Redis será caché.
- El comercio se obtiene del token verificado.
- `TIMEOUT` producirá `UNKNOWN`. Se guardará la referencia antes de cobrar y se
  consultará esa misma operación para reconciliar, sin iniciar otro cobro.
  El proveedor deberá soportar idempotencia y consulta; sin ellas, habrá revisión manual.
- Pago confirmado y outbox se guardarán juntos. El worker interno recuperará
  pendientes y procesará reintentos sin duplicar efectos; `LISTEN/NOTIFY` será un aviso.
- IndexedDB y un Web Worker permitirán sincronizar con claves estables por comercio
  y usuario. El pago offline quedará en `PENDING_SYNC`. La instalación inicial requiere red.

Consulta [backend](backend/README.md), [frontend](frontend/README.md),
[OpenAPI](docs/api/openapi.yaml) y [pruebas](tests/integration/README.md).
