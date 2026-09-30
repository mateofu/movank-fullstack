# Backend Go

El servicio usa Go 1.26 y pgx para conectarse a PostgreSQL.
Docker compila con Go 1.26.8 y ejecuta un binario estático como usuario sin privilegios.

- `cmd/api/`: ensamblaje y arranque del único servicio. El worker de outbox vivirá
  en este proceso, no en un segundo ejecutable.
- `internal/`: handlers, negocio y acceso a datos. Los paquetes concretos se
  crearán cuando tengan implementación, sin capas vacías ni framework DDD.
- `migrations/`: cambios versionados de PostgreSQL, junto al código que los usa.

## Ejecución

Desde la raíz, con el `.env` preparado según [infraestructura](../infra/README.md):

```powershell
docker compose up -d --build --wait --wait-timeout 120
powershell -NoProfile -ExecutionPolicy Bypass -File infra/migrate.ps1
Invoke-RestMethod http://127.0.0.1:8080/healthz
Invoke-RestMethod http://127.0.0.1:8080/readyz
docker compose logs backend
```

`GET /healthz` devuelve HTTP 200 con `{"status":"ok"}`. Es una comprobación de
vida del proceso, no de disponibilidad de PostgreSQL ni Redis. Productos y ventas
siguen pendientes.

`GET /readyz` consulta PostgreSQL: devuelve 200 con `{"status":"ok"}` si responde
y 503 con `{"status":"unavailable"}` si falla o tarda más de un segundo.
No verifica tablas ni migraciones. Ambas rutas son públicas y no exponen errores
internos. El contrato está en [OpenAPI](../docs/api/openapi.yaml).

Compose proporciona `PGHOST`, `PGPORT`, `PGDATABASE`, `PGUSER`, `PGPASSWORD` y
`PGSSLMODE`. El puerto es opcional para pgx (5432); las otras variables son
obligatorias. En local se usa `PGSSLMODE=disable` dentro de Docker; un despliegue
remoto debe configurar TLS y sus certificados.

El pool abre conexiones según se necesitan, con un máximo de 10 y un plazo de
conexión de 3 segundos. La comprobación HTTP impone su límite más corto de un
segundo. Si PostgreSQL cae, Go permanece activo y el pool puede reconectar cuando
vuelva. El pool se cierra después de detener el servidor HTTP.

Go usa `movank_app`, con contraseña propia (`APP_DB_PASSWORD`) y solo lectura de
`merchants`. No puede crear tablas, cambiar el esquema ni modificar migraciones.
El usuario `movank` queda para administración y migraciones locales. Los permisos
de escritura se concederán por tabla cuando se implemente cada funcionalidad.
Esto separa responsabilidades, pero todavía no aísla filas entre comercios.

## Autenticación

`GET /v1/me` requiere `Authorization: Bearer TOKEN`. El JWT se verifica con
`golang-jwt/jwt`: HS256, emisor `movank-local`, destinatario `movank-api`, caducidad,
fecha de emisión y fecha de activación. `sub` identifica al usuario y `merchant_id`
al comercio, ambos con formato UUID. `AUTH_SIGNING_KEY` se configura según la
[guía de infraestructura](../infra/README.md).

La identidad verificada se guarda en el contexto de cada solicitud. El repositorio
consulta `merchants WHERE id = $1` con ese comercio. No toma el comercio del cuerpo,
la URL ni encabezados personalizados. Devuelve 401 si el token falla, 403 si el
comercio no existe y 503 si PostgreSQL no puede responder. No publica errores internos.

Para desarrollo, el operador puede crear un comercio y emitir un token de 15 minutos:

```powershell
$merchantId = [Guid]::NewGuid().ToString()
$userId = [Guid]::NewGuid().ToString()
docker compose exec -T postgres psql -U movank -d movank -v ON_ERROR_STOP=1 -c "INSERT INTO public.merchants (id, name) VALUES ('$merchantId', 'Demo');"
$token = docker compose exec -T backend /api token $merchantId $userId
Invoke-RestMethod http://127.0.0.1:8080/v1/me -Headers @{ Authorization = "Bearer $token" }
```

El comando `token` requiere acceso al contenedor, comprueba que el comercio existe
y escribe el token en stdout. No hay una ruta pública para emitir tokens. No los
guardes en archivos versionados, logs o localStorage.

Este emisor es local: el operador asigna el UUID del usuario; todavía no existe
registro de usuarios, login, renovación ni revocación individual de tokens. El
frontend tendrá su flujo de sesión con cookie HttpOnly. La firma acredita lo
emitido por el operador, no verifica credenciales de usuario. Esta etapa comprueba
aislamiento únicamente para la consulta `/v1/me`; productos, ventas y SSE deberán
aplicar el mismo ámbito al implementarse.

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File tests/integration/auth.ps1
```

La prueba crea dos comercios temporales, emite sus tokens y verifica que parámetros
manipulados no cambien de comercio. También rechaza tokens inválidos y un token cuyo
comercio se eliminó. Borra únicamente sus comercios de prueba al terminar.

## Migraciones

Después de levantar Compose, desde la raíz:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File infra/migrate.ps1
docker compose exec -T postgres psql -U movank -d movank -c "TABLE public.schema_migrations;"
```

El script aplica los archivos SQL por nombre y se detiene ante un error. Cada
archivo guarda sus cambios y su versión en una misma transacción. El bloqueo
transaccional evita que dos ejecuciones apliquen simultáneamente una migración.
Repetir el comando omite las versiones registradas y conserva los datos.
Después configura `movank_app` con `infra/configure-app.sql`, que usa la contraseña
del entorno y se puede volver a ejecutar. La configuración del usuario se mantiene
fuera de las migraciones versionadas porque depende de credenciales locales.

La primera migración crea `merchants`: UUID, nombre obligatorio de hasta 120
caracteres sin contar espacios exteriores y fecha de creación. No crea comercios
de ejemplo ni expone rutas nuevas. Las futuras tablas usarán el UUID del comercio
para sus relaciones; el aislamiento desde el token todavía está pendiente.

Las migraciones se ejecutan explícitamente, no al arrancar la API. Los archivos
aplicados no se editan: cada cambio tendrá un nuevo número y usará el mismo bloqueo.
Por ahora no hay checksum ni reversión automática de migraciones ya confirmadas;
las correcciones se harán con otra migración. `/readyz` sigue comprobando conexión,
no la versión del esquema.

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File tests/integration/migrations.ps1
```

La prueba usa una base temporal independiente: fuerza un error antes del commit,
comprueba rollback completo, ejecuta dos migraciones en paralelo y verifica que
repetirlas conserva los datos. También comprueba UUID único y nombre no vacío.
Elimina solo su base temporal al terminar.

Referencia: [bloqueos transaccionales de PostgreSQL](https://www.postgresql.org/docs/17/explicit-locking.html#ADVISORY-LOCKS).

`HTTP_PORT` configura el puerto del proceso (8080 por defecto). Compose fija el
interno en 8080 y permite cambiar el publicado mediante `API_PORT` en `.env`.
Puertos inválidos impiden el arranque. No se cargan archivos dotenv desde Go;
Compose suministra las variables al contenedor.

Al recibir SIGINT o SIGTERM, deja de aceptar conexiones y concede 10 segundos a
las solicitudes activas. Compose espera 15 segundos antes de forzar la terminación.
El contexto de cierre es independiente del contexto ya cancelado de la señal.
Las futuras conexiones SSE requerirán su propio mecanismo de cierre.

## Verificación

La construcción ejecuta comprobación de formato, `go vet` y `go test -race` antes
de compilar. Para repetirlas sin usar la caché de construcción:

```powershell
docker compose build --no-cache backend
```

Con Go 1.26 instalado, desde `backend/` también se puede ejecutar:

```powershell
go test ./...
go vet ./...
```

Para `go run ./cmd/api`, exporta primero las variables `PG*` anteriores apuntando
a `127.0.0.1` y al puerto publicado de PostgreSQL. Go no lee `.env` automáticamente.

Las pruebas cubren configuración inválida, rutas, errores de PostgreSQL, timeout
y cierre con una solicitud en curso. El detector de carreras se ejecuta dentro del build, con el
compilador C disponible en la imagen de construcción. No se requiere instalar Go
en Windows. `go.mod` y `go.sum` fijan las dependencias.

Para comprobar caída y recuperación con PostgreSQL real, desde la raíz:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File tests/integration/database.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File tests/integration/permissions.ps1
```

La prueba detiene PostgreSQL temporalmente, comprueba `/healthz` 200 y `/readyz`
503, lo levanta de nuevo y verifica recuperación sin reiniciar el backend. No
borra datos. Si cambiaste `API_PORT`, pasa `-BaseUrl http://127.0.0.1:TU_PUERTO`.
La prueba de permisos abre una conexión TCP autenticada como `movank_app` y
comprueba lectura permitida y operaciones prohibidas. Usa una transacción que
se revierte; no modifica los comercios.

La imagen final no incluye shell ni compilador. `/api healthcheck` permite que
Docker compruebe el servidor sin instalar utilidades adicionales. El sistema de
archivos es de solo lectura y el contexto de build excluye archivos ajenos al código.

Referencia de la herramienta: [distribuciones oficiales de Go](https://go.dev/dl/).
Driver: [pgxpool](https://pkg.go.dev/github.com/jackc/pgx/v5/pgxpool).
