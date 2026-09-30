BEGIN;
SET LOCAL lock_timeout = '10s';
SELECT pg_advisory_xact_lock(1297045070, 1);
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM public.schema_migrations WHERE version = 4) THEN RETURN; END IF;
    CREATE TABLE public.payments (
        merchant_id uuid NOT NULL,
        sale_id uuid NOT NULL,
        id uuid NOT NULL DEFAULT gen_random_uuid(),
        method text NOT NULL CHECK (method = 'CARD'),
        scenario text NOT NULL CHECK (scenario IN ('APPROVED','DECLINED','TIMEOUT')),
        status text NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','UNKNOWN','APPROVED','DECLINED')),
        amount_minor bigint NOT NULL CHECK (amount_minor BETWEEN 1 AND 1000000000000),
        attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
        last_error text,
        next_check_at timestamptz NOT NULL DEFAULT (now() + interval '5 seconds'),
        created_at timestamptz NOT NULL DEFAULT now(),
        updated_at timestamptz NOT NULL DEFAULT now(),
        completed_at timestamptz,
        PRIMARY KEY (merchant_id, sale_id),
        UNIQUE (id),
        UNIQUE (merchant_id, sale_id, id),
        FOREIGN KEY (merchant_id, sale_id) REFERENCES public.sales(merchant_id, id),
        CHECK ((status IN ('APPROVED','DECLINED')) = (completed_at IS NOT NULL))
    );
    CREATE INDEX payments_pending_idx ON public.payments(next_check_at) WHERE status IN ('PENDING','UNKNOWN');
    CREATE INDEX payments_dashboard_idx ON public.payments(merchant_id, completed_at) INCLUDE (amount_minor) WHERE status = 'APPROVED';
    CREATE TABLE public.provider_operations (
        reference uuid PRIMARY KEY REFERENCES public.payments(id),
        merchant_id uuid NOT NULL,
        sale_id uuid NOT NULL,
        amount_minor bigint NOT NULL,
        scenario text NOT NULL CHECK (scenario IN ('APPROVED','DECLINED','TIMEOUT')),
        outcome text NOT NULL CHECK (outcome IN ('APPROVED','DECLINED')),
        visible_at timestamptz NOT NULL,
        created_at timestamptz NOT NULL DEFAULT now(),
        FOREIGN KEY (merchant_id, sale_id, reference) REFERENCES public.payments(merchant_id, sale_id, id),
        UNIQUE (merchant_id, sale_id)
    );
    CREATE TABLE public.payment_checks (
        id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
        merchant_id uuid NOT NULL,
        sale_id uuid NOT NULL,
        status text NOT NULL CHECK (status IN ('UNKNOWN','APPROVED','DECLINED')),
        source text NOT NULL CHECK (source IN ('charge','lookup')),
        error text,
        created_at timestamptz NOT NULL DEFAULT now(),
        FOREIGN KEY (merchant_id, sale_id) REFERENCES public.payments(merchant_id, sale_id)
    );
    CREATE TABLE public.outbox (
        id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
        merchant_id uuid NOT NULL,
        sale_id uuid NOT NULL,
        created_at timestamptz NOT NULL DEFAULT now(),
        published_at timestamptz,
        UNIQUE (merchant_id, sale_id),
        FOREIGN KEY (merchant_id, sale_id) REFERENCES public.payments(merchant_id, sale_id)
    );
    CREATE INDEX outbox_pending_idx ON public.outbox(id) WHERE published_at IS NULL;
    INSERT INTO public.schema_migrations(version,name) VALUES (4,'payments');
END
$$;
COMMIT;
