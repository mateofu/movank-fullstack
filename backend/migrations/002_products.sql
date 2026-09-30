BEGIN;

SET LOCAL lock_timeout = '10s';
SET LOCAL client_min_messages = warning;
SELECT pg_advisory_xact_lock(1297045070, 1);

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM public.schema_migrations WHERE version = 2) THEN
        RETURN;
    END IF;

    CREATE TABLE public.products (
        merchant_id uuid NOT NULL REFERENCES public.merchants(id),
        id uuid NOT NULL DEFAULT gen_random_uuid(),
        sku text NOT NULL CHECK (sku ~ '^[A-Z0-9][A-Z0-9._-]{0,63}$'),
        name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120 AND name ~ '[^[:space:]]'),
        price_minor bigint NOT NULL CHECK (price_minor BETWEEN 1 AND 1000000000000),
        currency text NOT NULL CHECK (currency = 'COP'),
        created_at timestamptz NOT NULL DEFAULT now(),
        PRIMARY KEY (merchant_id, id),
        UNIQUE (merchant_id, sku)
    );

    INSERT INTO public.schema_migrations (version, name) VALUES (2, 'products');
END
$$;

COMMIT;
