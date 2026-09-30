param([string] $BaseUrl = 'http://127.0.0.1:8080')

$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Net.Http

function Invoke-Compose {
    param([string[]] $ComposeArgs)
    $result = & docker compose @ComposeArgs
    if ($LASTEXITCODE -ne 0) { throw "docker compose fallo: $($ComposeArgs -join ' ')" }
    return $result
}

function Assert-Status {
    param([string] $Path, [int] $Expected)
    for ($attempt = 0; $attempt -lt 20; $attempt++) {
        $response = $client.GetAsync("$BaseUrl$Path").GetAwaiter().GetResult()
        try {
            $status = [int] $response.StatusCode
            if ($status -eq $Expected) {
                $body = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult() | ConvertFrom-Json
                $wanted = if ($Expected -eq 200) { 'ok' } else { 'unavailable' }
                if ($body.status -ne $wanted) { throw "Respuesta inesperada en $Path" }
                return
            }
        }
        finally { $response.Dispose() }
        Start-Sleep -Milliseconds 500
    }
    throw "${Path}: esperado $Expected, recibido $status"
}

Push-Location (Split-Path -Parent (Split-Path -Parent $PSScriptRoot))
$client = New-Object System.Net.Http.HttpClient
$client.Timeout = [TimeSpan]::FromSeconds(5)
try {
    Invoke-Compose -ComposeArgs @('up', '-d', '--build', '--wait', '--wait-timeout', '120')
    & ./infra/migrate.ps1
    Assert-Status '/readyz' 200
    $container = Invoke-Compose -ComposeArgs @('ps', '-q', 'backend')
    $started = & docker inspect --format '{{.State.StartedAt}}' $container
    if ($LASTEXITCODE -ne 0) { throw 'No se pudo consultar el backend' }
    try {
        Invoke-Compose -ComposeArgs @('stop', 'postgres')
        Assert-Status '/healthz' 200
        Assert-Status '/readyz' 503
    }
    finally {
        Invoke-Compose -ComposeArgs @('up', '-d', '--wait', '--wait-timeout', '120', 'postgres')
    }
    Assert-Status '/readyz' 200
    $restarted = & docker inspect --format '{{.State.StartedAt}}' $container
    if ($LASTEXITCODE -ne 0 -or $started -ne $restarted) { throw 'El backend se reinicio durante la prueba' }
    Write-Output 'OK: PostgreSQL caido devuelve 503; healthz sigue en 200 y la conexion se recupera sin reiniciar Go.'
}
finally {
    $client.Dispose()
    Pop-Location
}
