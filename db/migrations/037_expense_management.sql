-- +goose Up
-- Phase 5B: branch-scoped expenses, approval history and immutable payment records.
CREATE TABLE expense_categories (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 business_id uuid NOT NULL REFERENCES businesses(id),
 name text NOT NULL CHECK(length(trim(name)) BETWEEN 2 AND 80),
 is_active boolean NOT NULL DEFAULT true,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(business_id,id), UNIQUE(business_id,name)
);
INSERT INTO expense_categories(business_id,name)
SELECT b.id,v.name FROM businesses b CROSS JOIN (VALUES
 ('Rent'),('Utilities'),('Transport'),('Repairs & maintenance'),('Cleaning'),('Staff welfare'),('Office supplies'),('Marketing'),('Licences & fees'),('Other')
) v(name) ON CONFLICT(business_id,name) DO NOTHING;
ALTER TABLE expense_categories ENABLE ROW LEVEL SECURITY;
ALTER TABLE expense_categories FORCE ROW LEVEL SECURITY;
CREATE POLICY expense_categories_tenant_policy ON expense_categories
 USING(business_id=nullif(current_setting('app.business_id',true),'')::uuid)
 WITH CHECK(business_id=nullif(current_setting('app.business_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE ON expense_categories TO mekuin_app;
REVOKE DELETE ON expense_categories FROM mekuin_app;

CREATE TABLE expenses (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), business_id uuid NOT NULL, branch_id uuid NOT NULL,
 category_id uuid NOT NULL, description text NOT NULL CHECK(length(trim(description)) BETWEEN 3 AND 500),
 amount_minor bigint NOT NULL CHECK(amount_minor>0), currency char(3) NOT NULL,
 expense_date date NOT NULL, payee text, reference text, notes text,
 status text NOT NULL DEFAULT 'submitted' CHECK(status IN ('submitted','approved','rejected','paid','void')),
 created_by uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 reviewed_by uuid, reviewed_at timestamptz, rejection_reason text,
 paid_at timestamptz,
 FOREIGN KEY(business_id,branch_id) REFERENCES branches(business_id,id),
 FOREIGN KEY(business_id,category_id) REFERENCES expense_categories(business_id,id),
 FOREIGN KEY(business_id,created_by) REFERENCES memberships(business_id,user_id),
 FOREIGN KEY(business_id,reviewed_by) REFERENCES memberships(business_id,user_id),
 UNIQUE(business_id,branch_id,id),
 CHECK((status='submitted' AND reviewed_at IS NULL AND reviewed_by IS NULL) OR (status IN ('approved','rejected','paid','void') AND reviewed_at IS NOT NULL AND reviewed_by IS NOT NULL)),
 CHECK(status <> 'rejected' OR length(trim(COALESCE(rejection_reason,'')))>=5),
 CHECK((status='paid')=(paid_at IS NOT NULL))
);
CREATE INDEX expenses_branch_date_idx ON expenses(business_id,branch_id,expense_date DESC,created_at DESC);
CREATE INDEX expenses_status_idx ON expenses(business_id,branch_id,status,created_at DESC);
ALTER TABLE expenses ENABLE ROW LEVEL SECURITY;
ALTER TABLE expenses FORCE ROW LEVEL SECURITY;
CREATE POLICY expenses_tenant_policy ON expenses
 USING(business_id=nullif(current_setting('app.business_id',true),'')::uuid)
 WITH CHECK(business_id=nullif(current_setting('app.business_id',true),'')::uuid);
GRANT SELECT,INSERT ON expenses TO mekuin_app;
GRANT UPDATE(status,reviewed_by,reviewed_at,rejection_reason,paid_at) ON expenses TO mekuin_app;
REVOKE DELETE ON expenses FROM mekuin_app;

CREATE TABLE expense_review_events (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), business_id uuid NOT NULL, branch_id uuid NOT NULL,
 expense_id uuid NOT NULL, event_no integer NOT NULL CHECK(event_no>0), decision text NOT NULL CHECK(decision IN ('approved','rejected')),
 reason text, actor_user_id uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(business_id,branch_id,expense_id) REFERENCES expenses(business_id,branch_id,id),
 FOREIGN KEY(business_id,actor_user_id) REFERENCES memberships(business_id,user_id),
 UNIQUE(business_id,expense_id,event_no)
);
CREATE INDEX expense_review_events_expense_idx ON expense_review_events(business_id,expense_id,event_no);
ALTER TABLE expense_review_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE expense_review_events FORCE ROW LEVEL SECURITY;
CREATE POLICY expense_review_events_tenant_policy ON expense_review_events
 USING(business_id=nullif(current_setting('app.business_id',true),'')::uuid)
 WITH CHECK(business_id=nullif(current_setting('app.business_id',true),'')::uuid);
GRANT SELECT,INSERT ON expense_review_events TO mekuin_app;
REVOKE UPDATE,DELETE ON expense_review_events FROM mekuin_app;

CREATE TABLE expense_payments (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), business_id uuid NOT NULL, branch_id uuid NOT NULL,
 expense_id uuid NOT NULL, payment_method text NOT NULL CHECK(payment_method IN ('cash','bank_transfer','mobile_money','card','other')),
 amount_minor bigint NOT NULL CHECK(amount_minor>0), payment_reference text, cash_shift_id uuid,
 recorded_by uuid NOT NULL, paid_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(business_id,branch_id,expense_id) REFERENCES expenses(business_id,branch_id,id),
 FOREIGN KEY(business_id,branch_id,cash_shift_id) REFERENCES cash_shifts(business_id,branch_id,id),
 FOREIGN KEY(business_id,recorded_by) REFERENCES memberships(business_id,user_id),
 UNIQUE(business_id,expense_id),
 CHECK((payment_method='cash' AND cash_shift_id IS NOT NULL) OR (payment_method<>'cash' AND cash_shift_id IS NULL))
);
CREATE INDEX expense_payments_branch_date_idx ON expense_payments(business_id,branch_id,paid_at DESC);
CREATE INDEX expense_payments_shift_idx ON expense_payments(business_id,cash_shift_id) WHERE cash_shift_id IS NOT NULL;
ALTER TABLE expense_payments ENABLE ROW LEVEL SECURITY;
ALTER TABLE expense_payments FORCE ROW LEVEL SECURITY;
CREATE POLICY expense_payments_tenant_policy ON expense_payments
 USING(business_id=nullif(current_setting('app.business_id',true),'')::uuid)
 WITH CHECK(business_id=nullif(current_setting('app.business_id',true),'')::uuid);
GRANT SELECT,INSERT ON expense_payments TO mekuin_app;
REVOKE UPDATE,DELETE ON expense_payments FROM mekuin_app;

INSERT INTO role_permissions(role,permission_key) VALUES
 ('manager','expenses.read'),('manager','expenses.approve'),('manager','expenses.manage')
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM role_permissions WHERE role='manager' AND permission_key IN ('expenses.read','expenses.approve','expenses.manage');
DROP TABLE IF EXISTS expense_payments;
DROP TABLE IF EXISTS expense_review_events;
DROP TABLE IF EXISTS expenses;
DROP TABLE IF EXISTS expense_categories;
