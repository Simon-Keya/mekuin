-- +goose Up
-- Persist immutable sale line snapshots, cash tender and branch-scoped idempotency.
CREATE TABLE sale_lines (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), business_id uuid NOT NULL, sale_id uuid NOT NULL,
 product_id uuid NOT NULL, sku_snapshot text NOT NULL, name_snapshot text NOT NULL,
 quantity bigint NOT NULL CHECK(quantity > 0), unit_price_minor bigint NOT NULL CHECK(unit_price_minor >= 0),
 line_total_minor bigint NOT NULL CHECK(line_total_minor >= 0), created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(business_id,sale_id) REFERENCES sales(business_id,id),
 FOREIGN KEY(business_id,product_id) REFERENCES products(business_id,id),
 UNIQUE(business_id,id)
);
CREATE INDEX sale_lines_sale_idx ON sale_lines(business_id,sale_id);
ALTER TABLE sale_lines ENABLE ROW LEVEL SECURITY; ALTER TABLE sale_lines FORCE ROW LEVEL SECURITY;
CREATE POLICY sale_lines_tenant_policy ON sale_lines USING(business_id=nullif(current_setting('app.business_id',true),'')::uuid) WITH CHECK(business_id=nullif(current_setting('app.business_id',true),'')::uuid);
REVOKE UPDATE,DELETE ON sale_lines FROM mekuin_app;
GRANT SELECT,INSERT ON sale_lines TO mekuin_app;

CREATE TABLE sale_payments (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), business_id uuid NOT NULL, branch_id uuid NOT NULL,
 sale_id uuid NOT NULL, method text NOT NULL CHECK(method IN ('cash','mpesa','card')),
 amount_minor bigint NOT NULL CHECK(amount_minor > 0), status text NOT NULL CHECK(status IN ('recorded','pending_verification','verified','failed')),
 provider_reference text, recorded_by uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(business_id,branch_id,sale_id) REFERENCES sales(business_id,branch_id,id),
 FOREIGN KEY(business_id,recorded_by) REFERENCES memberships(business_id,user_id),
 UNIQUE(business_id,id)
);
CREATE INDEX sale_payments_sale_idx ON sale_payments(business_id,sale_id);
ALTER TABLE sale_payments ENABLE ROW LEVEL SECURITY; ALTER TABLE sale_payments FORCE ROW LEVEL SECURITY;
CREATE POLICY sale_payments_tenant_policy ON sale_payments USING(business_id=nullif(current_setting('app.business_id',true),'')::uuid) WITH CHECK(business_id=nullif(current_setting('app.business_id',true),'')::uuid);
REVOKE UPDATE,DELETE ON sale_payments FROM mekuin_app;
GRANT SELECT,INSERT ON sale_payments TO mekuin_app;

CREATE TABLE checkout_idempotency (
 business_id uuid NOT NULL, branch_id uuid NOT NULL, idempotency_key text NOT NULL CHECK(length(idempotency_key) BETWEEN 8 AND 64),
 request_hash text NOT NULL CHECK(length(request_hash)=64), sale_id uuid NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(business_id,branch_id,idempotency_key),
 FOREIGN KEY(business_id,branch_id) REFERENCES branches(business_id,id),
 FOREIGN KEY(business_id,sale_id) REFERENCES sales(business_id,id)
);
ALTER TABLE checkout_idempotency ENABLE ROW LEVEL SECURITY; ALTER TABLE checkout_idempotency FORCE ROW LEVEL SECURITY;
CREATE POLICY checkout_idempotency_tenant_policy ON checkout_idempotency USING(business_id=nullif(current_setting('app.business_id',true),'')::uuid) WITH CHECK(business_id=nullif(current_setting('app.business_id',true),'')::uuid);
REVOKE UPDATE,DELETE ON checkout_idempotency FROM mekuin_app;
GRANT SELECT,INSERT ON checkout_idempotency TO mekuin_app;

-- +goose Down
DROP TABLE IF EXISTS checkout_idempotency;
DROP TABLE IF EXISTS sale_payments;
DROP TABLE IF EXISTS sale_lines;
