param([string] $BaseUrl = 'http://127.0.0.1:8080')

$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Net.Http

function Invoke-AdminSQL {
    param([string] $Query)
    & docker compose exec -T postgres psql -X -q -U movank -d movank -v ON_ERROR_STOP=1 -c $Query
    if ($LASTEXITCODE -ne 0) { throw 'Fallo SQL en la prueba de productos' }
}

function New-Request {
    param([string] $Method, [string] $Path, [string] $Token, [string] $Body)
    $request = New-Object System.Net.Http.HttpRequestMessage (New-Object System.Net.Http.HttpMethod $Method), "$BaseUrl$Path"
    if ($Token) { $request.Headers.TryAddWithoutValidation('Authorization', "Bearer $Token") | Out-Null }
    $request.Headers.TryAddWithoutValidation('X-Merchant-ID', $merchantB) | Out-Null
    if ($Body) { $request.Content = New-Object System.Net.Http.StringContent $Body, ([System.Text.Encoding]::UTF8), 'application/json' }
    return $request
}

function Invoke-API {
    param([string] $Method, [string] $Path, [string] $Token, [int] $Status, [string] $Body = '')
    $request = New-Request $Method $Path $Token $Body
    try {
        $response = $client.SendAsync($request).GetAwaiter().GetResult()
        try {
            if ([int]$response.StatusCode -ne $Status) { throw "${Method} ${Path}: esperado $Status, recibido $([int]$response.StatusCode)" }
            return $response.Content.ReadAsStringAsync().GetAwaiter().GetResult() | ConvertFrom-Json
        }
        finally { $response.Dispose() }
    }
    finally { $request.Dispose() }
}

Push-Location (Split-Path -Parent (Split-Path -Parent $PSScriptRoot))
$client = New-Object System.Net.Http.HttpClient
$client.Timeout = [TimeSpan]::FromSeconds(15)
$merchantA = [Guid]::NewGuid().ToString()
$merchantB = [Guid]::NewGuid().ToString()
$userId = [Guid]::NewGuid().ToString()
$created = $false
$pending = @()
try {
    & docker compose up -d --build --wait --wait-timeout 120
    if ($LASTEXITCODE -ne 0) { throw 'No se pudo iniciar el proyecto' }
    & ./infra/migrate.ps1
    Invoke-AdminSQL "INSERT INTO public.merchants (id, name) VALUES ('$merchantA', 'Products A'), ('$merchantB', 'Products B');"
    $created = $true
    $tokenA = & docker compose exec -T backend /api token $merchantA $userId
    if ($LASTEXITCODE -ne 0) { throw 'Fallo token A' }
    $tokenB = & docker compose exec -T backend /api token $merchantB $userId
    if ($LASTEXITCODE -ne 0) { throw 'Fallo token B' }
    $body = '{"sku":" cafe-01 ","name":" Cafe ","price_minor":125050,"currency":"COP"}'
    Invoke-API 'POST' '/v1/products' '' 401 $body | Out-Null
    $a = Invoke-API 'POST' '/v1/products' $tokenA 201 $body
    if ($a.sku -ne 'CAFE-01' -or $a.name -ne 'Cafe' -or $a.price_minor -ne 125050) { throw 'Normalizacion de producto incorrecta' }
    Invoke-API 'POST' '/v1/products' $tokenA 409 $body | Out-Null
    $b = Invoke-API 'POST' '/v1/products' $tokenB 201 $body
    Invoke-API 'GET' "/v1/products/$($a.id)" $tokenB 404 | Out-Null
    Invoke-API 'GET' "/v1/products/$($b.id)" $tokenA 404 | Out-Null
    $listA = Invoke-API 'GET' '/v1/products' $tokenA 200
    $listB = Invoke-API 'GET' '/v1/products' $tokenB 200
    if ($listA.items.Count -ne 1 -or $listA.items[0].id -ne $a.id -or $listB.items.Count -ne 1 -or $listB.items[0].id -ne $b.id) { throw 'Catalogos mezclados' }
    Invoke-API 'GET' "/v1/products?merchant_id=$merchantB" $tokenA 400 | Out-Null
    $spoofed = @{ sku = 'SPOOF'; name = 'Bad'; price_minor = 100; currency = 'COP'; merchant_id = $merchantB } | ConvertTo-Json -Compress
    Invoke-API 'POST' '/v1/products' $tokenA 400 $spoofed | Out-Null
    Invoke-API 'POST' '/v1/products' $tokenA 400 '{"sku":"BAD","name":"Bad","price_minor":-1,"currency":"COP"}' | Out-Null
    Invoke-API 'POST' '/v1/products' $tokenA 400 '{"sku":"BAD","name":"Bad","price_minor":1.5,"currency":"COP"}' | Out-Null
    Invoke-API 'POST' '/v1/products' $tokenA 201 '{"sku":"SECOND","name":"Second","price_minor":100,"currency":"COP"}' | Out-Null
    $first = Invoke-API 'GET' '/v1/products?limit=1' $tokenA 200
    if ($first.items.Count -ne 1 -or -not $first.next_cursor) { throw 'Primera pagina incorrecta' }
    $second = Invoke-API 'GET' "/v1/products?limit=1&after=$($first.next_cursor)" $tokenA 200
    if ($second.items.Count -ne 1 -or $second.next_cursor -or $first.items[0].id -eq $second.items[0].id) { throw 'Segunda pagina incorrecta' }

    foreach ($attempt in 1..6) {
        $request = New-Request 'POST' '/v1/products' $tokenA '{"sku":"RACE","name":"Concurrent","price_minor":100,"currency":"COP"}'
        $pending += @{ Request = $request; Task = $client.SendAsync($request) }
    }
    $statuses = foreach ($operation in $pending) {
        $response = $operation.Task.GetAwaiter().GetResult()
        try { [int]$response.StatusCode }
        finally { $response.Dispose() }
    }
    if (@($statuses | Where-Object { $_ -eq 201 }).Count -ne 1 -or @($statuses | Where-Object { $_ -eq 409 }).Count -ne 5) { throw 'La concurrencia no produjo un unico producto' }
    & ./infra/migrate.ps1
    $listA = Invoke-API 'GET' '/v1/products' $tokenA 200
    if ($listA.items.Count -ne 3) { throw 'Se perdieron o duplicaron productos' }
    Write-Output 'OK: creacion, validacion, aislamiento por UUID, paginacion y SKU unico bajo concurrencia.'
}
finally {
    foreach ($operation in $pending) { $operation.Request.Dispose() }
    $client.Dispose()
    try {
        if ($created) {
            Invoke-AdminSQL "DELETE FROM public.products WHERE merchant_id IN ('$merchantA', '$merchantB'); DELETE FROM public.merchants WHERE id IN ('$merchantA', '$merchantB');"
        }
    }
    finally { Pop-Location }
}
