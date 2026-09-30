$ErrorActionPreference = 'Stop'
Push-Location (Split-Path -Parent (Split-Path -Parent $PSScriptRoot))
try {
    & docker compose up -d --build --wait --wait-timeout 120
    if ($LASTEXITCODE -ne 0) { throw 'No se pudo iniciar el backend' }
    & ./infra/migrate.ps1
    & docker build --target build -t movank-backend-tests ./backend
    if ($LASTEXITCODE -ne 0) { throw 'No se pudo preparar la imagen de pruebas' }
    $backendPath = Join-Path $PWD 'backend'
    & docker run --rm --network movank_default --env-file .env -e MOVANK_INTEGRATION=1 -e REDIS_ADDR=redis:6379 --mount "type=bind,source=$backendPath,target=/src" -w /src movank-backend-tests go test -race -count=1 -timeout=120s ./...
    if ($LASTEXITCODE -ne 0) { throw 'Fallaron las pruebas del backend' }
}
finally { Pop-Location }
