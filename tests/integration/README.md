# Pruebas de integración

Aquí vivirán las pruebas que necesiten componentes reales o recorran la aplicación
completa. Las pruebas unitarias de Go vivirán junto a sus paquetes; las del frontend,
junto a su código.

Se probarán solicitudes concurrentes, reintentos, recuperación tras reinicios,
pagos inciertos, Redis caído, aislamiento entre comercios y sincronización offline.

Desde la raíz del proyecto:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File tests/integration/database.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File tests/integration/migrations.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File tests/integration/permissions.ps1
```

La primera comprueba caída y recuperación de PostgreSQL. La segunda comprueba
migraciones en una base temporal: rollback ante fallos, ejecuciones concurrentes,
reintentos sin pérdida de datos y restricciones de comercios.
La tercera requiere haber ejecutado `infra/migrate.ps1`: autentica por TCP como
`movank_app` y comprueba que pueda leer comercios pero no modificar datos ni esquema.
