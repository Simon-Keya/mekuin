DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_class WHERE relname='branch_operational_settings' AND relrowsecurity AND relforcerowsecurity) THEN RAISE EXCEPTION 'branch settings must enforce RLS'; END IF;
 IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='branch_operational_settings' AND column_name='cash_variance_alert_threshold_minor') THEN RAISE EXCEPTION 'cash variance threshold missing'; END IF;
 IF NOT EXISTS (SELECT 1 FROM information_schema.table_privileges WHERE table_name='branch_operational_settings' AND grantee='mekuin_app' AND privilege_type='UPDATE') THEN RAISE EXCEPTION 'runtime update privilege missing'; END IF;
 IF EXISTS (SELECT 1 FROM information_schema.table_privileges WHERE table_name='branch_operational_settings' AND grantee='mekuin_app' AND privilege_type='DELETE') THEN RAISE EXCEPTION 'runtime DELETE must remain unavailable'; END IF;
END $$;
