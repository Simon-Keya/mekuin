\set ON_ERROR_STOP on
DO $$
BEGIN
 IF to_regclass('public.business_integration_settings') IS NULL THEN RAISE EXCEPTION 'business integration settings table missing'; END IF;
 IF NOT has_table_privilege('mekuin_app','public.business_integration_settings','SELECT') OR NOT has_table_privilege('mekuin_app','public.business_integration_settings','INSERT') THEN RAISE EXCEPTION 'runtime integration settings read/write grants missing'; END IF;
 IF has_table_privilege('mekuin_app','public.business_integration_settings','DELETE') THEN RAISE EXCEPTION 'runtime can delete integration settings'; END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_policies WHERE schemaname='public' AND tablename='business_integration_settings' AND policyname='business_integration_settings_tenant_policy') THEN RAISE EXCEPTION 'integration settings tenant RLS policy missing'; END IF;
END $$;
