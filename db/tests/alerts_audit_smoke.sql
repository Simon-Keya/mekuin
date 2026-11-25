\set ON_ERROR_STOP on
DO $$
DECLARE t text; r boolean;
BEGIN
 FOREACH t IN ARRAY ARRAY['operational_alerts','operational_alert_events'] LOOP
  SELECT relrowsecurity AND relforcerowsecurity INTO r FROM pg_class WHERE oid=to_regclass('public.'||t);
  IF NOT COALESCE(r,false) THEN RAISE EXCEPTION 'alert table % must enforce RLS',t; END IF;
 END LOOP;
 IF NOT has_table_privilege('mekuin_app','public.operational_alerts','SELECT') OR NOT has_table_privilege('mekuin_app','public.operational_alerts','INSERT') OR has_table_privilege('mekuin_app','public.operational_alerts','UPDATE') OR NOT has_column_privilege('mekuin_app','public.operational_alerts','status','UPDATE') THEN RAISE EXCEPTION 'alert runtime privileges missing'; END IF;
 IF has_table_privilege('mekuin_app','public.operational_alerts','DELETE') OR has_table_privilege('mekuin_app','public.operational_alert_events','UPDATE') OR has_table_privilege('mekuin_app','public.operational_alert_events','DELETE') THEN RAISE EXCEPTION 'alert event history must be append-only'; END IF;
 IF NOT has_table_privilege('mekuin_app','public.operational_alert_events','INSERT') THEN RAISE EXCEPTION 'alert event insert privilege missing'; END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_indexes WHERE schemaname='public' AND indexname='operational_alerts_active_source_uq') THEN RAISE EXCEPTION 'active alert deduplication index missing'; END IF;
 IF NOT EXISTS(SELECT 1 FROM role_permissions WHERE role='manager' AND permission_key='alerts.manage') OR NOT EXISTS(SELECT 1 FROM role_permissions WHERE role='manager' AND permission_key='audit.read') OR NOT EXISTS(SELECT 1 FROM role_permissions WHERE role='stock_controller' AND permission_key='alerts.read') THEN RAISE EXCEPTION 'alert/audit role permissions missing'; END IF;
 IF NOT has_table_privilege('mekuin_app','public.audit_logs','SELECT') THEN RAISE EXCEPTION 'audit ledger read privilege missing'; END IF;
 IF has_table_privilege('mekuin_app','public.audit_logs','UPDATE') OR has_table_privilege('mekuin_app','public.audit_logs','DELETE') THEN RAISE EXCEPTION 'audit ledger must remain append-only'; END IF;
END $$;
