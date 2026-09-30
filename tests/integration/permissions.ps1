$ErrorActionPreference = 'Stop'

Push-Location (Split-Path -Parent (Split-Path -Parent $PSScriptRoot))
try {
    $invalidPassword = 'invalid-' + [Guid]::NewGuid().ToString('N')
    $previous = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        $failure = & docker compose exec -T -e "PGPASSWORD=$invalidPassword" postgres psql -X -w -h postgres -U movank_app -d movank -c 'SELECT 1' 2>&1
        $code = $LASTEXITCODE
    }
    finally { $ErrorActionPreference = $previous }
    if ($code -ne 2 -or ($failure -join ' ') -notmatch 'password authentication failed') {
        throw 'No se verifico el rechazo de una contrasena incorrecta'
    }
    Get-Content -Raw -Encoding UTF8 'tests/integration/permissions.sql' |
        & docker compose exec -T postgres psql -X -U movank -d movank -v ON_ERROR_STOP=1 -f -
    if ($LASTEXITCODE -ne 0) { throw 'Fallo la prueba de permisos' }
    Write-Output 'OK: autenticacion TCP como movank_app, lectura permitida y operaciones administrativas rechazadas.'
}
finally { Pop-Location }
