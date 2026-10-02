# Movank

Prueba full stack con Go, PostgreSQL, Redis y SvelteKit.

Implementado: JWT, productos, ventas idempotentes, pagos simulados, reconciliación,
outbox, worker y dashboard con Redis y SSE. La interfaz permite vender y conserva
el carrito en IndexedDB. Pendiente: instalación PWA y sincronización offline.

## Ejecutar

Configura `.env` siguiendo [infraestructura](infra/README.md). Desde la raíz:

```powershell
docker compose up -d --build --wait
powershell -NoProfile -ExecutionPolicy Bypass -File infra/migrate.ps1
Invoke-RestMethod http://127.0.0.1:8080/readyz
```

Interfaz: sigue los comandos de [frontend](frontend/README.md).

## Decisiones

- PostgreSQL guarda los datos y la idempotencia; Redis es una caché reconstruible.
- El comercio se obtiene del token verificado.
- `TIMEOUT` produce `UNKNOWN`. La reconciliación consulta la misma referencia;
  si no hay respuesta, conserva el estado y las comprobaciones. No crea otro cobro.
- Pago confirmado y outbox se guardan juntos. El worker recupera pendientes;
  `LISTEN/NOTIFY` es un aviso. Los eventos repetidos envían el agregado completo.
- IndexedDB y un Web Worker permitirán sincronizar con claves estables por comercio
  y usuario. El pago offline quedará en `PENDING_SYNC`. La instalación inicial requiere red.

Consulta [backend](backend/README.md), [frontend](frontend/README.md),
[OpenAPI](docs/api/openapi.yaml) y [pruebas](tests/integration/README.md).
