-- +goose Up
-- Per-branch moving weighted-average inventory valuation. All values are minor currency units.
ALTER TABLE inventory_movements ADD COLUMN unit_cost_minor numeric(20,6) CHECK(unit_cost_minor IS NULL OR unit_cost_minor>=0);
UPDATE inventory_movements im SET unit_cost_minor=prl.unit_cost_minor
FROM purchase_receipt_lines prl WHERE prl.business_id=im.business_id AND prl.movement_id=im.id AND im.movement_type='purchase_receipt';

CREATE TABLE inventory_valuation_balances (
 business_id uuid NOT NULL,
 branch_id uuid NOT NULL,
 product_id uuid NOT NULL,
 on_hand_quantity numeric(20,8) NOT NULL DEFAULT 0 CHECK(on_hand_quantity>=0),
 average_unit_cost_minor numeric(20,6) NOT NULL DEFAULT 0 CHECK(average_unit_cost_minor>=0),
 inventory_value_minor numeric(24,6) NOT NULL DEFAULT 0 CHECK(inventory_value_minor>=0),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(business_id,branch_id,product_id),
 FOREIGN KEY(business_id,branch_id) REFERENCES branches(business_id,id),
 FOREIGN KEY(business_id,product_id) REFERENCES products(business_id,id)
);
CREATE TABLE inventory_valuation_movements (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), business_id uuid NOT NULL, branch_id uuid NOT NULL, product_id uuid NOT NULL,
 inventory_movement_id uuid NOT NULL, movement_type text NOT NULL, quantity_delta numeric(20,8) NOT NULL,
 unit_cost_minor numeric(20,6) NOT NULL CHECK(unit_cost_minor>=0), value_delta_minor numeric(24,6) NOT NULL,
 quantity_after numeric(20,8) NOT NULL, value_after_minor numeric(24,6) NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(business_id,inventory_movement_id),
 FOREIGN KEY(business_id,branch_id) REFERENCES branches(business_id,id),
 FOREIGN KEY(business_id,product_id) REFERENCES products(business_id,id)
);
CREATE INDEX inventory_valuation_movements_product_idx ON inventory_valuation_movements(business_id,branch_id,product_id,created_at,id);
ALTER TABLE inventory_valuation_balances ENABLE ROW LEVEL SECURITY;
ALTER TABLE inventory_valuation_balances FORCE ROW LEVEL SECURITY;
ALTER TABLE inventory_valuation_movements ENABLE ROW LEVEL SECURITY;
ALTER TABLE inventory_valuation_movements FORCE ROW LEVEL SECURITY;
CREATE POLICY inventory_valuation_balances_tenant_policy ON inventory_valuation_balances USING(business_id=nullif(current_setting('app.business_id',true),'')::uuid);
CREATE POLICY inventory_valuation_movements_tenant_policy ON inventory_valuation_movements USING(business_id=nullif(current_setting('app.business_id',true),'')::uuid);
GRANT SELECT ON inventory_valuation_balances,inventory_valuation_movements TO mekuin_app;
REVOKE INSERT,UPDATE,DELETE ON inventory_valuation_balances,inventory_valuation_movements FROM mekuin_app;

CREATE FUNCTION apply_inventory_valuation() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
DECLARE
 q numeric(20,8); avg_cost numeric(20,6); value_before numeric(24,6); movement_cost numeric(20,6);
 q_after numeric(20,8); value_delta numeric(24,6); value_after numeric(24,6); avg_after numeric(20,6);
BEGIN
 IF EXISTS(SELECT 1 FROM public.inventory_valuation_movements WHERE business_id=NEW.business_id AND inventory_movement_id=NEW.id) THEN RETURN NEW; END IF;
 INSERT INTO public.inventory_valuation_balances(business_id,branch_id,product_id,on_hand_quantity,average_unit_cost_minor,inventory_value_minor)
 SELECT NEW.business_id,NEW.branch_id,NEW.product_id,0,p.cost_price_minor,0 FROM public.products p
 WHERE p.business_id=NEW.business_id AND p.id=NEW.product_id ON CONFLICT DO NOTHING;
 SELECT on_hand_quantity,average_unit_cost_minor,inventory_value_minor INTO q,avg_cost,value_before
 FROM public.inventory_valuation_balances WHERE business_id=NEW.business_id AND branch_id=NEW.branch_id AND product_id=NEW.product_id FOR UPDATE;
 movement_cost:=COALESCE(NEW.unit_cost_minor,avg_cost);
 IF NEW.movement_type='purchase_receipt' THEN
   IF NEW.unit_cost_minor IS NULL THEN RAISE EXCEPTION 'purchase receipt movement requires unit_cost_minor'; END IF;
 ELSIF NEW.movement_type='transfer_in' THEN
   SELECT v.unit_cost_minor INTO movement_cost FROM public.inventory_valuation_movements v
   JOIN public.inventory_movements src ON src.business_id=v.business_id AND src.id=v.inventory_movement_id
   WHERE src.business_id=NEW.business_id AND src.reference_id=NEW.reference_id AND src.product_id=NEW.product_id AND src.movement_type='transfer_out'
   ORDER BY src.created_at,src.id LIMIT 1;
   movement_cost:=COALESCE(movement_cost,avg_cost);
 ELSIF NEW.movement_type IN ('sale_void_reversal','sale_refund_reversal') THEN
   SELECT v.unit_cost_minor INTO movement_cost FROM public.inventory_valuation_movements v
   JOIN public.inventory_movements src ON src.business_id=v.business_id AND src.id=v.inventory_movement_id
   WHERE src.business_id=NEW.business_id AND src.reference_id=COALESCE((SELECT rf.sale_id FROM public.sale_refunds rf WHERE rf.business_id=NEW.business_id AND rf.id=NEW.reference_id),NEW.reference_id) AND src.product_id=NEW.product_id AND src.movement_type='sale_consumption'
   ORDER BY src.created_at,src.id LIMIT 1;
   movement_cost:=COALESCE(movement_cost,avg_cost);
 END IF;
 IF NEW.quantity_delta<0 THEN
   IF q + NEW.quantity_delta < 0 THEN RAISE EXCEPTION 'inventory valuation cannot become negative'; END IF;
   value_delta:=NEW.quantity_delta*avg_cost;
   value_after:=greatest(0,value_before+value_delta);
   q_after:=q+NEW.quantity_delta;
   avg_after:=CASE WHEN q_after=0 THEN avg_cost ELSE avg_cost END;
 ELSE
   value_delta:=NEW.quantity_delta*movement_cost;
   value_after:=value_before+value_delta;
   q_after:=q+NEW.quantity_delta;
   avg_after:=CASE WHEN q_after=0 THEN movement_cost ELSE value_after/q_after END;
 END IF;
 INSERT INTO public.inventory_valuation_movements(business_id,branch_id,product_id,inventory_movement_id,movement_type,quantity_delta,unit_cost_minor,value_delta_minor,quantity_after,value_after_minor)
 VALUES(NEW.business_id,NEW.branch_id,NEW.product_id,NEW.id,NEW.movement_type,NEW.quantity_delta,movement_cost,value_delta,q_after,value_after);
 UPDATE public.inventory_valuation_balances SET on_hand_quantity=q_after,average_unit_cost_minor=CASE WHEN q_after=0 THEN avg_after ELSE value_after/q_after END,inventory_value_minor=value_after,updated_at=NEW.created_at
 WHERE business_id=NEW.business_id AND branch_id=NEW.branch_id AND product_id=NEW.product_id;
 RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION apply_inventory_valuation() FROM PUBLIC;
CREATE TRIGGER inventory_movement_apply_valuation AFTER INSERT OR UPDATE OF unit_cost_minor ON inventory_movements FOR EACH ROW EXECUTE FUNCTION apply_inventory_valuation();

-- Rebuild a deterministic opening valuation ledger for existing immutable movements.
DO $$ DECLARE r record; BEGIN
 FOR r IN SELECT id FROM inventory_movements ORDER BY created_at,CASE WHEN movement_type='transfer_out' THEN 0 ELSE 1 END,id LOOP
   UPDATE inventory_movements SET unit_cost_minor=unit_cost_minor WHERE id=r.id;
 END LOOP;
END $$;

-- +goose Down
DROP TRIGGER IF EXISTS inventory_movement_apply_valuation ON inventory_movements;
DROP FUNCTION IF EXISTS apply_inventory_valuation();
DROP TABLE IF EXISTS inventory_valuation_movements;
DROP TABLE IF EXISTS inventory_valuation_balances;
ALTER TABLE inventory_movements DROP COLUMN IF EXISTS unit_cost_minor;
