-- Run with psql as the migration/database owner after `goose up`.
-- All fixtures are rolled back. This verifies runtime-role row visibility and write isolation.
\set ON_ERROR_STOP on
BEGIN;
INSERT INTO businesses(id,name) VALUES
 ('10000000-0000-4000-8000-000000000001','RLS Test A'),
 ('10000000-0000-4000-8000-000000000002','RLS Test B');
INSERT INTO app_users(id,email,full_name,password_hash) VALUES
 ('20000000-0000-4000-8000-000000000001','rls-a@example.invalid','RLS A','$2a$10$fixture'),
 ('20000000-0000-4000-8000-000000000002','rls-b@example.invalid','RLS B','$2a$10$fixture');
INSERT INTO branches(id,business_id,name) VALUES
 ('30000000-0000-4000-8000-000000000001','10000000-0000-4000-8000-000000000001','Branch A'),
 ('30000000-0000-4000-8000-000000000002','10000000-0000-4000-8000-000000000002','Branch B');
INSERT INTO memberships(id,business_id,user_id,role) VALUES
 ('40000000-0000-4000-8000-000000000001','10000000-0000-4000-8000-000000000001','20000000-0000-4000-8000-000000000001','owner'),
 ('40000000-0000-4000-8000-000000000002','10000000-0000-4000-8000-000000000002','20000000-0000-4000-8000-000000000002','owner');
INSERT INTO membership_branches(business_id,membership_id,branch_id) VALUES
 ('10000000-0000-4000-8000-000000000001','40000000-0000-4000-8000-000000000001','30000000-0000-4000-8000-000000000001'),
 ('10000000-0000-4000-8000-000000000002','40000000-0000-4000-8000-000000000002','30000000-0000-4000-8000-000000000002');
INSERT INTO user_sessions(id,user_id,token_hash,expires_at) VALUES
 ('50000000-0000-4000-8000-000000000001','20000000-0000-4000-8000-000000000001',decode(repeat('a',64),'hex'),now()+interval '1 hour'),
 ('50000000-0000-4000-8000-000000000002','20000000-0000-4000-8000-000000000002',decode(repeat('b',64),'hex'),now()+interval '1 hour');

SET LOCAL ROLE mekuin_app;
SELECT set_config('app.user_id','20000000-0000-4000-8000-000000000001',true);
SELECT set_config('app.business_id','10000000-0000-4000-8000-000000000001',true);
SELECT set_config('app.branch_id','30000000-0000-4000-8000-000000000001',true);
DO $$ BEGIN
  IF (SELECT count(*) FROM lookup_auth_user('rls-a@example.invalid')) <> 1 THEN RAISE EXCEPTION 'auth lookup failed'; END IF;
  IF (SELECT count(*) FROM lookup_auth_user('rls-b@example.invalid')) <> 1 THEN RAISE EXCEPTION 'auth lookup for second fixture failed'; END IF;
  IF (SELECT count(*) FROM lookup_session(decode(repeat('a',64),'hex'))) <> 1 THEN RAISE EXCEPTION 'valid session lookup failed'; END IF;
  IF (SELECT count(*) FROM lookup_session(decode(repeat('b',64),'hex'))) <> 1 THEN RAISE EXCEPTION 'second session lookup failed'; END IF;
  IF (SELECT count(*) FROM businesses) <> 1 THEN RAISE EXCEPTION 'business RLS read isolation failed'; END IF;
  IF (SELECT count(*) FROM app_users) <> 1 THEN RAISE EXCEPTION 'user self-read isolation failed'; END IF;
  IF (SELECT count(*) FROM user_sessions) <> 1 THEN RAISE EXCEPTION 'session self-read isolation failed'; END IF;
  IF (SELECT count(*) FROM branches) <> 1 THEN RAISE EXCEPTION 'branch RLS isolation failed'; END IF;
  IF (SELECT count(*) FROM memberships) <> 1 THEN RAISE EXCEPTION 'membership RLS isolation failed'; END IF;
  IF (SELECT count(*) FROM membership_branches) <> 1 THEN RAISE EXCEPTION 'membership-branch RLS isolation failed'; END IF;
  IF (SELECT count(*) FROM list_current_user_workspaces()) <> 1 THEN RAISE EXCEPTION 'workspace discovery leaked or omitted a workspace'; END IF;
  IF (SELECT count(*) FROM resolve_branch_membership(
      '20000000-0000-4000-8000-000000000001',
      '10000000-0000-4000-8000-000000000001',
      '30000000-0000-4000-8000-000000000001')) <> 1 THEN RAISE EXCEPTION 'valid tenant membership did not resolve'; END IF;
  IF (SELECT count(*) FROM resolve_branch_membership(
      '20000000-0000-4000-8000-000000000001',
      '10000000-0000-4000-8000-000000000002',
      '30000000-0000-4000-8000-000000000002')) <> 0 THEN RAISE EXCEPTION 'cross-tenant membership unexpectedly resolved'; END IF;
END $$;
DO $$ BEGIN
  BEGIN
    INSERT INTO branches(business_id,name) VALUES ('10000000-0000-4000-8000-000000000002','Illegal Cross Tenant');
    RAISE EXCEPTION 'cross-tenant branch insert unexpectedly succeeded';
  EXCEPTION WHEN insufficient_privilege THEN NULL;
  END;
END $$;
DO $$ BEGIN
  BEGIN
    INSERT INTO memberships(business_id,user_id,role)
    VALUES ('10000000-0000-4000-8000-000000000002','20000000-0000-4000-8000-000000000001','manager');
    RAISE EXCEPTION 'cross-tenant membership insert unexpectedly succeeded';
  EXCEPTION WHEN insufficient_privilege THEN NULL;
  END;
END $$;
DO $$ BEGIN
  BEGIN
    INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,summary)
    VALUES ('10000000-0000-4000-8000-000000000002','20000000-0000-4000-8000-000000000001','test','test','cross tenant');
    RAISE EXCEPTION 'cross-tenant audit insert unexpectedly succeeded';
  EXCEPTION WHEN insufficient_privilege THEN NULL;
  END;
END $$;
RESET ROLE;
ROLLBACK;
