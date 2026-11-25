\set ON_ERROR_STOP on
DO $$
DECLARE shifts_rls boolean; movements_rls boolean; unique_open boolean; cash_fk boolean; manager_permission boolean; cash_check boolean;
BEGIN
 SELECT relrowsecurity AND relforcerowsecurity INTO shifts_rls FROM pg_class WHERE oid='public.cash_shifts'::regclass;
 SELECT relrowsecurity AND relforcerowsecurity INTO movements_rls FROM pg_class WHERE oid='public.cash_shift_movements'::regclass;
 IF NOT shifts_rls OR NOT movements_rls THEN RAISE EXCEPTION 'cash shift tables must enforce RLS'; END IF;
 SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE schemaname='public' AND indexname='cash_shifts_one_open_user_branch_uq') INTO unique_open;
 IF NOT unique_open THEN RAISE EXCEPTION 'one-open-shift constraint missing'; END IF;
 SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='public.sale_payments'::regclass AND conname='sale_payments_cash_shift_fk') INTO cash_fk;
 IF NOT cash_fk THEN RAISE EXCEPTION 'cash payment shift attribution missing'; END IF;
 SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='public.sale_payments'::regclass AND conname='sale_payments_cash_requires_shift') INTO cash_check;
 IF NOT cash_check THEN RAISE EXCEPTION 'cash tenders must be attributed to shifts'; END IF;
 SELECT EXISTS(SELECT 1 FROM role_permissions WHERE role='manager' AND permission_key='shifts.own') INTO manager_permission;
 IF NOT manager_permission THEN RAISE EXCEPTION 'manager shift permission missing'; END IF;
 IF NOT has_table_privilege('mekuin_app','public.cash_shifts','SELECT') OR NOT has_table_privilege('mekuin_app','public.cash_shifts','INSERT') OR NOT has_table_privilege('mekuin_app','public.cash_shifts','UPDATE') THEN RAISE EXCEPTION 'cash shift runtime privileges missing'; END IF;
 IF has_table_privilege('mekuin_app','public.cash_shifts','DELETE') OR has_table_privilege('mekuin_app','public.cash_shift_movements','UPDATE') OR has_table_privilege('mekuin_app','public.cash_shift_movements','DELETE') THEN RAISE EXCEPTION 'cash shift history must be append-only'; END IF;
 IF NOT has_table_privilege('mekuin_app','public.cash_shift_movements','INSERT') THEN RAISE EXCEPTION 'cash movement insert privilege missing'; END IF;
END $$;
