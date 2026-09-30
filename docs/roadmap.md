# Plan de trabajo

La etapa 1 está completada. La etapa 2 se divide: 2a prepara y verifica PostgreSQL
y Redis; el contenedor Go se integrará cuando exista el ejecutable.
Cada paso termina para revisión del usuario antes
de continuar. Las etapas grandes se dividirán en varios pasos equivalentes a commits.

| Etapa | Entrega prevista | Verificación principal |
| --- | --- | --- |
| 1. Estructura | Carpetas, alcance y decisiones iniciales | Inspección de archivos y coherencia con el PDF |
| 2. Infraestructura y configuración | Compose, configuración y herramientas fijadas | Validación de Compose y arranque de los componentes disponibles |
| 3. API Go y datos | Módulo, servidor, migraciones y OpenAPI inicial | Compilación, migraciones y ciclo de arranque/cierre |
| 4. Autenticación y productos | Token verificado, ámbito de comercio, catálogo | Tokens inválidos y acceso cruzado entre comercios |
| 5. Ventas e idempotencia | Transacciones y claves persistentes | Solicitudes concurrentes, conflicto de contenido y reintento tras reinicio |
| 6. Pagos y reconciliación | Intentos persistentes, simulador, UNKNOWN | Timeout, respuesta perdida, reconciliación fallida y recuperación sin doble cobro |
| 7. Outbox, dashboard, caché y SSE | Worker interno, agregados, fan-out | Crash/reentrega, carreras de caché, Redis caído y aislamiento SSE |
| 8. Interfaz y persistencia | SvelteKit, catálogo, carrito y comprobante | Persistencia tras cierre y separación de usuarios |
| 9. PWA y sincronización | Service Worker y Web Worker | Apertura offline, pestañas concurrentes y worker interrumpido |
| 10. Integración y entrega | Recorrido completo, documentación y respuestas técnicas | Ejecución reproducible de los casos críticos |

Pruebas unitarias e integración se incorporan junto al comportamiento que validan.
La etapa final reúne la demostración completa y verifica los comandos documentados.
El entorno inicial tiene Node y Docker CLI; Go no fue encontrado en PATH. Su
disponibilidad y la del motor Docker se resolverán al preparar la infraestructura.
