$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Invoke-SQL {
    param([string] $Database, [string] $Query, [switch] $ExpectFailure)
    $previous = $ErrorActionPreference
    try {
        if ($ExpectFailure) { $ErrorActionPreference = 'Continue' }
        $output = @($Query | & docker compose exec -T postgres psql -X -qAt -U movank -d $Database -v ON_ERROR_STOP=1 -f - 2>&1)
        $code = $LASTEXITCODE
    }
    finally { $ErrorActionPreference = $previous }
    if ($ExpectFailure) {
        if ($code -ne 3) { throw "Se esperaba un error SQL; codigo recibido: $code" }
        return
    }
    if ($code -ne 0) { throw "Fallo SQL: $($output -join ' ')" }
    return ($output -join "`n").Trim()
}

function Assert-Value {
    param([string] $Query, [string] $Expected)
    $actual = Invoke-SQL -Database $testDatabase -Query $Query
    if ($actual -ne $Expected) { throw "Esperado: $Expected; recibido: $actual" }
}

$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
Push-Location $root
$testDatabase = 'movank_migrations_' + [Guid]::NewGuid().ToString('N')
$created = $false
$jobs = @()
try {
    & docker compose up -d --wait --wait-timeout 120 postgres
    if ($LASTEXITCODE -ne 0) { throw 'No se pudo iniciar PostgreSQL' }
    Invoke-SQL -Database 'movank' -Query "CREATE DATABASE $testDatabase;"
    $created = $true
    $migration = Get-Content -Raw -Encoding UTF8 'backend/migrations/001_merchants.sql'

    $broken = $migration.Replace('COMMIT;', "SELECT 1 / 0;`nCOMMIT;")
    Invoke-SQL -Database $testDatabase -Query $broken -ExpectFailure
    Assert-Value "SELECT to_regclass('public.merchants') IS NULL AND to_regclass('public.schema_migrations') IS NULL;" 't'

    $concurrentMigration = $migration.Replace('SELECT pg_advisory_xact_lock(1297045070, 1);', 'SELECT pg_advisory_xact_lock(1297045070, 1); SELECT pg_sleep(1);')
    foreach ($attempt in 1..2) {
        $jobs += Start-Job -ArgumentList $root, $testDatabase, $concurrentMigration -ScriptBlock {
            param($ProjectRoot, $Database, $Migration)
            Set-Location -LiteralPath $ProjectRoot
            $Migration | & docker compose exec -T postgres psql -X -qAt -U movank -d $Database -v ON_ERROR_STOP=1 -f -
            if ($LASTEXITCODE -ne 0) { throw 'Fallo una migracion concurrente' }
        }
    }
    $jobs | Wait-Job -Timeout 60 | Out-Null
    foreach ($job in $jobs) {
        if ($job.State -ne 'Completed') { throw "La migracion concurrente termino en $($job.State)" }
        Receive-Job -Job $job -ErrorAction Stop | Out-Null
    }
    Assert-Value 'SELECT count(*) FROM public.schema_migrations WHERE version = 1;' '1'

    Invoke-SQL -Database $testDatabase -Query "INSERT INTO public.merchants (id, name) VALUES ('00000000-0000-0000-0000-000000000001', 'Comercio de prueba');"
    Invoke-SQL -Database $testDatabase -Query $migration | Out-Null
    Assert-Value 'SELECT count(*) FROM public.merchants;' '1'
    Assert-Value 'SELECT count(*) FROM public.schema_migrations;' '1'
    Invoke-SQL -Database $testDatabase -Query "INSERT INTO public.merchants (name) VALUES ('   ');" -ExpectFailure
    Invoke-SQL -Database $testDatabase -Query "INSERT INTO public.merchants (id, name) VALUES ('00000000-0000-0000-0000-000000000001', 'Duplicado');" -ExpectFailure
    Assert-Value 'SELECT count(*) FROM public.merchants;' '1'
    $products = Get-Content -Raw -Encoding UTF8 'backend/migrations/002_products.sql'
    Invoke-SQL -Database $testDatabase -Query $products.Replace('COMMIT;', "SELECT 1 / 0;`nCOMMIT;") -ExpectFailure
    Assert-Value "SELECT to_regclass('public.products') IS NULL;" 't'
    Assert-Value 'SELECT count(*) FROM public.schema_migrations;' '1'
    Invoke-SQL -Database $testDatabase -Query $products | Out-Null
    Invoke-SQL -Database $testDatabase -Query $products | Out-Null
    Assert-Value 'SELECT count(*) FROM public.schema_migrations;' '2'
    Invoke-SQL -Database $testDatabase -Query "INSERT INTO public.products (merchant_id, sku, name, price_minor, currency) VALUES ('00000000-0000-0000-0000-000000000001', 'BAD', 'Bad price', -1, 'COP');" -ExpectFailure
    Invoke-SQL -Database $testDatabase -Query "INSERT INTO public.products (merchant_id, sku, name, price_minor, currency) VALUES ('00000000-0000-0000-0000-000000000002', 'BAD', 'Missing merchant', 100, 'COP');" -ExpectFailure
    Write-Output 'OK: rollback, concurrencia y reintentos de migraciones; restricciones de comercios y productos.'
}
finally {
    try {
        foreach ($job in $jobs) {
            if ($job.State -eq 'Running') { Stop-Job -Job $job }
            Remove-Job -Job $job -Force
        }
        if ($created) {
            if ($testDatabase -notmatch '^movank_migrations_[a-f0-9]{32}$') { throw 'Nombre de base de prueba invalido' }
            Invoke-SQL -Database 'movank' -Query "DROP DATABASE $testDatabase;"
        }
    }
    finally { Pop-Location }
}
