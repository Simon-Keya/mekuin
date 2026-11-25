-- Run after migrations 033 and 034 in a disposable database.
DO $$
BEGIN
  IF to_regclass('public.supplier_invoice_review_events') IS NULL THEN
    RAISE EXCEPTION 'supplier_invoice_review_events table missing';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='supplier_invoice_review_events' AND column_name='event_no') THEN
    RAISE EXCEPTION 'review event sequence column missing';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE schemaname='public' AND indexname='supplier_invoice_review_latest_idx') THEN
    RAISE EXCEPTION 'latest review index missing';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_class WHERE oid='public.supplier_invoice_review_events'::regclass AND relrowsecurity AND relforcerowsecurity) THEN
    RAISE EXCEPTION 'review event RLS must be enabled and forced';
  END IF;
END $$;
