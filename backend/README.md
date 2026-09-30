# Backend Go

Esta etapa reserva la estructura; todavía no hay módulo Go ni código ejecutable.

- `cmd/api/`: ensamblaje y arranque del único servicio. El worker de outbox vivirá
  en este proceso, no en un segundo ejecutable.
- `internal/`: handlers, negocio y acceso a datos. Los paquetes concretos se
  crearán cuando tengan implementación, sin capas vacías ni framework DDD.
- `migrations/`: cambios versionados de PostgreSQL, junto al código que los usa.

La configuración se incorporará en la etapa 2; la API y el modelo, en la etapa 3.
