-- Run after migrations 029-031 using a privileged test connection.
-- Verifies procurement tenant guards, idempotency uniqueness and narrow runtime grants.
DO $$
DECLARE
  rel text;
BEGIN
  FOREACH rel IN ARRAY ARRAY['purchase_orders','purchase_order_lines','purchase_receipts','purchase_receipt_lines'] LOOP
    IF NOT EXISTS (
      SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
      WHERE n.nspname=current_schema() AND c.relname=rel AND c.relrowsecurity AND c.relforcerowsecurity
    ) THEN
      RAISE EXCEPTION 'RLS/FORCE RLS missing on %', rel;
    END IF;
  END LOOP;

  IF NOT EXISTS (
    SELECT 1 FROM pg_indexes
    WHERE schemaname=current_schema() AND indexname='purchase_receipts_idempotency_uq'
  ) THEN
    RAISE EXCEPTION 'receipt idempotency unique index missing';
  END IF;

  IF NOT has_table_privilege('mekuin_app','purchase_orders','SELECT')
     OR NOT has_table_privilege('mekuin_app','purchase_orders','INSERT')
     OR has_table_privilege('mekuin_app','purchase_orders','UPDATE')
     OR has_table_privilege('mekuin_app','purchase_orders','DELETE') THEN
    RAISE EXCEPTION 'purchase_orders runtime privileges are broader or narrower than expected';
  END IF;

  IF NOT has_column_privilege('mekuin_app','purchase_orders','status','UPDATE')
     OR has_column_privilege('mekuin_app','purchase_orders','supplier_id','UPDATE') THEN
    RAISE EXCEPTION 'purchase_orders narrow update grants are incorrect';
  END IF;

  IF NOT has_column_privilege('mekuin_app','purchase_order_lines','received_quantity','UPDATE')
     OR has_column_privilege('mekuin_app','purchase_order_lines','ordered_quantity','UPDATE') THEN
    RAISE EXCEPTION 'purchase_order_lines narrow update grants are incorrect';
  END IF;

  IF NOT has_table_privilege('mekuin_app','inventory_movements','INSERT')
     OR has_table_privilege('mekuin_app','inventory_movements','UPDATE')
     OR has_table_privilege('mekuin_app','inventory_movements','DELETE') THEN
    RAISE EXCEPTION 'inventory movement ledger is not append-only for runtime role';
  END IF;
END $$;
