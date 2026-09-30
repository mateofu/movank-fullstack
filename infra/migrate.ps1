$ErrorActionPreference = 'Stop'

Push-Location (Split-Path -Parent $PSScriptRoot)
try {
    $files = Get-ChildItem -LiteralPath 'backend/migrations' -Filter '*.sql' | Sort-Object Name
    foreach ($file in $files) {
        Write-Output "Aplicando $($file.Name)"
        & docker compose exec -T postgres psql -X -U movank -d movank -v ON_ERROR_STOP=1 -f "/migrations/$($file.Name)"
        if ($LASTEXITCODE -ne 0) { throw "Fallo la migracion $($file.Name)" }
    }
    Write-Output 'Configurando usuario de aplicacion'
    & docker compose exec -T postgres psql -X -U movank -d movank -v ON_ERROR_STOP=1 -f '/setup/configure-app.sql'
    if ($LASTEXITCODE -ne 0) { throw 'Fallo la configuracion del usuario de aplicacion' }
}
finally { Pop-Location }
