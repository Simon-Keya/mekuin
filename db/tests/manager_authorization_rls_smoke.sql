-- Run as migration/database owner after goose up. Fixtures roll back.
\set ON_ERROR_STOP on
BEGIN;
INSERT INTO businesses(id,name) VALUES
 ('91000000-0000-4000-8000-000000000001','Approval Tenant A'),
 ('91000000-0000-4000-8000-000000000002','Approval Tenant B');
INSERT INTO app_users(id,email,full_name,password_hash) VALUES
 ('92000000-0000-4000-8000-000000000001','requester-a@example.test','Requester A','unused'),
 ('92000000-0000-4000-8000-000000000002','manager-a@example.test','Manager A','unused'),
 ('92000000-0000-4000-8000-000000000003','requester-b@example.test','Requester B','unused');
INSERT INTO branches(id,business_id,name) VALUES
 ('93000000-0000-4000-8000-000000000001','91000000-0000-4000-8000-000000000001','A Main'),
 ('93000000-0000-4000-8000-000000000002','91000000-0000-4000-8000-000000000002','B Main');
INSERT INTO memberships(id,business_id,user_id,role,status) VALUES
 ('94000000-0000-4000-8000-000000000001','91000000-0000-4000-8000-000000000001','92000000-0000-4000-8000-000000000001','cashier','active'),
 ('94000000-0000-4000-8000-000000000002','91000000-0000-4000-8000-000000000001','92000000-0000-4000-8000-000000000002','manager','active'),
 ('94000000-0000-4000-8000-000000000003','91000000-0000-4000-8000-000000000002','92000000-0000-4000-8000-000000000003','cashier','active');
INSERT INTO membership_branches(business_id,membership_id,branch_id) VALUES
 ('91000000-0000-4000-8000-000000000001','94000000-0000-4000-8000-000000000001','93000000-0000-4000-8000-000000000001'),
 ('91000000-0000-4000-8000-000000000001','94000000-0000-4000-8000-000000000002','93000000-0000-4000-8000-000000000001');
INSERT INTO staff_profiles(business_id,membership_id,display_name,manager_pin_hash)
VALUES ('91000000-0000-4000-8000-000000000001','94000000-0000-4000-8000-000000000002','Manager A','$2a$10$placeholderhash');
INSERT INTO manager_action_approvals(business_id,branch_id,requester_user_id,approver_membership_id,action,token_hash,expires_at)
VALUES
 ('91000000-0000-4000-8000-000000000001','93000000-0000-4000-8000-000000000001','92000000-0000-4000-8000-000000000001','94000000-0000-4000-8000-000000000002','sales.void',decode(repeat('a',64),'hex'),now()+interval '2 minutes'),
 ('91000000-0000-4000-8000-000000000001','93000000-0000-4000-8000-000000000001','92000000-0000-4000-8000-000000000001','94000000-0000-4000-8000-000000000002','sales.refund',decode(repeat('e',64),'hex'),now()-interval '1 second'),
 ('91000000-0000-4000-8000-000000000001','93000000-0000-4000-8000-000000000001','92000000-0000-4000-8000-000000000001','94000000-0000-4000-8000-000000000002','sales.refund',decode(repeat('f',64),'hex'),now()+interval '2 minutes');

SET LOCAL ROLE mekuin_app;
SELECT set_config('app.business_id','91000000-0000-4000-8000-000000000001',true);
DO $$ BEGIN
 IF (SELECT count(*) FROM manager_action_approvals) <> 3 THEN RAISE EXCEPTION 'approval tenant read isolation failed'; END IF;
END $$;
DO $$ BEGIN
 BEGIN
  INSERT INTO manager_action_approvals(business_id,branch_id,requester_user_id,approver_membership_id,action,token_hash,expires_at)
  VALUES ('91000000-0000-4000-8000-000000000002','93000000-0000-4000-8000-000000000002','92000000-0000-4000-8000-000000000003','94000000-0000-4000-8000-000000000003','sales.void',decode(repeat('b',64),'hex'),now()+interval '2 minutes');
  RAISE EXCEPTION 'cross-tenant approval insert unexpectedly succeeded';
 EXCEPTION WHEN insufficient_privilege THEN NULL;
 END;
END $$;
DO $$ BEGIN
 BEGIN
  INSERT INTO manager_action_approvals(business_id,branch_id,requester_user_id,approver_membership_id,action,token_hash,expires_at)
  VALUES ('91000000-0000-4000-8000-000000000001','93000000-0000-4000-8000-000000000002','92000000-0000-4000-8000-000000000001','94000000-0000-4000-8000-000000000002','sales.void',decode(repeat('c',64),'hex'),now()+interval '2 minutes');
  RAISE EXCEPTION 'cross-tenant branch approval unexpectedly succeeded';
 EXCEPTION WHEN foreign_key_violation THEN NULL;
 END;
END $$;
DO $$ BEGIN
 BEGIN
  INSERT INTO manager_action_approvals(business_id,branch_id,requester_user_id,approver_membership_id,action,token_hash,expires_at)
  VALUES ('91000000-0000-4000-8000-000000000001','93000000-0000-4000-8000-000000000001','92000000-0000-4000-8000-000000000001','94000000-0000-4000-8000-000000000002','unrestricted.admin',decode(repeat('d',64),'hex'),now()+interval '2 minutes');
  RAISE EXCEPTION 'unsupported approval action unexpectedly succeeded';
 EXCEPTION WHEN check_violation THEN NULL;
 END;
END $$;
-- Runtime may consume an approval but cannot rewrite its security scope.
DO $$ BEGIN
 BEGIN
  UPDATE manager_action_approvals SET action='sales.refund'
  WHERE token_hash=decode(repeat('a',64),'hex');
  RAISE EXCEPTION 'runtime unexpectedly changed approval action';
 EXCEPTION WHEN insufficient_privilege THEN NULL;
 END;
 BEGIN
  UPDATE manager_action_approvals SET expires_at=now()+interval '1 day'
  WHERE token_hash=decode(repeat('a',64),'hex');
  RAISE EXCEPTION 'runtime unexpectedly extended approval expiry';
 EXCEPTION WHEN insufficient_privilege THEN NULL;
 END;
 BEGIN
  UPDATE manager_action_approvals SET token_hash=decode(repeat('9',64),'hex')
  WHERE token_hash=decode(repeat('a',64),'hex');
  RAISE EXCEPTION 'runtime unexpectedly replaced approval token digest';
 EXCEPTION WHEN insufficient_privilege THEN NULL;
 END;
 BEGIN
  UPDATE manager_action_approvals SET business_id='91000000-0000-4000-8000-000000000002'
  WHERE token_hash=decode(repeat('a',64),'hex');
  RAISE EXCEPTION 'runtime unexpectedly changed approval tenant';
 EXCEPTION WHEN insufficient_privilege THEN NULL;
 END;
 BEGIN
  UPDATE manager_action_approvals SET branch_id='93000000-0000-4000-8000-000000000002'
  WHERE token_hash=decode(repeat('a',64),'hex');
  RAISE EXCEPTION 'runtime unexpectedly changed approval branch';
 EXCEPTION WHEN insufficient_privilege THEN NULL;
 END;
 BEGIN
  UPDATE manager_action_approvals SET requester_user_id='92000000-0000-4000-8000-000000000003'
  WHERE token_hash=decode(repeat('a',64),'hex');
  RAISE EXCEPTION 'runtime unexpectedly changed approval requester';
 EXCEPTION WHEN insufficient_privilege THEN NULL;
 END;
 BEGIN
  UPDATE manager_action_approvals SET target_id='95000000-0000-4000-8000-000000000001'
  WHERE token_hash=decode(repeat('a',64),'hex');
  RAISE EXCEPTION 'runtime unexpectedly changed approval target';
 EXCEPTION WHEN insufficient_privilege THEN NULL;
 END;
 BEGIN
  UPDATE manager_action_approvals SET target_amount_minor=999
  WHERE token_hash=decode(repeat('a',64),'hex');
  RAISE EXCEPTION 'runtime unexpectedly changed approval amount binding';
 EXCEPTION WHEN insufficient_privilege THEN NULL;
 END;
 BEGIN
  UPDATE manager_action_approvals SET approver_membership_id='94000000-0000-4000-8000-000000000001'
  WHERE token_hash=decode(repeat('a',64),'hex');
  RAISE EXCEPTION 'runtime unexpectedly changed approval approver';
 EXCEPTION WHEN insufficient_privilege THEN NULL;
 END;
 BEGIN
  DELETE FROM manager_action_approvals WHERE token_hash=decode(repeat('a',64),'hex');
  RAISE EXCEPTION 'runtime unexpectedly deleted an approval';
 EXCEPTION WHEN insufficient_privilege THEN NULL;
 END;
END $$;
-- A matching, unexpired approval is consumed exactly once.
DO $$ DECLARE n integer; BEGIN
 UPDATE manager_action_approvals SET consumed_at=now()
 WHERE token_hash=decode(repeat('a',64),'hex') AND business_id='91000000-0000-4000-8000-000000000001'
   AND branch_id='93000000-0000-4000-8000-000000000001' AND requester_user_id='92000000-0000-4000-8000-000000000001'
   AND action='sales.void' AND consumed_at IS NULL AND expires_at>now();
 GET DIAGNOSTICS n = ROW_COUNT;
 IF n <> 1 THEN RAISE EXCEPTION 'valid approval was not consumed'; END IF;
 UPDATE manager_action_approvals SET consumed_at=now()
 WHERE token_hash=decode(repeat('a',64),'hex') AND consumed_at IS NULL AND expires_at>now();
 GET DIAGNOSTICS n = ROW_COUNT;
 IF n <> 0 THEN RAISE EXCEPTION 'approval replay unexpectedly succeeded'; END IF;
END $$;
-- Expired and wrong-action approvals must not be consumable.
DO $$ DECLARE n integer; BEGIN
 UPDATE manager_action_approvals SET consumed_at=now()
 WHERE token_hash=decode(repeat('e',64),'hex') AND action='sales.refund' AND consumed_at IS NULL AND expires_at>now();
 GET DIAGNOSTICS n = ROW_COUNT;
 IF n <> 0 THEN RAISE EXCEPTION 'expired approval unexpectedly consumed'; END IF;
 UPDATE manager_action_approvals SET consumed_at=now()
 WHERE token_hash=decode(repeat('e',64),'hex') AND action='sales.void' AND consumed_at IS NULL AND expires_at>now();
 GET DIAGNOSTICS n = ROW_COUNT;
 IF n <> 0 THEN RAISE EXCEPTION 'expired approval unexpectedly consumed under another action'; END IF;
 -- This second fixture is unexpired: its refusal proves action binding rather
 -- than merely relying on the expiry predicate.
 UPDATE manager_action_approvals SET consumed_at=now()
 WHERE token_hash=decode(repeat('f',64),'hex') AND action='sales.void' AND consumed_at IS NULL AND expires_at>now();
 GET DIAGNOSTICS n = ROW_COUNT;
 IF n <> 0 THEN RAISE EXCEPTION 'unexpired approval was consumed for the wrong action'; END IF;
 UPDATE manager_action_approvals SET consumed_at=now()
 WHERE token_hash=decode(repeat('f',64),'hex') AND action='sales.refund' AND consumed_at IS NULL AND expires_at>now();
 GET DIAGNOSTICS n = ROW_COUNT;
 IF n <> 1 THEN RAISE EXCEPTION 'unexpired approval could not be consumed for its bound action'; END IF;
END $$;
-- Failed PIN counters are ordinary tenant-scoped state and must persist when
-- the caller commits the verification-failure response rather than rolling back.
UPDATE staff_profiles SET manager_pin_failed_attempts=1, manager_pin_window_started_at=now()
WHERE business_id='91000000-0000-4000-8000-000000000001' AND membership_id='94000000-0000-4000-8000-000000000002';
DO $$ BEGIN
 IF (SELECT manager_pin_failed_attempts FROM staff_profiles WHERE business_id='91000000-0000-4000-8000-000000000001' AND membership_id='94000000-0000-4000-8000-000000000002') <> 1 THEN
  RAISE EXCEPTION 'failed PIN attempt state did not persist in transaction';
 END IF;
END $$;
ROLLBACK;
