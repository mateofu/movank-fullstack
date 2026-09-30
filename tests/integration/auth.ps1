param([string] $BaseUrl = 'http://127.0.0.1:8080')

$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Net.Http

function Invoke-AdminSQL {
    param([string] $Query)
    & docker compose exec -T postgres psql -X -q -U movank -d movank -v ON_ERROR_STOP=1 -c $Query
    if ($LASTEXITCODE -ne 0) { throw 'Fallo la preparacion de comercios de prueba' }
}

function Assert-Identity {
    param([string] $Token, [int] $Status, [string] $MerchantId, [string] $UserId, [string] $SpoofedId)
    $request = New-Object System.Net.Http.HttpRequestMessage ([System.Net.Http.HttpMethod]::Get), "$BaseUrl/v1/me?merchant_id=$SpoofedId"
    if ($Token) { $request.Headers.TryAddWithoutValidation('Authorization', "Bearer $Token") | Out-Null }
    $request.Headers.TryAddWithoutValidation('X-Merchant-ID', $SpoofedId) | Out-Null
    try {
        $response = $client.SendAsync($request).GetAwaiter().GetResult()
        try {
            if ([int]$response.StatusCode -ne $Status) { throw "Estado inesperado: $([int]$response.StatusCode), esperado: $Status" }
            if ($Status -eq 200) {
                $body = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult() | ConvertFrom-Json
                if ($body.merchant.id -ne $MerchantId -or $body.user_id -ne $UserId) { throw 'La identidad no coincide con el token' }
            }
        }
        finally { $response.Dispose() }
    }
    finally { $request.Dispose() }
}

Push-Location (Split-Path -Parent (Split-Path -Parent $PSScriptRoot))
$client = New-Object System.Net.Http.HttpClient
$client.Timeout = [TimeSpan]::FromSeconds(10)
$merchantA = [Guid]::NewGuid().ToString()
$merchantB = [Guid]::NewGuid().ToString()
$userA = [Guid]::NewGuid().ToString()
$userB = [Guid]::NewGuid().ToString()
$created = $false
try {
    & docker compose up -d --build --wait --wait-timeout 120
    if ($LASTEXITCODE -ne 0) { throw 'No se pudo iniciar el proyecto' }
    & ./infra/migrate.ps1
    Invoke-AdminSQL "INSERT INTO public.merchants (id, name) VALUES ('$merchantA', 'Auth test A'), ('$merchantB', 'Auth test B');"
    $created = $true
    $tokenA = & docker compose exec -T backend /api token $merchantA $userA
    if ($LASTEXITCODE -ne 0) { throw 'No se pudo emitir el token A' }
    $tokenB = & docker compose exec -T backend /api token $merchantB $userB
    if ($LASTEXITCODE -ne 0) { throw 'No se pudo emitir el token B' }
    Assert-Identity '' 401 '' '' $merchantB
    Assert-Identity 'invalid' 401 '' '' $merchantB
    Assert-Identity $tokenA 200 $merchantA $userA $merchantB
    Assert-Identity $tokenB 200 $merchantB $userB $merchantA
    Invoke-AdminSQL "DELETE FROM public.merchants WHERE id = '$merchantA';"
    Assert-Identity $tokenA 403 '' '' $merchantB
    Write-Output 'OK: tokens verificados, comercios separados, parametros manipulados ignorados y comercio eliminado rechazado.'
}
finally {
    try {
        if ($created) { Invoke-AdminSQL "DELETE FROM public.merchants WHERE id IN ('$merchantA', '$merchantB');" }
    }
    finally {
        $client.Dispose()
        Pop-Location
    }
}
