# Pruebas de integración

Requieren Docker y las [migraciones aplicadas](../../README.md). Desde la raíz:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File infra/verify.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File tests/integration/database.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File tests/integration/migrations.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File tests/integration/permissions.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File tests/integration/auth.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File tests/integration/products.ps1
```

- `verify`: persistencia de PostgreSQL y caché descartable; recrea contenedores.
- `database`: caída y recuperación de PostgreSQL; lo detiene temporalmente.
- `migrations`: rollback, concurrencia, reintentos y restricciones en una base temporal;
  incluye aislamiento y precios históricos de ventas mediante `sales-schema.sql`.
- `permissions`: permisos del usuario de aplicación.
- `auth`: tokens y aislamiento entre comercios.
- `products`: validación, paginación, aislamiento y SKU único bajo concurrencia.

Las pruebas limpian sus datos temporales. Las HTTP aceptan `-BaseUrl` para otro puerto.
