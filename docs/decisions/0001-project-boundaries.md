# 0001 — Límites del proyecto y garantías previstas

Estado: acordado como dirección de implementación; todavía no implementado.

## Organización

Monorepo con `backend/` y `frontend/`. Un solo proceso Go aloja API, reconciliación,
worker de outbox y distribución SSE. SvelteKit será una PWA con shell estático,
sin dependencia de SSR por petición. No se necesitan microservicios ni DDD puro.
Las dependencias y sus versiones se elegirán al incorporarlas y quedarán fijadas
en los manifiestos y archivos de bloqueo correspondientes.

## Datos e idempotencia

PostgreSQL es la autoridad. La idempotencia de ventas y pagos debe persistir allí,
con restricciones únicas por comercio y operación, y validación del contenido:
reutilizar una clave con otro contenido debe ser un conflicto. Redis nunca será
la única barrera contra duplicados. Las transacciones y restricciones deberán
resolver concurrencia entre solicitudes, no solo un mutex en memoria.

## Comercio confiable

El comercio se obtiene exclusivamente de un token verificado. Body, query o un
UUID conocido no otorgan acceso. Repositorios, claves de caché, eventos de outbox
y suscripciones SSE deben conservar el ámbito del comercio. El worker procesará
eventos de varios comercios con ámbito explícito por evento, sin mezclar destinos.
Las sesiones locales también deben separar usuarios de un mismo dispositivo.

## Incertidumbre de pagos

`TIMEOUT` produce `UNKNOWN`, nunca `DECLINED`. El intento y una referencia estable
del proveedor se persistirán antes de solicitar el cobro. Un reintento de la venta
no crea otro intento mientras exista uno incierto, ni cambia su escenario para
reinterpretarlo como aprobado o rechazado.

La reconciliación consultará el resultado usando esa misma referencia. Solo una
respuesta definitiva permitirá pasar a aprobado o rechazado. Si consultar falla
o sigue siendo ambiguo, permanecerá `UNKNOWN`, con historial e intentos auditables.
Un reinicio después de enviar el cobro también debe recuperar ese intento incierto.
Los nombres y transiciones completos se fijarán con el modelo de pagos.

Evitar cobros duplicados tras una respuesta perdida requiere que el proveedor
admita idempotencia duradera y/o consulta por referencia estable. El simulador
deberá modelar esa capacidad. Sin ella, no se puede prometer cobro exactamente una
vez: el caso incierto exige revisión, nunca un nuevo cobro automático a ciegas.

## Entrega de eventos y sincronización

Pago confirmado y outbox se escribirán en una misma transacción. Se prevé
`LISTEN/NOTIFY` como aviso al worker, con lectura durable y recuperación del outbox:
una notificación perdida no puede perder un evento. La entrega puede repetirse;
el consumo, reconstrucción de agregados y actualización de caché deben ser
idempotentes y evitar que una versión antigua reemplace una nueva.

La cola offline conservará claves estables en IndexedDB. No es posible garantizar
un único envío HTTP ante un cierre después de enviar y antes de guardar respuesta.
La garantía buscada es un único efecto persistente aunque haya varios envíos.
La coordinación entre pestañas y la recuperación del worker se definirán y probarán
en la etapa 9. Un pago offline es `PENDING_SYNC`, no una autorización de cobro.

## Límite del primer arranque offline

Un navegador sin una instalación previa no puede descargar una app sin red.
Se requiere una instalación inicial online que complete el precache; después,
incluso el primer arranque de la app instalada debe funcionar offline. El catálogo
disponible offline debe haberse aprovisionado también. Se documentará esta condición
en la entrega y se probará apertura con el navegador cerrado y la red desactivada.
