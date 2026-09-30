\getenv app_password APP_DB_PASSWORD
\setenv PGPASSWORD :app_password
\connect 'host=postgres port=5432 dbname=movank user=movank_app'

BEGIN;

DO $$
BEGIN
    IF current_user <> 'movank_app' THEN
        RAISE EXCEPTION 'Expected application user';
    END IF;
    IF EXISTS (
        SELECT 1 FROM pg_roles
        WHERE rolname = current_user
          AND (rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls)
    ) THEN
        RAISE EXCEPTION 'Application user has elevated privileges';
    END IF;

    PERFORM 1 FROM public.merchants LIMIT 1;

    BEGIN
        CREATE TABLE public.permission_probe (id integer);
        RAISE EXCEPTION 'Application user can create tables';
    EXCEPTION WHEN insufficient_privilege THEN
        NULL;
    END;

    BEGIN
        ALTER TABLE public.merchants ADD COLUMN permission_probe integer;
        RAISE EXCEPTION 'Application user can alter tables';
    EXCEPTION WHEN insufficient_privilege THEN
        NULL;
    END;

    BEGIN
        INSERT INTO public.merchants (name) VALUES ('Permission probe');
        RAISE EXCEPTION 'Application user can insert merchants';
    EXCEPTION WHEN insufficient_privilege THEN
        NULL;
    END;

    BEGIN
        UPDATE public.merchants SET name = name WHERE false;
        RAISE EXCEPTION 'Application user can update merchants';
    EXCEPTION WHEN insufficient_privilege THEN
        NULL;
    END;

    BEGIN
        DELETE FROM public.merchants WHERE false;
        RAISE EXCEPTION 'Application user can delete merchants';
    EXCEPTION WHEN insufficient_privilege THEN
        NULL;
    END;

    BEGIN
        DELETE FROM public.schema_migrations WHERE false;
        RAISE EXCEPTION 'Application user can modify migrations';
    EXCEPTION WHEN insufficient_privilege THEN
        NULL;
    END;

    BEGIN
        CREATE ROLE movank_permission_probe;
        RAISE EXCEPTION 'Application user can create roles';
    EXCEPTION WHEN insufficient_privilege THEN
        NULL;
    END;
END
$$;

ROLLBACK;
