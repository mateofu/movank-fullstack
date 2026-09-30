BEGIN;

SET LOCAL lock_timeout = '10s';
SET LOCAL client_min_messages = warning;
SELECT pg_advisory_xact_lock(1297045070, 1);

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM public.schema_migrations WHERE version = 3) THEN
        RETURN;
    END IF;

    CREATE TABLE public.sales (
        merchant_id uuid NOT NULL REFERENCES public.merchants(id),
        id uuid NOT NULL DEFAULT gen_random_uuid(),
        idempotency_key text NOT NULL CHECK (idempotency_key ~ '^[A-Za-z0-9._:-]{1,128}$'),
        request_hash bytea NOT NULL CHECK (octet_length(request_hash) = 32),
        total_minor bigint NOT NULL CHECK (total_minor BETWEEN 1 AND 1000000000000),
        currency text NOT NULL CHECK (currency = 'COP'),
        created_at timestamptz NOT NULL DEFAULT now(),
        PRIMARY KEY (merchant_id, id),
        UNIQUE (merchant_id, idempotency_key)
    );

    CREATE TABLE public.sale_items (
        merchant_id uuid NOT NULL,
        sale_id uuid NOT NULL,
        product_id uuid NOT NULL,
        product_name text NOT NULL CHECK (char_length(product_name) BETWEEN 1 AND 120 AND product_name ~ '[^[:space:]]'),
        quantity integer NOT NULL CHECK (quantity BETWEEN 1 AND 10000),
        unit_price_minor bigint NOT NULL CHECK (unit_price_minor BETWEEN 1 AND 1000000000000),
        subtotal_minor bigint GENERATED ALWAYS AS (quantity::bigint * unit_price_minor) STORED,
        PRIMARY KEY (merchant_id, sale_id, product_id),
        FOREIGN KEY (merchant_id, sale_id) REFERENCES public.sales(merchant_id, id),
        FOREIGN KEY (merchant_id, product_id) REFERENCES public.products(merchant_id, id),
        CHECK (subtotal_minor BETWEEN 1 AND 1000000000000)
    );

    CREATE INDEX sale_items_product_idx ON public.sale_items (merchant_id, product_id);

    INSERT INTO public.schema_migrations (version, name) VALUES (3, 'sales');
END
$$;

COMMIT;
