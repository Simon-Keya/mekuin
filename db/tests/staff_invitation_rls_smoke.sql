-- Run as migration/database owner after Goose migrations. All fixtures roll back.
\set ON_ERROR_STOP on
BEGIN;
INSERT INTO businesses(id,name) VALUES ('a1000000-0000-4000-8000-000000000001','Invitation Tenant A');
INSERT INTO app_users(id,email,full_name,password_hash) VALUES
 ('a2000000-0000-4000-8000-000000000001','inviter@example.invalid','Inviter','$2a$10$fixture'),
 ('a2000000-0000-4000-8000-000000000002','invitee@example.invalid','Invitee','$2a$10$fixture');
INSERT INTO branches(id,business_id,name) VALUES ('a3000000-0000-4000-8000-000000000001','a1000000-0000-4000-8000-000000000001','Invitation Branch');
INSERT INTO memberships(id,business_id,user_id,role) VALUES ('a4000000-0000-4000-8000-000000000001','a1000000-0000-4000-8000-000000000001','a2000000-0000-4000-8000-000000000001','owner');
INSERT INTO membership_branches(business_id,membership_id,branch_id) VALUES ('a1000000-0000-4000-8000-000000000001','a4000000-0000-4000-8000-000000000001','a3000000-0000-4000-8000-000000000001');
INSERT INTO staff_invitations(id,business_id,invited_email,role,branch_ids,token_hash,invited_by,expires_at)
VALUES
 ('a5000000-0000-4000-8000-000000000001','a1000000-0000-4000-8000-000000000001','invitee@example.invalid','cashier',ARRAY['a3000000-0000-4000-8000-000000000001'::uuid],decode(repeat('c',64),'hex'),'a2000000-0000-4000-8000-000000000001',now()+interval '1 hour'),
 ('a5000000-0000-4000-8000-000000000002','a1000000-0000-4000-8000-000000000001','newinvitee@example.invalid','bartender',ARRAY['a3000000-0000-4000-8000-000000000001'::uuid],decode(repeat('d',64),'hex'),'a2000000-0000-4000-8000-000000000001',now()+interval '1 hour');
SET LOCAL ROLE mekuin_app;
SELECT set_config('app.user_id','a2000000-0000-4000-8000-000000000001',true);
SELECT set_config('app.business_id','a1000000-0000-4000-8000-000000000001',true);
SELECT set_config('app.branch_id','a3000000-0000-4000-8000-000000000001',true);
DO $$ DECLARE mid uuid; new_uid uuid; new_email text; replay_succeeded boolean := false; BEGIN
  IF (SELECT count(*) FROM staff_invitations) <> 2 THEN RAISE EXCEPTION 'invitation tenant RLS read failed'; END IF;
  SELECT membership_id INTO mid FROM accept_staff_invitation(decode(repeat('c',64),'hex'),'a2000000-0000-4000-8000-000000000002');
  IF mid IS NULL THEN RAISE EXCEPTION 'invitation did not create membership'; END IF;
  IF (SELECT count(*) FROM resolve_branch_membership('a2000000-0000-4000-8000-000000000002','a1000000-0000-4000-8000-000000000001','a3000000-0000-4000-8000-000000000001')) <> 1 THEN RAISE EXCEPTION 'accepted invite branch access missing'; END IF;
  SELECT user_id,email INTO new_uid,new_email FROM register_staff_invitee(decode(repeat('d',64),'hex'),'New Invitee',repeat('x',60));
  IF new_uid IS NULL OR new_email <> 'newinvitee@example.invalid' THEN RAISE EXCEPTION 'invited account registration failed'; END IF;
  SELECT membership_id INTO mid FROM accept_staff_invitation(decode(repeat('d',64),'hex'),new_uid);
  IF mid IS NULL THEN RAISE EXCEPTION 'new invitee membership acceptance failed'; END IF;
  BEGIN
    BEGIN
      PERFORM * FROM accept_staff_invitation(decode(repeat('c',64),'hex'),'a2000000-0000-4000-8000-000000000002');
      replay_succeeded := true;
    EXCEPTION WHEN SQLSTATE 'P0001' THEN NULL;
    END;
    IF replay_succeeded THEN RAISE EXCEPTION 'invitation replay unexpectedly succeeded'; END IF;
  END;
END $$;
RESET ROLE;
ROLLBACK;
