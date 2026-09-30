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
$userId = [Guid]::NewGuid().ToString()
$created = $false
$faultInstalled = $false
$faultName = 'sales_failure_' + [Guid]::NewGuid().ToString('N')
$pending = @()
try {
    & docker compose up -d --build --wait --wait-timeout 120
    if ($LASTEXITCODE -ne 0) { throw 'No se pudo iniciar el proyecto' }
    & ./infra/migrate.ps1
    Invoke-AdminSQL "INSERT INTO public.merchants (id, name) VALUES ('$merchantA', 'Sales A'), ('$merchantB', 'Sales B');"
    $created = $true
    $tokenA = & docker compose exec -T backend /api token $merchantA $userId
    if ($LASTEXITCODE -ne 0) { throw 'Fallo token A' }
    $tokenB = & docker compose exec -T backend /api token $merchantB $userId
    if ($LASTEXITCODE -ne 0) { throw 'Fallo token B' }
    $product = '{"sku":"TEST","name":"Cafe","price_minor":125050,"currency":"COP"}'
    $a = Invoke-API 'POST' '/v1/products' $tokenA 201 $product
    $b = Invoke-API 'POST' '/v1/products' $tokenB 201 $product
    $body = @{ items = @(@{ product_id = $a.id; quantity = 2 }) } | ConvertTo-Json -Depth 4 -Compress
    $bodyB = @{ items = @(@{ product_id = $b.id; quantity = 2 }) } | ConvertTo-Json -Depth 4 -Compress
    Invoke-API 'POST' '/v1/sales' '' 401 $body 'same-key' | Out-Null
    Invoke-API 'POST' '/v1/sales' $tokenA 400 $body | Out-Null
    $spoofed = @{ items = @(@{ product_id = $a.id; quantity = 2 }); merchant_id = $merchantB } | ConvertTo-Json -Depth 4 -Compress
    Invoke-API 'POST' '/v1/sales' $tokenA 400 $spoofed 'spoof' | Out-Null
    Invoke-API 'POST' "/v1/sales?merchant_id=$merchantB" $tokenA 400 $body 'spoof' | Out-Null
    $first = Invoke-API 'POST' '/v1/sales' $tokenA 201 $body 'same-key'
    if ($first.total_minor -ne 250100 -or $first.items.Count -ne 1) { throw 'Total o detalle incorrecto' }
    $replay = Invoke-API 'POST' '/v1/sales' $tokenA 201 $body 'same-key'
    if (($first | ConvertTo-Json -Depth 5 -Compress) -ne ($replay | ConvertTo-Json -Depth 5 -Compress)) { throw 'Reintento diferente' }
    $changed = $body.Replace(':2', ':3')
    Invoke-API 'POST' '/v1/sales' $tokenA 409 $changed 'same-key' | Out-Null
    $other = Invoke-API 'POST' '/v1/sales' $tokenB 201 $bodyB 'same-key'
    if ($other.id -eq $first.id) { throw 'Clave compartida entre comercios' }
    Invoke-API 'GET' "/v1/sales/$($first.id)" $tokenB 404 | Out-Null
    Invoke-API 'GET' "/v1/sales/$($other.id)" $tokenA 404 | Out-Null
    Invoke-API 'POST' '/v1/sales' $tokenA 400 $bodyB 'retry-invalid' | Out-Null
    Invoke-API 'POST' '/v1/sales' $tokenA 201 $body 'retry-invalid' | Out-Null
    Invoke-API 'POST' '/v1/sales' $tokenA 400 '{"items":[]}' 'empty' | Out-Null
    Invoke-API 'POST' '/v1/sales' $tokenA 400 ($body.Replace(':2', ':0')) 'zero' | Out-Null
    Invoke-API 'POST' '/v1/sales' $tokenA 400 ($body.Replace(':2', ':1.5')) 'fraction' | Out-Null
    Invoke-AdminSQL "UPDATE public.products SET price_minor=1000000000000 WHERE merchant_id='$merchantA';"
    Invoke-API 'POST' '/v1/sales' $tokenA 400 $body 'excessive-total' | Out-Null
    Invoke-AdminSQL "UPDATE public.products SET price_minor=125050 WHERE merchant_id='$merchantA';"

    foreach ($attempt in 1..6) {
        $request = New-Request 'POST' '/v1/sales' $tokenA $body 'concurrent-key'
        $pending += @{ Request = $request; Task = $client.SendAsync($request) }
    }
    $ids = foreach ($operation in $pending) {
        $response = $operation.Task.GetAwaiter().GetResult()
        try {
            if ([int]$response.StatusCode -ne 201) { throw 'Fallo creacion concurrente' }
            ($response.Content.ReadAsStringAsync().GetAwaiter().GetResult() | ConvertFrom-Json).id
        }
        finally { $response.Dispose() }
    }
    if (@($ids | Select-Object -Unique).Count -ne 1) { throw 'Ventas duplicadas bajo concurrencia' }
    Invoke-AdminSQL "BEGIN; CREATE FUNCTION public.$faultName() RETURNS trigger LANGUAGE plpgsql AS '`nBEGIN RAISE EXCEPTION ''Injected failure''; END'; CREATE TRIGGER $faultName BEFORE INSERT ON public.sale_items FOR EACH ROW WHEN (NEW.merchant_id = '$merchantA'::uuid) EXECUTE FUNCTION public.$faultName(); COMMIT;"
    $faultInstalled = $true
    Invoke-API 'POST' '/v1/sales' $tokenA 503 $body 'retry-failure' | Out-Null
    Invoke-AdminSQL "DO `$`$ BEGIN IF EXISTS (SELECT 1 FROM public.sales WHERE merchant_id='$merchantA' AND idempotency_key='retry-failure') THEN RAISE EXCEPTION 'Partial sale persisted'; END IF; END `$`$;"
    Invoke-AdminSQL "DROP TRIGGER $faultName ON public.sale_items; DROP FUNCTION public.$faultName();"
    $faultInstalled = $false
    Invoke-API 'POST' '/v1/sales' $tokenA 201 $body 'retry-failure' | Out-Null
    Invoke-AdminSQL "UPDATE public.products SET name='Updated', price_minor=100 WHERE merchant_id='$merchantA';"
    & docker compose restart backend
    if ($LASTEXITCODE -ne 0) { throw 'No se pudo reiniciar Go' }
    & docker compose up -d --wait --wait-timeout 120 backend
    if ($LASTEXITCODE -ne 0) { throw 'Go no se recupero' }
    $recovered = Invoke-API 'POST' '/v1/sales' $tokenA 201 $body 'same-key'
    if (($first | ConvertTo-Json -Depth 5 -Compress) -ne ($recovered | ConvertTo-Json -Depth 5 -Compress)) { throw 'Reintento perdido tras reinicio o cambio de precio' }
    $get = Invoke-API 'GET' "/v1/sales/$($first.id)" $tokenA 200
    if ($get.total_minor -ne 250100 -or $get.items[0].product_name -ne 'Cafe') { throw 'Se modifico el historico' }
    Invoke-AdminSQL "DO `$`$ BEGIN IF (SELECT count(*) FROM public.sales WHERE merchant_id='$merchantA') <> 4 THEN RAISE EXCEPTION 'Duplicate sales'; END IF; END `$`$;"
    Write-Output 'OK: ventas, reintentos, conflictos, concurrencia, aislamiento y recuperacion tras reiniciar Go.'
}
finally {
    foreach ($operation in $pending) { $operation.Request.Dispose() }
    $client.Dispose()
    try {
        if ($faultInstalled) { Invoke-AdminSQL "DROP TRIGGER $faultName ON public.sale_items; DROP FUNCTION public.$faultName();" }
        if ($created) {
            Invoke-AdminSQL "DELETE FROM public.sale_items WHERE merchant_id IN ('$merchantA', '$merchantB'); DELETE FROM public.sales WHERE merchant_id IN ('$merchantA', '$merchantB'); DELETE FROM public.products WHERE merchant_id IN ('$merchantA', '$merchantB'); DELETE FROM public.merchants WHERE id IN ('$merchantA', '$merchantB');"
        }
    }
    finally { Pop-Location }
}
