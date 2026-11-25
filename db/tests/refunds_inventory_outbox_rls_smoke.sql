-- Run as migration/database owner after all migrations. All fixtures roll back.
\set ON_ERROR_STOP on
BEGIN;
INSERT INTO businesses(id,name) VALUES
 ('b1000000-0000-4000-8000-000000000001','Finance Tenant A'),
 ('b1000000-0000-4000-8000-000000000002','Finance Tenant B');
INSERT INTO app_users(id,email,full_name,password_hash) VALUES
 ('b2000000-0000-4000-8000-000000000001','finance-a@example.invalid','Finance A','$2a$10$fixture'),
 ('b2000000-0000-4000-8000-000000000002','finance-b@example.invalid','Finance B','$2a$10$fixture');
INSERT INTO branches(id,business_id,name) VALUES
 ('b3000000-0000-4000-8000-000000000001','b1000000-0000-4000-8000-000000000001','Branch A'),
 ('b3000000-0000-4000-8000-000000000002','b1000000-0000-4000-8000-000000000002','Branch B');
INSERT INTO memberships(id,business_id,user_id,role) VALUES
 ('b4000000-0000-4000-8000-000000000001','b1000000-0000-4000-8000-000000000001','b2000000-0000-4000-8000-000000000001','owner'),
 ('b4000000-0000-4000-8000-000000000002','b1000000-0000-4000-8000-000000000002','b2000000-0000-4000-8000-000000000002','owner');
INSERT INTO units_of_measure(id,business_id,code,name,dimension) VALUES
 ('b5000000-0000-4000-8000-000000000001','b1000000-0000-4000-8000-000000000001','EA','Each','count');
INSERT INTO products(id,business_id,base_unit_id,sku,name,product_kind,cost_price_minor,sale_price_minor)
VALUES ('b6000000-0000-4000-8000-000000000001','b1000000-0000-4000-8000-000000000001','b5000000-0000-4000-8000-000000000001','FIN-1','Finance Product','unit',100,200);
INSERT INTO sales(id,business_id,branch_id,sale_number,total_minor,created_by)
VALUES ('b7000000-0000-4000-8000-000000000001','b1000000-0000-4000-8000-000000000001','b3000000-0000-4000-8000-000000000001','FIN-SALE',1000,'b2000000-0000-4000-8000-000000000001');
SET LOCAL ROLE mekuin_app;
SELECT set_config('app.user_id','b2000000-0000-4000-8000-000000000001',true);
SELECT set_config('app.business_id','b1000000-0000-4000-8000-000000000001',true);
SELECT set_config('app.branch_id','b3000000-0000-4000-8000-000000000001',true);
INSERT INTO sale_refunds(business_id,branch_id,sale_id,amount_minor,reason,refunded_by)
VALUES ('b1000000-0000-4000-8000-000000000001','b3000000-0000-4000-8000-000000000001','b7000000-0000-4000-8000-000000000001',250,'RLS refund fixture','b2000000-0000-4000-8000-000000000001');
INSERT INTO inventory_counts(id,business_id,branch_id,created_by)
VALUES ('b8000000-0000-4000-8000-000000000001','b1000000-0000-4000-8000-000000000001','b3000000-0000-4000-8000-000000000001','b2000000-0000-4000-8000-000000000001');
INSERT INTO inventory_count_lines(business_id,count_id,product_id,expected_quantity,counted_quantity)
VALUES ('b1000000-0000-4000-8000-000000000001','b8000000-0000-4000-8000-000000000001','b6000000-0000-4000-8000-000000000001',0,12);
INSERT INTO inventory_movements(business_id,branch_id,product_id,quantity_delta,movement_type,reference_id,created_by)
VALUES ('b1000000-0000-4000-8000-000000000001','b3000000-0000-4000-8000-000000000001','b6000000-0000-4000-8000-000000000001',12,'count_reconciliation','b8000000-0000-4000-8000-000000000001','b2000000-0000-4000-8000-000000000001');
INSERT INTO email_outbox(business_id,recipient,subject,body)
VALUES ('b1000000-0000-4000-8000-000000000001','staff@example.invalid','Outbox test','one-time test body');
DO $$ DECLARE queued record; BEGIN
 IF (SELECT count(*) FROM sale_refunds)<>1 THEN RAISE EXCEPTION 'refund tenant RLS read failed'; END IF;
 IF (SELECT count(*) FROM inventory_counts)<>1 OR (SELECT count(*) FROM inventory_count_lines)<>1 THEN RAISE EXCEPTION 'inventory count RLS read failed'; END IF;
 IF (SELECT count(*) FROM inventory_movements)<>1 THEN RAISE EXCEPTION 'inventory movement RLS read failed'; END IF;
 IF has_table_privilege(current_user,'email_outbox','SELECT') OR has_table_privilege(current_user,'email_outbox','UPDATE') THEN RAISE EXCEPTION 'runtime role can directly read or mutate outbox'; END IF;
 IF NOT has_function_privilege(current_user,'claim_email_outbox()','EXECUTE') OR NOT has_function_privilege(current_user,'complete_email_outbox(uuid,boolean,text,integer)','EXECUTE') THEN RAISE EXCEPTION 'outbox worker function grants missing'; END IF;
 IF EXISTS(SELECT 1 FROM sale_refunds WHERE business_id='b1000000-0000-4000-8000-000000000002') THEN RAISE EXCEPTION 'cross-tenant refund visible'; END IF;
 IF EXISTS(SELECT 1 FROM inventory_counts WHERE business_id='b1000000-0000-4000-8000-000000000002') THEN RAISE EXCEPTION 'cross-tenant count visible'; END IF;
 SELECT * INTO queued FROM claim_email_outbox();
 IF queued.recipient <> 'staff@example.invalid' OR queued.attempts <> 1 THEN RAISE EXCEPTION 'outbox claim function failed'; END IF;
 PERFORM complete_email_outbox(queued.id,true,'',1);
END $$;
RESET ROLE;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM email_outbox WHERE recipient='staff@example.invalid' AND status='sent' AND body='') THEN RAISE EXCEPTION 'outbox completion did not clear delivered payload'; END IF;
END $$;
ROLLBACK;
