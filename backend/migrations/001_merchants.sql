BEGIN;

SET LOCAL lock_timeout = '10s';
SET LOCAL client_min_messages = warning;
SELECT pg_advisory_xact_lock(1297045070, 1);

CREATE TABLE IF NOT EXISTS public.schema_migrations (
    version integer PRIMARY KEY CHECK (version > 0),
    name text NOT NULL,
    applied_at timestamptz NOT NULL DEFAULT now()
);

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM public.schema_migrations WHERE version = 1) THEN
        RETURN;
    END IF;

    CREATE TABLE public.merchants (
        id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
        name text NOT NULL CHECK (char_length(btrim(name)) BETWEEN 1 AND 120),
        created_at timestamptz NOT NULL DEFAULT now()
    );

    INSERT INTO public.schema_migrations (version, name)
    VALUES (1, 'merchants');
END
$$;

COMMIT;
