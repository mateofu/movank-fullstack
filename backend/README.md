# Backend Go

El servicio usa Go 1.26 y únicamente la biblioteca estándar en esta etapa.
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
Invoke-RestMethod http://127.0.0.1:8080/healthz
docker compose logs backend
```

`GET /healthz` devuelve HTTP 200 con `{"status":"ok"}`. Es una comprobación de
vida del proceso, no de disponibilidad de PostgreSQL ni Redis. No hay endpoints
de negocio, autenticación o conexiones a datos todavía.

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
go run ./cmd/api
```

Las pruebas cubren configuración inválida, rutas y cierre con una solicitud en
curso. El detector de carreras se ejecuta en Linux dentro del build, con el
compilador C disponible en la imagen de construcción. No se requiere instalar Go
en Windows. No hay `go.sum` porque aún no existen dependencias externas.

La imagen final no incluye shell ni compilador. `/api healthcheck` permite que
Docker compruebe el servidor sin instalar utilidades adicionales. El sistema de
archivos es de solo lectura y el contexto de build excluye archivos ajenos al código.

Referencia de la herramienta: [distribuciones oficiales de Go](https://go.dev/dl/).
