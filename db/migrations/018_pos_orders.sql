-- +goose Up
CREATE TABLE pos_orders (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 business_id uuid NOT NULL,
 branch_id uuid NOT NULL,
 order_number text NOT NULL CHECK(length(trim(order_number)) BETWEEN 1 AND 80),
 service_type text NOT NULL CHECK(service_type IN ('walk_in','table','tab')),
 status text NOT NULL DEFAULT 'open' CHECK(status IN ('open','held','checked_out','cancelled')),
 table_id uuid,
 label text CHECK(label IS NULL OR length(trim(label)) BETWEEN 1 AND 100),
 created_by uuid NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 checked_out_sale_id uuid,
 UNIQUE(business_id,id), UNIQUE(business_id,branch_id,order_number),
 FOREIGN KEY(business_id,branch_id) REFERENCES branches(business_id,id),
 FOREIGN KEY(business_id,created_by) REFERENCES memberships(business_id,user_id),
 FOREIGN KEY(business_id,table_id) REFERENCES bar_tables(business_id,id),
 FOREIGN KEY(business_id,checked_out_sale_id) REFERENCES sales(business_id,id),
 CHECK((service_type='table' AND table_id IS NOT NULL) OR (service_type<>'table' AND table_id IS NULL)),
 CHECK((status='checked_out' AND checked_out_sale_id IS NOT NULL) OR (status<>'checked_out' AND checked_out_sale_id IS NULL))
);
CREATE INDEX pos_orders_branch_status_updated_idx ON pos_orders(business_id,branch_id,status,updated_at DESC);
ALTER TABLE pos_orders ENABLE ROW LEVEL SECURITY;
ALTER TABLE pos_orders FORCE ROW LEVEL SECURITY;
CREATE POLICY pos_orders_tenant_policy ON pos_orders USING(business_id=nullif(current_setting('app.business_id',true),'')::uuid) WITH CHECK(business_id=nullif(current_setting('app.business_id',true),'')::uuid);
GRANT SELECT,INSERT ON pos_orders TO mekuin_app;
GRANT UPDATE(status,updated_at,checked_out_sale_id) ON pos_orders TO mekuin_app;

CREATE TABLE pos_order_lines (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 business_id uuid NOT NULL,
 order_id uuid NOT NULL,
 product_id uuid NOT NULL,
 sku_snapshot text NOT NULL,
 name_snapshot text NOT NULL,
 quantity integer NOT NULL CHECK(quantity BETWEEN 1 AND 10000),
 unit_price_minor bigint NOT NULL CHECK(unit_price_minor>=0),
 line_total_minor bigint NOT NULL CHECK(line_total_minor>=0),
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(business_id,order_id) REFERENCES pos_orders(business_id,id),
 FOREIGN KEY(business_id,product_id) REFERENCES products(business_id,id)
);
CREATE INDEX pos_order_lines_order_idx ON pos_order_lines(business_id,order_id,created_at);
ALTER TABLE pos_order_lines ENABLE ROW LEVEL SECURITY;
ALTER TABLE pos_order_lines FORCE ROW LEVEL SECURITY;
CREATE POLICY pos_order_lines_tenant_policy ON pos_order_lines USING(business_id=nullif(current_setting('app.business_id',true),'')::uuid) WITH CHECK(business_id=nullif(current_setting('app.business_id',true),'')::uuid);
GRANT SELECT,INSERT ON pos_order_lines TO mekuin_app;

-- +goose Down
DROP TABLE IF EXISTS pos_order_lines;
DROP TABLE IF EXISTS pos_orders;
