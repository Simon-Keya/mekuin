-- +goose Up
-- Phase 5A: branch/user cash shifts with immutable paid-in/out movements.
CREATE TABLE cash_shifts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 business_id uuid NOT NULL,
 branch_id uuid NOT NULL,
 user_id uuid NOT NULL,
 status text NOT NULL DEFAULT 'open' CHECK(status IN ('open','closed')),
 opening_float_minor bigint NOT NULL CHECK(opening_float_minor >= 0),
 expected_cash_minor bigint,
 counted_cash_minor bigint,
 variance_minor bigint,
 opened_at timestamptz NOT NULL DEFAULT now(),
 closed_at timestamptz,
 closed_by uuid,
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(business_id,branch_id) REFERENCES branches(business_id,id),
 FOREIGN KEY(business_id,user_id) REFERENCES memberships(business_id,user_id),
 FOREIGN KEY(business_id,closed_by) REFERENCES memberships(business_id,user_id),
 UNIQUE(business_id,branch_id,id),
 CHECK ((status='open' AND closed_at IS NULL AND closed_by IS NULL AND expected_cash_minor IS NULL AND counted_cash_minor IS NULL AND variance_minor IS NULL)
     OR (status='closed' AND closed_at IS NOT NULL AND closed_by IS NOT NULL AND expected_cash_minor IS NOT NULL AND counted_cash_minor IS NOT NULL AND variance_minor = counted_cash_minor-expected_cash_minor))
);
CREATE UNIQUE INDEX cash_shifts_one_open_user_branch_uq ON cash_shifts(business_id,branch_id,user_id) WHERE status='open';
CREATE INDEX cash_shifts_branch_opened_idx ON cash_shifts(business_id,branch_id,opened_at DESC);
ALTER TABLE cash_shifts ENABLE ROW LEVEL SECURITY;
ALTER TABLE cash_shifts FORCE ROW LEVEL SECURITY;
CREATE POLICY cash_shifts_tenant_policy ON cash_shifts
 USING(business_id=nullif(current_setting('app.business_id',true),'')::uuid)
 WITH CHECK(business_id=nullif(current_setting('app.business_id',true),'')::uuid);
REVOKE UPDATE,DELETE ON cash_shifts FROM mekuin_app;
GRANT SELECT,INSERT ON cash_shifts TO mekuin_app;
GRANT UPDATE(status,expected_cash_minor,counted_cash_minor,variance_minor,closed_at,closed_by) ON cash_shifts TO mekuin_app;

CREATE TABLE cash_shift_movements (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 business_id uuid NOT NULL,
 branch_id uuid NOT NULL,
 shift_id uuid NOT NULL,
 direction text NOT NULL CHECK(direction IN ('paid_in','paid_out')),
 amount_minor bigint NOT NULL CHECK(amount_minor > 0),
 reason text NOT NULL CHECK(length(trim(reason)) BETWEEN 5 AND 300),
 recorded_by uuid NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(business_id,branch_id,shift_id) REFERENCES cash_shifts(business_id,branch_id,id),
 FOREIGN KEY(business_id,recorded_by) REFERENCES memberships(business_id,user_id),
 UNIQUE(business_id,id)
);
CREATE INDEX cash_shift_movements_shift_idx ON cash_shift_movements(business_id,shift_id,created_at,id);
ALTER TABLE cash_shift_movements ENABLE ROW LEVEL SECURITY;
ALTER TABLE cash_shift_movements FORCE ROW LEVEL SECURITY;
CREATE POLICY cash_shift_movements_tenant_policy ON cash_shift_movements
 USING(business_id=nullif(current_setting('app.business_id',true),'')::uuid)
 WITH CHECK(business_id=nullif(current_setting('app.business_id',true),'')::uuid);
REVOKE UPDATE,DELETE ON cash_shift_movements FROM mekuin_app;
GRANT SELECT,INSERT ON cash_shift_movements TO mekuin_app;

-- Cash tenders are explicitly assigned to the active drawer shift. M-Pesa/card
-- tenders remain outside the cash drawer reconciliation.
ALTER TABLE sale_payments ADD COLUMN cash_shift_id uuid;
ALTER TABLE sale_payments ADD CONSTRAINT sale_payments_cash_shift_fk
 FOREIGN KEY(business_id,branch_id,cash_shift_id) REFERENCES cash_shifts(business_id,branch_id,id);
-- NOT VALID preserves legacy cash payments while enforcing attribution for all
-- new cash tenders created after this migration.
ALTER TABLE sale_payments ADD CONSTRAINT sale_payments_cash_requires_shift
 CHECK(method <> 'cash' OR cash_shift_id IS NOT NULL) NOT VALID;
CREATE INDEX sale_payments_cash_shift_idx ON sale_payments(business_id,cash_shift_id,created_at) WHERE cash_shift_id IS NOT NULL;

INSERT INTO role_permissions(role,permission_key) VALUES ('manager','shifts.own') ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM role_permissions WHERE role='manager' AND permission_key='shifts.own';
DROP INDEX IF EXISTS sale_payments_cash_shift_idx;
ALTER TABLE sale_payments DROP CONSTRAINT IF EXISTS sale_payments_cash_requires_shift;
ALTER TABLE sale_payments DROP CONSTRAINT IF EXISTS sale_payments_cash_shift_fk;
ALTER TABLE sale_payments DROP COLUMN IF EXISTS cash_shift_id;
DROP TABLE IF EXISTS cash_shift_movements;
DROP TABLE IF EXISTS cash_shifts;
