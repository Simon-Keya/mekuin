\set ON_ERROR_STOP on
DO $$
DECLARE movement_check text; transfer_index boolean; reversal_index boolean; valuation_trigger boolean; snapshot_table boolean; count_status_check text;
BEGIN
 SELECT pg_get_constraintdef(oid) INTO movement_check FROM pg_constraint
  WHERE conrelid='public.inventory_movements'::regclass AND conname='inventory_movements_movement_type_check';
 IF movement_check IS NULL OR position('wastage' in movement_check)=0 OR position('breakage' in movement_check)=0 OR position('transfer_in' in movement_check)=0 OR position('transfer_out' in movement_check)=0 OR position('manual_adjustment' in movement_check)=0 OR position('sale_void_reversal' in movement_check)=0 OR position('sale_refund_reversal' in movement_check)=0 THEN
  RAISE EXCEPTION 'inventory movement types missing';
 END IF;
 SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE schemaname='public' AND indexname='inventory_transfer_movement_once_uq') INTO transfer_index;
 IF NOT transfer_index THEN RAISE EXCEPTION 'transfer duplicate protection index missing'; END IF;
 SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE schemaname='public' AND indexname='inventory_sale_reversal_once_uq') INTO reversal_index;
 IF NOT reversal_index THEN RAISE EXCEPTION 'sale inventory reversal duplicate protection index missing'; END IF;
 IF NOT has_table_privilege('mekuin_app','public.inventory_movements','SELECT') OR NOT has_table_privilege('mekuin_app','public.inventory_movements','INSERT') THEN
  RAISE EXCEPTION 'runtime inventory ledger privileges missing';
 END IF;
 IF has_table_privilege('mekuin_app','public.inventory_movements','UPDATE') OR has_table_privilege('mekuin_app','public.inventory_movements','DELETE') THEN
  RAISE EXCEPTION 'runtime role can mutate immutable inventory ledger';
 END IF;
 IF NOT has_column_privilege('mekuin_app','public.inventory_counts','count_mode','INSERT') OR NOT has_column_privilege('mekuin_app','public.inventory_counts','area','INSERT') THEN
  RAISE EXCEPTION 'runtime inventory count mode/area insert grants missing';
 END IF;
 SELECT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='public.inventory_movements'::regclass AND tgname='inventory_movement_apply_valuation' AND NOT tgisinternal) INTO valuation_trigger;
 IF NOT valuation_trigger OR to_regclass('public.inventory_valuation_balances') IS NULL OR to_regclass('public.inventory_valuation_movements') IS NULL THEN RAISE EXCEPTION 'weighted-average valuation ledger or trigger missing'; END IF;
 IF NOT has_table_privilege('mekuin_app','public.inventory_valuation_balances','SELECT') OR has_table_privilege('mekuin_app','public.inventory_valuation_balances','UPDATE') THEN RAISE EXCEPTION 'valuation balances must be readable but not directly mutable by runtime'; END IF;
 IF to_regclass('public.sale_recipe_consumption_snapshots') IS NULL THEN RAISE EXCEPTION 'historical recipe consumption snapshots missing'; END IF;
 SELECT pg_get_constraintdef(oid) INTO count_status_check FROM pg_constraint WHERE conrelid='public.inventory_counts'::regclass AND conname='inventory_counts_status_check';
 IF count_status_check IS NULL OR position('submitted' in count_status_check)=0 THEN RAISE EXCEPTION 'stock count submitted approval state missing'; END IF;
END $$;
