param([string] $BaseUrl = 'http://127.0.0.1:8080')

$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Net.Http

function Invoke-AdminSQL {
    param([string] $Query)
    & docker compose exec -T postgres psql -X -q -U movank -d movank -v ON_ERROR_STOP=1 -c $Query
    if ($LASTEXITCODE -ne 0) { throw 'Fallo SQL en la prueba de ventas' }
}

function New-Request {
    param([string] $Method, [string] $Path, [string] $Token, [string] $Body, [string] $Key)
    $request = New-Object System.Net.Http.HttpRequestMessage (New-Object System.Net.Http.HttpMethod $Method), "$BaseUrl$Path"
    if ($Key) { $request.Headers.TryAddWithoutValidation('Idempotency-Key', $Key) | Out-Null }
    if ($Token) { $request.Headers.TryAddWithoutValidation('Authorization', "Bearer $Token") | Out-Null }
    $request.Headers.TryAddWithoutValidation('X-Merchant-ID', $merchantB) | Out-Null
    if ($Body) { $request.Content = New-Object System.Net.Http.StringContent $Body, ([System.Text.Encoding]::UTF8), 'application/json' }
    return $request
}

function Invoke-API {
    param([string] $Method, [string] $Path, [string] $Token, [int] $Status, [string] $Body = '', [string] $Key = '')
    $request = New-Request $Method $Path $Token $Body $Key
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
$created = $false
$redisStopped = $false
try {
    & docker compose up -d --build --wait --wait-timeout 120
    if ($LASTEXITCODE -ne 0) { throw 'No se pudo iniciar el backend' }
    & ./infra/migrate.ps1
    Invoke-AdminSQL "INSERT INTO public.merchants(id,name) VALUES('$merchantA','Payments A'),('$merchantB','Payments B');"
    $created = $true
    $tokenA = & docker compose exec -T backend /api token $merchantA $merchantA
    if ($LASTEXITCODE -ne 0) { throw 'Fallo token A' }
    $tokenB = & docker compose exec -T backend /api token $merchantB $merchantB
    if ($LASTEXITCODE -ne 0) { throw 'Fallo token B' }
    $product = Invoke-API 'POST' '/v1/products' $tokenA 201 '{"sku":"TEST","name":"Cafe","price_minor":10000,"currency":"COP"}'
    $body = @{ items = @(@{ product_id = $product.id; quantity = 2 }) } | ConvertTo-Json -Depth 4 -Compress
    $approved = Invoke-API 'POST' '/v1/sales' $tokenA 201 $body 'approved'
    $declined = Invoke-API 'POST' '/v1/sales' $tokenA 201 $body 'declined'
    $timeout = Invoke-API 'POST' '/v1/sales' $tokenA 201 $body 'timeout'
    Invoke-API 'POST' "/v1/sales/$($approved.id)/pay" '' 401 '{"method":"CARD","scenario":"APPROVED"}' | Out-Null
    Invoke-API 'POST' "/v1/sales/$($approved.id)/pay" $tokenB 404 '{"method":"CARD","scenario":"APPROVED"}' | Out-Null
    Invoke-API 'POST' "/v1/sales/$($approved.id)/pay" $tokenA 400 '{"method":"CARD","scenario":"APPROVED","merchant_id":"spoof"}' | Out-Null
    $paid = Invoke-API 'POST' "/v1/sales/$($approved.id)/pay" $tokenA 200 '{"method":"CARD","scenario":"APPROVED"}'
    if ($paid.status -ne 'APPROVED') { throw 'Pago no aprobado' }
    Invoke-API 'POST' "/v1/sales/$($approved.id)/pay" $tokenA 409 '{"method":"CARD","scenario":"DECLINED"}' | Out-Null
    $rejected = Invoke-API 'POST' "/v1/sales/$($declined.id)/pay" $tokenA 200 '{"method":"CARD","scenario":"DECLINED"}'
    if ($rejected.status -ne 'DECLINED') { throw 'Rechazo incorrecto' }
    $unknown = Invoke-API 'POST' "/v1/sales/$($timeout.id)/pay" $tokenA 202 '{"method":"CARD","scenario":"TIMEOUT"}'
    if ($unknown.status -ne 'UNKNOWN') { throw 'TIMEOUT no produjo UNKNOWN' }
    & docker compose restart backend
    if ($LASTEXITCODE -ne 0) { throw 'No se pudo reiniciar Go' }
    & docker compose up -d --wait --wait-timeout 120 backend
    if ($LASTEXITCODE -ne 0) { throw 'Go no se recupero' }
    $deadline = [DateTime]::UtcNow.AddSeconds(30)
    do {
        $resolved = Invoke-API 'GET' "/v1/sales/$($timeout.id)" $tokenA 200
        $dash = Invoke-API 'GET' '/v1/dashboard/today' $tokenA 200
        if ($resolved.payment.status -eq 'APPROVED' -and $dash.paid_sales -eq 2) { break }
        Start-Sleep -Milliseconds 300
    } while ([DateTime]::UtcNow -lt $deadline)
    if ($resolved.payment.status -ne 'APPROVED' -or $resolved.payment.id -ne $unknown.id -or $dash.paid_sales -ne 2 -or $dash.total_minor -ne 40000) { throw 'Fallo reconciliacion o dashboard' }
    $other = Invoke-API 'GET' '/v1/dashboard/today' $tokenB 200
    if ($other.paid_sales -ne 0) { throw 'Dashboard de otro comercio' }
    Invoke-API 'GET' "/v1/dashboard/today?merchant_id=$merchantA" $tokenB 400 | Out-Null
    & docker compose stop redis
    if ($LASTEXITCODE -ne 0) { throw 'No se pudo detener Redis' }
    $redisStopped = $true
    $fallback = Invoke-API 'GET' '/v1/dashboard/today' $tokenA 200
    if ($fallback.paid_sales -ne 2 -or $fallback.total_minor -ne 40000) { throw 'Fallo reconstruccion sin Redis' }
    Invoke-AdminSQL "DO `$`$ BEGIN IF (SELECT count(*) FROM public.provider_operations WHERE merchant_id='$merchantA') <> 3 OR (SELECT count(*) FROM public.outbox WHERE merchant_id='$merchantA') <> 2 THEN RAISE EXCEPTION 'Duplicate effects'; END IF; END `$`$;"
    Write-Output 'OK: pagos, UNKNOWN, recuperacion, aislamiento y dashboard con Redis apagado.'
}
finally {
    $client.Dispose()
    try {
        if ($redisStopped) {
            & docker compose up -d --wait --wait-timeout 120 redis
            if ($LASTEXITCODE -ne 0) { throw 'No se pudo restaurar Redis' }
        }
        if ($created) {
            Invoke-AdminSQL "DELETE FROM public.outbox WHERE merchant_id='$merchantA'; DELETE FROM public.payment_checks WHERE merchant_id='$merchantA'; DELETE FROM public.provider_operations WHERE merchant_id='$merchantA'; DELETE FROM public.payments WHERE merchant_id='$merchantA'; DELETE FROM public.sale_items WHERE merchant_id='$merchantA'; DELETE FROM public.sales WHERE merchant_id='$merchantA'; DELETE FROM public.products WHERE merchant_id='$merchantA'; DELETE FROM public.merchants WHERE id IN ('$merchantA','$merchantB');"
        }
    }
    finally { Pop-Location }
}
