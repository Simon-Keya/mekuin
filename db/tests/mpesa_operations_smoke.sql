DO $$
BEGIN
 IF to_regclass('public.mpesa_attempt_branch_recent_idx') IS NULL THEN
  RAISE EXCEPTION 'M-Pesa branch recent index missing';
 END IF;
 IF to_regclass('public.mpesa_attempt_branch_review_idx') IS NULL THEN
  RAISE EXCEPTION 'M-Pesa branch review index missing';
 END IF;
 IF NOT EXISTS (
  SELECT 1 FROM pg_class c JOIN pg_index i ON i.indexrelid=c.oid
  WHERE c.relname='mpesa_attempt_branch_review_idx' AND i.indisvalid
 ) THEN
  RAISE EXCEPTION 'M-Pesa review index is invalid';
 END IF;
END $$;
