# Frontend SvelteKit

Esta etapa reserva `src/` y `static/`. El scaffold, manifiestos de dependencias y
comandos de desarrollo se incorporarán con la interfaz en la etapa 8.

La aplicación no dependerá de SSR por petición. IndexedDB almacenará catálogo,
carrito, cola y último dashboard, separados por comercio y usuario. El Service
Worker precacheará el shell; un Web Worker distinto ejecutará la sincronización.
Estos comportamientos son requisitos pendientes, no funcionalidades disponibles.
