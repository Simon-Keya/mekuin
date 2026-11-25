\set ON_ERROR_STOP on
DO $$
DECLARE t text; r boolean;
BEGIN
 FOREACH t IN ARRAY ARRAY['expense_categories','expenses','expense_review_events','expense_payments'] LOOP
  SELECT relrowsecurity AND relforcerowsecurity INTO r FROM pg_class WHERE oid=to_regclass('public.'||t);
  IF NOT COALESCE(r,false) THEN RAISE EXCEPTION 'expense table % must enforce RLS',t; END IF;
 END LOOP;
 IF NOT has_table_privilege('mekuin_app','public.expenses','SELECT') OR NOT has_table_privilege('mekuin_app','public.expenses','INSERT') OR NOT has_column_privilege('mekuin_app','public.expenses','status','UPDATE') THEN RAISE EXCEPTION 'expense runtime privileges missing'; END IF;
 IF has_table_privilege('mekuin_app','public.expenses','DELETE') OR has_column_privilege('mekuin_app','public.expenses','amount_minor','UPDATE') OR has_table_privilege('mekuin_app','public.expense_payments','UPDATE') OR has_table_privilege('mekuin_app','public.expense_payments','DELETE') OR has_table_privilege('mekuin_app','public.expense_review_events','UPDATE') OR has_table_privilege('mekuin_app','public.expense_review_events','DELETE') THEN RAISE EXCEPTION 'expense payment/review history must be append-only'; END IF;
 IF NOT has_table_privilege('mekuin_app','public.expense_payments','INSERT') OR NOT has_table_privilege('mekuin_app','public.expense_review_events','INSERT') THEN RAISE EXCEPTION 'expense event insert privilege missing'; END IF;
 IF NOT EXISTS(SELECT 1 FROM role_permissions WHERE role='manager' AND permission_key='expenses.approve') THEN RAISE EXCEPTION 'manager expense approval permission missing'; END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='public.expense_payments'::regclass AND conname LIKE '%check%') THEN RAISE EXCEPTION 'expense payment method constraints missing'; END IF;
END $$;
