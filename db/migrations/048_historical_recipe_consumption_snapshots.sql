-- +goose Up
-- Immutable recipe inputs as actually used at checkout; later recipe edits cannot rewrite history.
CREATE TABLE sale_recipe_consumption_snapshots (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), business_id uuid NOT NULL, branch_id uuid NOT NULL,
 sale_id uuid NOT NULL, sold_product_id uuid NOT NULL, ingredient_product_id uuid NOT NULL,
 sold_quantity numeric(20,8) NOT NULL CHECK(sold_quantity>0), recipe_output_quantity numeric(20,8) NOT NULL CHECK(recipe_output_quantity>0),
 ingredient_quantity_per_output numeric(20,8) NOT NULL CHECK(ingredient_quantity_per_output>0),
 consumed_quantity numeric(20,8) NOT NULL CHECK(consumed_quantity>0), recipe_snapshot jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(business_id,branch_id,sale_id) REFERENCES sales(business_id,branch_id,id),
 FOREIGN KEY(business_id,sold_product_id) REFERENCES products(business_id,id),
 FOREIGN KEY(business_id,ingredient_product_id) REFERENCES products(business_id,id)
);
CREATE INDEX sale_recipe_consumption_period_idx ON sale_recipe_consumption_snapshots(business_id,branch_id,created_at,sale_id);
ALTER TABLE sale_recipe_consumption_snapshots ENABLE ROW LEVEL SECURITY;
ALTER TABLE sale_recipe_consumption_snapshots FORCE ROW LEVEL SECURITY;
CREATE POLICY sale_recipe_consumption_tenant_policy ON sale_recipe_consumption_snapshots USING(business_id=nullif(current_setting('app.business_id',true),'')::uuid) WITH CHECK(business_id=nullif(current_setting('app.business_id',true),'')::uuid);
REVOKE UPDATE,DELETE ON sale_recipe_consumption_snapshots FROM mekuin_app;
GRANT SELECT,INSERT ON sale_recipe_consumption_snapshots TO mekuin_app;

-- +goose Down
DROP TABLE IF EXISTS sale_recipe_consumption_snapshots;
