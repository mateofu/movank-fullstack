BEGIN;

DO $$
DECLARE
    merchant_a uuid := '00000000-0000-0000-0000-000000000001';
    merchant_b uuid := '00000000-0000-0000-0000-000000000002';
    product_a uuid;
    product_b uuid;
    sale_a uuid;
    sale_b uuid;
BEGIN
    INSERT INTO public.merchants (id, name) VALUES (merchant_b, 'Second merchant');
    INSERT INTO public.products (merchant_id, sku, name, price_minor, currency)
        VALUES (merchant_a, 'TEST', 'Original name', 125050, 'COP') RETURNING id INTO product_a;
    INSERT INTO public.products (merchant_id, sku, name, price_minor, currency)
        VALUES (merchant_b, 'TEST', 'Other product', 200, 'COP') RETURNING id INTO product_b;
    INSERT INTO public.sales (merchant_id, idempotency_key, request_hash, total_minor, currency)
        VALUES (merchant_a, 'same-key', decode(repeat('ab', 32), 'hex'), 250100, 'COP') RETURNING id INTO sale_a;
    INSERT INTO public.sales (merchant_id, idempotency_key, request_hash, total_minor, currency)
        VALUES (merchant_b, 'same-key', decode(repeat('ab', 32), 'hex'), 200, 'COP') RETURNING id INTO sale_b;
    INSERT INTO public.sale_items (merchant_id, sale_id, product_id, product_name, quantity, unit_price_minor)
        VALUES (merchant_a, sale_a, product_a, 'Original name', 2, 125050);

    BEGIN
        INSERT INTO public.sales (merchant_id, idempotency_key, request_hash, total_minor, currency)
            VALUES (merchant_a, 'same-key', decode(repeat('cd', 32), 'hex'), 100, 'COP');
        RAISE EXCEPTION 'Duplicate key accepted';
    EXCEPTION WHEN unique_violation THEN NULL;
    END;
    BEGIN
        INSERT INTO public.sale_items (merchant_id, sale_id, product_id, product_name, quantity, unit_price_minor)
            VALUES (merchant_a, sale_a, product_b, 'Foreign product', 1, 200);
        RAISE EXCEPTION 'Foreign product accepted';
    EXCEPTION WHEN foreign_key_violation THEN NULL;
    END;
    BEGIN
        INSERT INTO public.sale_items (merchant_id, sale_id, product_id, product_name, quantity, unit_price_minor)
            VALUES (merchant_b, sale_a, product_b, 'Foreign sale', 1, 200);
        RAISE EXCEPTION 'Foreign sale accepted';
    EXCEPTION WHEN foreign_key_violation THEN NULL;
    END;
    BEGIN
        UPDATE public.sale_items SET quantity = 0 WHERE sale_id = sale_a;
        RAISE EXCEPTION 'Zero quantity accepted';
    EXCEPTION WHEN check_violation THEN NULL;
    END;
    BEGIN
        UPDATE public.sale_items SET unit_price_minor = -1 WHERE sale_id = sale_a;
        RAISE EXCEPTION 'Negative price accepted';
    EXCEPTION WHEN check_violation THEN NULL;
    END;
    BEGIN
        UPDATE public.sale_items SET quantity = 10000, unit_price_minor = 1000000000000 WHERE sale_id = sale_a;
        RAISE EXCEPTION 'Excessive subtotal accepted';
    EXCEPTION WHEN check_violation THEN NULL;
    END;
    BEGIN
        UPDATE public.sales SET request_hash = decode('ab', 'hex') WHERE id = sale_a;
        RAISE EXCEPTION 'Invalid hash accepted';
    EXCEPTION WHEN check_violation THEN NULL;
    END;
    BEGIN
        UPDATE public.sales SET total_minor = 0 WHERE id = sale_a;
        RAISE EXCEPTION 'Zero total accepted';
    EXCEPTION WHEN check_violation THEN NULL;
    END;
    BEGIN
        UPDATE public.sales SET currency = 'USD' WHERE id = sale_a;
        RAISE EXCEPTION 'Other currency accepted';
    EXCEPTION WHEN check_violation THEN NULL;
    END;
    BEGIN
        UPDATE public.sales SET idempotency_key = '' WHERE id = sale_a;
        RAISE EXCEPTION 'Empty key accepted';
    EXCEPTION WHEN check_violation THEN NULL;
    END;
    BEGIN
        INSERT INTO public.sales (merchant_id, idempotency_key, request_hash, total_minor, currency)
            VALUES (merchant_a, 'rollback-key', decode(repeat('ab', 32), 'hex'), 100, 'COP');
        INSERT INTO public.sale_items (merchant_id, sale_id, product_id, product_name, quantity, unit_price_minor)
            VALUES (merchant_a, sale_a, product_b, 'Invalid item', 1, 100);
        RAISE EXCEPTION 'Invalid transaction accepted';
    EXCEPTION WHEN foreign_key_violation THEN NULL;
    END;
    IF EXISTS (SELECT 1 FROM public.sales WHERE idempotency_key = 'rollback-key') THEN
        RAISE EXCEPTION 'Failed transaction retained its key';
    END IF;

    UPDATE public.products SET name = 'New name', price_minor = 300 WHERE id = product_a;
    IF NOT EXISTS (SELECT 1 FROM public.sale_items WHERE sale_id = sale_a
        AND product_name = 'Original name' AND unit_price_minor = 125050 AND subtotal_minor = 250100) THEN
        RAISE EXCEPTION 'Sale snapshot changed';
    END IF;
END
$$;

COMMIT;
