# Infraestructura local

Requiere Docker Desktop iniciado con contenedores Linux y Compose v2.
No necesitas instalar Go ni Node para ejecutar el backend.

## Configuración

Desde la raíz, solo si no existe `.env`:

```powershell
Copy-Item .env.example .env
notepad .env
```

Completa `POSTGRES_PASSWORD` y `APP_DB_PASSWORD` con contraseñas distintas.
Para `AUTH_SIGNING_KEY`, genera y copia esta clave hexadecimal:

```powershell
$bytes = New-Object byte[] 32
$rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
$rng.GetBytes($bytes)
$rng.Dispose()
[BitConverter]::ToString($bytes).Replace('-', '').ToLowerInvariant()
```

`.env` está excluido de Git. La clave debe tener al menos 32 bytes aleatorios.
Cambiarla y recrear el backend invalida los tokens anteriores.

## Uso

```powershell
docker compose up -d --build --wait
powershell -NoProfile -ExecutionPolicy Bypass -File infra/migrate.ps1
docker compose ps
Invoke-RestMethod http://127.0.0.1:8080/readyz
docker compose logs backend
docker compose down
```

Puertos locales: API `8080`, PostgreSQL `5432`, Redis `6379`. Se cambian con
`API_PORT`, `POSTGRES_PORT` y `REDIS_PORT`. Solo se publican en `127.0.0.1`.

## Límites

- PostgreSQL persiste en un volumen; `down` conserva datos, `down --volumes` los borra.
- Redis guarda el dashboard y es descartable. No tiene contraseña; configuración local.
- La base es `movank`. El administrador `movank` aplica migraciones;
  `movank_app` usa permisos limitados por tabla y columna para las operaciones de la API.
- Cambiar `POSTGRES_PASSWORD` en `.env` no modifica una base ya inicializada.
  Para cambiar `APP_DB_PASSWORD`, recrea los servicios y ejecuta las migraciones.
- `healthy` comprueba el proceso Go; `/readyz` comprueba PostgreSQL.

[Pruebas de integración](../tests/integration/README.md).
