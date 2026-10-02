$ErrorActionPreference = 'Stop'
Push-Location (Split-Path -Parent $PSScriptRoot)
try {
    $merchantId = 'b1000000-0000-4000-8000-000000000001'
    $userId = 'b1000000-0000-4000-8000-000000000002'
    $query = @"
BEGIN;
INSERT INTO public.merchants(id,name) VALUES('$merchantId','Tienda Demo') ON CONFLICT(id) DO NOTHING;
INSERT INTO public.products(merchant_id,sku,name,price_minor,currency) VALUES
('$merchantId','CAFE-01','Cafe americano',500000,'COP'),
('$merchantId','CAFE-02','Cappuccino',750000,'COP'),
('$merchantId','PAN-01','Croissant',650000,'COP'),
('$merchantId','COM-01','Sandwich de jamon',1400000,'COP'),
('$merchantId','BEB-01','Jugo de naranja',800000,'COP'),
('$merchantId','BEB-02','Agua mineral',350000,'COP')
ON CONFLICT(merchant_id,sku) DO NOTHING;
COMMIT;
"@
    & docker compose exec -T postgres psql -X -q -U movank -d movank -v ON_ERROR_STOP=1 -c $query
    if ($LASTEXITCODE -ne 0) { throw 'No se pudieron preparar los datos de ejemplo' }
    & docker compose exec -T backend /api token $merchantId $userId
    if ($LASTEXITCODE -ne 0) { throw 'No se pudo emitir el token de ejemplo' }
}
finally { Pop-Location }
