\set ON_ERROR_STOP on
DO $$
BEGIN
  IF to_regclass('public.platform_audit_logs') IS NULL THEN
    RAISE EXCEPTION 'platform_audit_logs table is missing';
  END IF;

  IF NOT has_table_privilege('mekuin_app','public.platform_audit_logs','SELECT')
     OR NOT has_table_privilege('mekuin_app','public.platform_audit_logs','INSERT') THEN
    RAISE EXCEPTION 'platform audit runtime read/insert privileges are missing';
  END IF;

  IF has_table_privilege('mekuin_app','public.platform_audit_logs','UPDATE')
     OR has_table_privilege('mekuin_app','public.platform_audit_logs','DELETE')
     OR has_table_privilege('mekuin_app','public.platform_audit_logs','TRUNCATE')
     OR has_table_privilege('mekuin_app','public.platform_audit_logs','REFERENCES')
     OR has_table_privilege('mekuin_app','public.platform_audit_logs','TRIGGER') THEN
    RAISE EXCEPTION 'platform audit history is mutable by the runtime role';
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_indexes
    WHERE schemaname='public' AND tablename='platform_audit_logs'
      AND indexname='platform_audit_logs_entity_time_idx'
  ) OR NOT EXISTS (
    SELECT 1 FROM pg_indexes
    WHERE schemaname='public' AND tablename='platform_audit_logs'
      AND indexname='platform_audit_logs_time_idx'
  ) THEN
    RAISE EXCEPTION 'platform audit query indexes are missing';
  END IF;
END $$;
