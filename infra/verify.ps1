$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Invoke-Compose {
    param([string[]] $ComposeArgs)
    $result = & docker compose @ComposeArgs
    if ($LASTEXITCODE -ne 0) {
        throw "docker compose fallo: $($ComposeArgs -join ' ')"
    }
    return $result
}

Push-Location (Split-Path -Parent $PSScriptRoot)
$probe = 'infra_probe_' + [Guid]::NewGuid().ToString('N')
$created = $false
try {
    Invoke-Compose -ComposeArgs @('config', '--quiet')
    Invoke-Compose -ComposeArgs @('up', '-d', '--wait', '--wait-timeout', '120')
    Invoke-Compose -ComposeArgs @('exec', '-T', 'postgres', 'psql', '-U', 'movank', '-d', 'movank', '-v', 'ON_ERROR_STOP=1', '-c', "CREATE TABLE $probe (value integer NOT NULL); INSERT INTO $probe VALUES (42);")
    $created = $true
    $set = Invoke-Compose -ComposeArgs @('exec', '-T', 'redis', 'redis-cli', 'SET', $probe, '42', 'EX', '600')
    if (($set -join "`n").Trim() -ne 'OK') { throw 'Redis no confirmo la escritura.' }

    # Recrear contenedores verifica el volumen, no solo memoria de un proceso vivo.
    Invoke-Compose -ComposeArgs @('up', '-d', '--force-recreate', '--wait', '--wait-timeout', '120', 'postgres', 'redis')
    $value = Invoke-Compose -ComposeArgs @('exec', '-T', 'postgres', 'psql', '-U', 'movank', '-d', 'movank', '-v', 'ON_ERROR_STOP=1', '-tAc', "SELECT value FROM $probe;")
    if (($value -join "`n").Trim() -ne '42') { throw 'PostgreSQL no conservo la fila.' }
    $exists = Invoke-Compose -ComposeArgs @('exec', '-T', 'redis', 'redis-cli', 'EXISTS', $probe)
    if (($exists -join "`n").Trim() -ne '0') { throw 'Redis conservo una clave que debia ser descartable.' }
    Write-Output 'OK: PostgreSQL conserva datos y Redis es descartable tras recrear contenedores.'
}
finally {
    try {
        if ($created) {
            Invoke-Compose -ComposeArgs @('exec', '-T', 'postgres', 'psql', '-U', 'movank', '-d', 'movank', '-v', 'ON_ERROR_STOP=1', '-c', "DROP TABLE IF EXISTS $probe;")
            Invoke-Compose -ComposeArgs @('exec', '-T', 'redis', 'redis-cli', 'DEL', $probe)
        }
    }
    finally { Pop-Location }
}
