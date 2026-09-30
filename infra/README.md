# Infraestructura local

Compose levanta PostgreSQL, Redis y el backend Go con `/healthz` y `/readyz`.
Go se conecta a PostgreSQL. La primera migración crea comercios; no hay endpoints
de negocio todavía.

## Requisitos y configuración

Docker Engine con contenedores Linux y Compose v2 o posterior. En Windows,
Docker Desktop debe estar iniciado: `docker version` debe mostrar Client y Server.
Se necesita red para descargar imágenes la primera vez, pero no Go ni Node.

Las imágenes usan PostgreSQL `17.11-bookworm` y Redis `8.6.7-alpine3.23`.
Los tags fijan versiones, aunque pueden ser republicados: no son digests inmutables.

Desde la raíz, crea `.env` una sola vez:

```powershell
Copy-Item .env.example .env
notepad .env
```

Completa `POSTGRES_PASSWORD` con una contraseña local propia. Una cadena aleatoria
alfanumérica evita problemas de interpolación dotenv. No sobrescribas un `.env`
existente. Compose rechaza una contraseña vacía. `.env` está excluido de Git.

## Arrancar y comprobar

```powershell
docker compose config --quiet
docker compose up -d --build --wait --wait-timeout 120
powershell -NoProfile -ExecutionPolicy Bypass -File infra/migrate.ps1
docker compose ps
Invoke-RestMethod http://127.0.0.1:8080/healthz
Invoke-RestMethod http://127.0.0.1:8080/readyz
docker compose exec -T postgres psql -U movank -d movank -v ON_ERROR_STOP=1 -c "SELECT current_database(), current_user;"
docker compose exec -T redis redis-cli ping
powershell -NoProfile -ExecutionPolicy Bypass -File infra/verify.ps1
```

Se esperan tres servicios `healthy`, respuesta `status: ok`, base/usuario `movank`
y `PONG`. `pg_isready` mide
disponibilidad; la consulta comprueba ejecución SQL, no futuras migraciones.
Evita compartir `docker compose config` sin `--quiet`: muestra la contraseña.

El script crea una tabla con nombre aleatorio y una clave temporal, recrea ambos
contenedores y comprueba que PostgreSQL conserva la fila y Redis pierde la clave.
Elimina únicamente sus datos de prueba. Interrumpe las conexiones locales durante
la recreación: úsalo en este entorno de desarrollo.

## Conexiones

| Origen | PostgreSQL | Redis |
| --- | --- | --- |
| Tu equipo | `127.0.0.1:5432` | `127.0.0.1:6379` |
| Backend en Compose | `postgres:5432` | `redis:6379` (pendiente) |

Base y usuario: `movank`; contraseña: la de `.env`. Si un puerto está ocupado,
cambia `POSTGRES_PORT` o `REDIS_PORT`; los puertos internos no cambian. Solo se
publican en loopback, no en toda la red local. `API_PORT` cambia el puerto publicado
del backend (8080 por defecto); ajusta la URL de comprobación si lo cambias.

## Detener y recuperar

```powershell
docker compose stop redis
docker compose start redis
docker compose down
docker compose up -d --wait --wait-timeout 120
```

`down` conserva PostgreSQL. Añadir `--volumes` eliminaría sus datos y no forma
parte del flujo habitual. Cambiar `.env` no cambia la contraseña de una base ya
inicializada; las variables de la imagen se aplican en la primera inicialización.

## Decisiones y límites

- PostgreSQL persiste en `postgres_data`. Redis no guarda snapshots ni AOF;
  tiene un límite de 64 MB y expulsa claves por LRU al alcanzarlo.
- Redis no usa contraseña en este entorno local. Loopback y la red Docker limitan
  acceso; esta configuración no es para producción.
- El usuario PostgreSQL inicial es administrador y se usa temporalmente para
  comprobar conectividad. Los permisos de aplicación se separarán con las
  migraciones antes de incorporar operaciones de negocio.
- PostgreSQL y Redis arrancan independientemente. La futura API deberá tolerar
  Redis caído. La reconstrucción del dashboard se probará en la etapa 7.

El healthcheck del contenedor Go usa `/healthz`. Que Docker indique `healthy` no
garantiza disponibilidad de la base: comprueba `/readyz` para eso. No se exige que
PostgreSQL esté listo antes de arrancar Go; la recuperación se comprueba con
`tests/integration/database.ps1`.

Referencias: [imagen PostgreSQL](https://hub.docker.com/_/postgres),
[imagen Redis](https://hub.docker.com/_/redis),
[healthchecks de Compose](https://docs.docker.com/compose/how-tos/startup-order/).
