# Requisitos de la prueba

Fuente leída: `Movank_FullStack_PruebaTecnica.pdf`, cinco páginas (la quinta sin
texto extraíble). Este documento resume requisitos; no afirma que estén implementados.

## Backend e infraestructura

- Un servicio Go, PostgreSQL y Redis/Dragonfly; elegimos Redis según lo solicitado.
- Compose debe levantar base, caché y backend.
- Endpoints mínimos:
  - `POST /v1/products`
  - `POST /v1/sales`
  - `POST /v1/sales/{id}/pay`, con `method: CARD` y escenario simulado.
  - `GET /v1/sales/{id}`
  - `GET /v1/dashboard/today`
  - `GET /v1/dashboard/stream` (SSE).
- Escenarios de pago: `APPROVED`, `DECLINED`, `TIMEOUT`. El último resulta en
  `UNKNOWN` y conserva la identidad y semántica del intento al reintentarlo.
- Venta pagada y registro de outbox en la misma transacción. Worker interno Go,
  activado por LISTEN/NOTIFY o stream, y distribución del agregado por SSE.
- Dashboard con caché reconstruible desde PostgreSQL al apagar Redis.
- Aislamiento por comercio desde token verificado, también a nivel de repositorio,
  outbox, caché y SSE. Sin secretos versionados.

## Frontend

- Una PWA Svelte/SvelteKit, instalable, con shell precacheado y sin dependencia de
  SSR por petición. Véase la condición de instalación inicial en las decisiones.
- Catálogo, carrito, cola de ventas y último dashboard en IndexedDB; no depender
  de memoria volátil ni localStorage para esos datos.
- Flujo offline completo: catálogo, carrito, pago, resultado y comprobante.
  El pago offline permanece `PENDING_SYNC`.
- Web Worker dedicado para sincronizar al recuperar red, recuperar intentos
  interrumpidos y evitar efectos duplicados incluso al reabrir pestañas.
- SSE online; dashboard persistido en IndexedDB cuando no hay red.
- Carrito persistente y separado por sesión de usuario en dispositivos compartidos.

## Aspectos que debemos poder defender

Backend: escrituras concurrentes de caché; cancelación y commit de transacciones;
reconciliación auditable de UNKNOWN; atomicidad del outbox frente a publicación
directa; consumo duplicado y recuperación tras crash; TTL y estampida de caché;
fan-out a 500 conexiones SSE sin polling de base por cliente; aislamiento entre
comercios en todas las capas.

Frontend: estrategia de precache; diferencias SPA/MPA/SSR/SSG; hidratación frente a
IndexedDB; persistencia y separación de usuarios; comunicación y recuperación del
Web Worker; cookies HttpOnly/Secure/SameSite y protección de sesión; composición
de Button, PrimaryButton y DangerButton; catálogo de 1 a 4 columnas según ancho
del contenedor.

## Entrega solicitada adicionalmente

Contrato OpenAPI, decisiones y limitaciones explícitas, comandos comprobados y
pruebas relevantes de concurrencia, reintentos, recuperación y aislamiento.
Desarrollo en pasos revisables; sin commits ni push automáticos.
