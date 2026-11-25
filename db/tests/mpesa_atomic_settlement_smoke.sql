-- Run as the migration/CI role after all migrations. Confirms the settlement
-- migration exposes only the exact column updates needed by the runtime role.
DO $$
BEGIN
 IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='sales' AND column_name='source_order_id') THEN
   RAISE EXCEPTION 'sales.source_order_id missing';
 END IF;
 IF NOT has_column_privilege('mekuin_app','public.sales','status','UPDATE') THEN
   RAISE EXCEPTION 'mekuin_app cannot complete a pending sale';
 END IF;
 IF NOT has_column_privilege('mekuin_app','public.sale_payments','status','UPDATE') THEN
   RAISE EXCEPTION 'mekuin_app cannot verify the M-Pesa tender';
 END IF;
 IF NOT has_column_privilege('mekuin_app','public.sale_payments','provider_reference','UPDATE') THEN
   RAISE EXCEPTION 'mekuin_app cannot store the provider receipt';
 END IF;
 IF has_table_privilege('mekuin_app','public.sale_payments','DELETE') THEN
   RAISE EXCEPTION 'mekuin_app must not delete payment history';
 END IF;
 IF has_table_privilege('mekuin_app','public.inventory_movements','UPDATE') OR has_table_privilege('mekuin_app','public.inventory_movements','DELETE') THEN
   RAISE EXCEPTION 'mekuin_app must not rewrite inventory movement history';
 END IF;
END $$;
