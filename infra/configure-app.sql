\getenv app_password APP_DB_PASSWORD
SELECT length(:'app_password') > 0 AS password_present \gset
\if :password_present
\else
    DO $$ BEGIN RAISE EXCEPTION 'APP_DB_PASSWORD is required'; END $$;
\endif

BEGIN;

SET LOCAL lock_timeout = '10s';
SET LOCAL client_min_messages = warning;
SELECT pg_advisory_xact_lock(1297045070, 1);

DO $$
BEGIN
    IF current_database() <> 'movank' THEN
        RAISE EXCEPTION 'Expected database movank';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'movank_app') THEN
        CREATE ROLE movank_app;
    END IF;
END
$$;

SELECT format(
    'ALTER ROLE movank_app WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD %L',
    :'app_password'
) \gexec

REVOKE ALL ON DATABASE movank FROM PUBLIC;
REVOKE ALL ON DATABASE movank FROM movank_app;
GRANT CONNECT ON DATABASE movank TO movank_app;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
REVOKE ALL ON SCHEMA public FROM movank_app;
GRANT USAGE ON SCHEMA public TO movank_app;
REVOKE ALL ON TABLE public.schema_migrations FROM PUBLIC, movank_app;
REVOKE ALL ON TABLE public.merchants FROM PUBLIC;
GRANT SELECT ON TABLE public.merchants TO movank_app;
GRANT SELECT, INSERT ON TABLE public.products TO movank_app;
GRANT SELECT, INSERT ON TABLE public.sales, public.sale_items TO movank_app;
GRANT SELECT, INSERT ON TABLE public.payments, public.provider_operations, public.payment_checks, public.outbox TO movank_app;
GRANT UPDATE (status, attempts, last_error, next_check_at, updated_at, completed_at) ON public.payments TO movank_app;
GRANT UPDATE (published_at) ON public.outbox TO movank_app;
GRANT USAGE ON SEQUENCE public.payment_checks_id_seq, public.outbox_id_seq TO movank_app;

COMMIT;
