-- +goose Up
-- Phase 2A: tenant-scoped catalog, staff profiles, recipes, suppliers and venue tables.
-- Quantities use exact NUMERIC values; monetary values use integer minor units.
-- No stock ledger or valuation policy is introduced in this migration.

CREATE TABLE role_permissions (
  role text NOT NULL CHECK (role IN ('owner','manager','cashier','bartender','stock_controller')),
  permission_key text NOT NULL CHECK (length(trim(permission_key)) BETWEEN 3 AND 100),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (role, permission_key)
);

INSERT INTO role_permissions(role, permission_key) VALUES
 ('owner','*'),
 ('manager','pos.use'),('manager','sales.read'),('manager','sales.void'),('manager','sales.refund'),
 ('manager','discount.approve'),('manager','inventory.read'),('manager','inventory.adjust'),
 ('manager','inventory.count.approve'),('manager','purchasing.manage'),('manager','reports.read'),
 ('manager','staff.read'),('manager','shifts.manage'),('manager','expenses.manage'),
 ('cashier','pos.use'),('cashier','sales.read.own'),('cashier','payments.take'),('cashier','shifts.own'),
 ('bartender','pos.use'),('bartender','tables.manage'),('bartender','sales.read.own'),('bartender','shifts.own'),
 ('stock_controller','inventory.read'),('stock_controller','inventory.count.create'),
 ('stock_controller','purchasing.receive'),('stock_controller','suppliers.read'),('stock_controller','products.read'),('stock_controller','products.manage'),('stock_controller','suppliers.manage'),
 ('manager','products.read'),('manager','tables.manage'),('manager','suppliers.read'),
 ('cashier','products.read'),('bartender','products.read');

CREATE TABLE staff_profiles (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  business_id uuid NOT NULL,
  membership_id uuid NOT NULL,
  display_name text NOT NULL CHECK (length(trim(display_name)) BETWEEN 1 AND 160),
  phone text,
  manager_pin_hash text,
  is_active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (business_id, id),
  UNIQUE (business_id, membership_id),
  FOREIGN KEY (business_id, membership_id) REFERENCES memberships(business_id, id)
);

CREATE TABLE units_of_measure (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  business_id uuid NOT NULL REFERENCES businesses(id),
  code text NOT NULL CHECK (length(trim(code)) BETWEEN 1 AND 24),
  name text NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 80),
  dimension text NOT NULL CHECK (dimension IN ('count','mass','volume')),
  is_active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (business_id, id),
  UNIQUE (business_id, code)
);

CREATE TABLE unit_conversions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  business_id uuid NOT NULL,
  from_unit_id uuid NOT NULL,
  to_unit_id uuid NOT NULL,
  factor numeric(20,8) NOT NULL CHECK (factor > 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (business_id, id),
  UNIQUE (business_id, from_unit_id, to_unit_id),
  CHECK (from_unit_id <> to_unit_id),
  FOREIGN KEY (business_id, from_unit_id) REFERENCES units_of_measure(business_id, id),
  FOREIGN KEY (business_id, to_unit_id) REFERENCES units_of_measure(business_id, id)
);

CREATE TABLE product_categories (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  business_id uuid NOT NULL REFERENCES businesses(id),
  name text NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 100),
  description text,
  is_active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (business_id, id),
  UNIQUE (business_id, name)
);

CREATE TABLE products (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  business_id uuid NOT NULL REFERENCES businesses(id),
  category_id uuid,
  base_unit_id uuid NOT NULL,
  sku text NOT NULL CHECK (length(trim(sku)) BETWEEN 1 AND 80),
  name text NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 160),
  description text,
  product_kind text NOT NULL CHECK (product_kind IN ('unit','liquid')),
  cost_price_minor bigint NOT NULL CHECK (cost_price_minor >= 0),
  sale_price_minor bigint NOT NULL CHECK (sale_price_minor >= 0),
  reorder_level numeric(20,8) NOT NULL DEFAULT 0 CHECK (reorder_level >= 0),
  bottle_size numeric(20,8),
  serving_size numeric(20,8),
  is_sellable boolean NOT NULL DEFAULT true,
  is_active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (business_id, id),
  UNIQUE (business_id, sku),
  FOREIGN KEY (business_id, category_id) REFERENCES product_categories(business_id, id),
  FOREIGN KEY (business_id, base_unit_id) REFERENCES units_of_measure(business_id, id),
  CHECK (
    (product_kind = 'unit' AND bottle_size IS NULL AND serving_size IS NULL)
    OR
    (product_kind = 'liquid' AND bottle_size > 0 AND serving_size > 0 AND serving_size <= bottle_size)
  )
);

CREATE TABLE suppliers (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  business_id uuid NOT NULL REFERENCES businesses(id),
  name text NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 160),
  contact_name text,
  phone text,
  email text,
  address text,
  notes text,
  is_active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (business_id, id),
  UNIQUE (business_id, name)
);

CREATE TABLE recipes (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  business_id uuid NOT NULL,
  output_product_id uuid NOT NULL,
  name text NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 160),
  output_quantity numeric(20,8) NOT NULL DEFAULT 1 CHECK (output_quantity > 0),
  instructions text,
  is_active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (business_id, id),
  UNIQUE (business_id, output_product_id),
  FOREIGN KEY (business_id, output_product_id) REFERENCES products(business_id, id)
);

CREATE TABLE recipe_ingredients (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  business_id uuid NOT NULL,
  recipe_id uuid NOT NULL,
  ingredient_product_id uuid NOT NULL,
  unit_id uuid NOT NULL,
  quantity numeric(20,8) NOT NULL CHECK (quantity > 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (business_id, id),
  UNIQUE (business_id, recipe_id, ingredient_product_id, unit_id),
  FOREIGN KEY (business_id, recipe_id) REFERENCES recipes(business_id, id) ON DELETE CASCADE,
  FOREIGN KEY (business_id, ingredient_product_id) REFERENCES products(business_id, id),
  FOREIGN KEY (business_id, unit_id) REFERENCES units_of_measure(business_id, id)
);

CREATE TABLE bar_tables (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  business_id uuid NOT NULL,
  branch_id uuid NOT NULL,
  table_code text NOT NULL CHECK (length(trim(table_code)) BETWEEN 1 AND 40),
  display_name text NOT NULL CHECK (length(trim(display_name)) BETWEEN 1 AND 80),
  capacity integer NOT NULL DEFAULT 2 CHECK (capacity > 0),
  area text,
  is_active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (business_id, id),
  UNIQUE (business_id, branch_id, table_code),
  FOREIGN KEY (business_id, branch_id) REFERENCES branches(business_id, id)
);

-- Cross-row validation that ordinary CHECK constraints cannot express.
CREATE FUNCTION validate_unit_conversion_dimensions()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE from_dimension text; to_dimension text;
BEGIN
  SELECT dimension INTO from_dimension FROM units_of_measure
   WHERE business_id = NEW.business_id AND id = NEW.from_unit_id;
  SELECT dimension INTO to_dimension FROM units_of_measure
   WHERE business_id = NEW.business_id AND id = NEW.to_unit_id;
  IF from_dimension IS NULL OR to_dimension IS NULL OR from_dimension <> to_dimension THEN
    RAISE EXCEPTION 'unit conversions must use units of the same dimension' USING ERRCODE = 'check_violation';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER unit_conversions_dimension_guard
BEFORE INSERT OR UPDATE ON unit_conversions
FOR EACH ROW EXECUTE FUNCTION validate_unit_conversion_dimensions();

CREATE FUNCTION validate_product_unit_dimension()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE unit_dimension text;
BEGIN
  SELECT dimension INTO unit_dimension FROM units_of_measure
   WHERE business_id = NEW.business_id AND id = NEW.base_unit_id;
  IF unit_dimension IS NULL THEN
    RAISE EXCEPTION 'product base unit is unavailable' USING ERRCODE = 'foreign_key_violation';
  END IF;
  IF NEW.product_kind = 'liquid' AND unit_dimension <> 'volume' THEN
    RAISE EXCEPTION 'liquid products require a volume base unit' USING ERRCODE = 'check_violation';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER products_unit_dimension_guard
BEFORE INSERT OR UPDATE OF business_id, base_unit_id, product_kind ON products
FOR EACH ROW EXECUTE FUNCTION validate_product_unit_dimension();

CREATE FUNCTION validate_recipe_ingredient_dimension()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE product_dimension text; ingredient_dimension text;
BEGIN
  SELECT u.dimension INTO product_dimension
    FROM products p JOIN units_of_measure u
      ON u.business_id = p.business_id AND u.id = p.base_unit_id
   WHERE p.business_id = NEW.business_id AND p.id = NEW.ingredient_product_id;
  SELECT dimension INTO ingredient_dimension FROM units_of_measure
   WHERE business_id = NEW.business_id AND id = NEW.unit_id;
  IF product_dimension IS NULL OR ingredient_dimension IS NULL OR product_dimension <> ingredient_dimension THEN
    RAISE EXCEPTION 'recipe ingredient unit must match the ingredient product dimension' USING ERRCODE = 'check_violation';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER recipe_ingredients_dimension_guard
BEFORE INSERT OR UPDATE OF business_id, ingredient_product_id, unit_id ON recipe_ingredients
FOR EACH ROW EXECUTE FUNCTION validate_recipe_ingredient_dimension();

-- Every catalog table is forced through tenant RLS. Branch-level table rows
-- additionally retain a composite FK to the same tenant's branch.
DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY['staff_profiles','units_of_measure','unit_conversions','product_categories','products','suppliers','recipes','recipe_ingredients','bar_tables'] LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
    EXECUTE format('CREATE POLICY %I ON %I USING (business_id = nullif(current_setting(''app.business_id'', true), '''')::uuid) WITH CHECK (business_id = nullif(current_setting(''app.business_id'', true), '''')::uuid)', t || '_tenant_policy', t);
  END LOOP;
END $$;

ALTER TABLE role_permissions ENABLE ROW LEVEL SECURITY;
ALTER TABLE role_permissions FORCE ROW LEVEL SECURITY;
CREATE POLICY role_permissions_read ON role_permissions FOR SELECT USING (true);
REVOKE INSERT, UPDATE, DELETE, TRUNCATE ON role_permissions FROM mekuin_app;
GRANT SELECT ON role_permissions TO mekuin_app;

-- Future migration-created tables should receive only ordinary DML; RLS must
-- still be explicitly enabled and forced in each migration.
GRANT SELECT, INSERT, UPDATE ON staff_profiles, units_of_measure, unit_conversions,
  product_categories, products, suppliers, recipes, recipe_ingredients, bar_tables TO mekuin_app;

CREATE INDEX staff_profiles_business_active_idx ON staff_profiles(business_id, is_active);
CREATE INDEX products_business_category_idx ON products(business_id, category_id, is_active);
CREATE INDEX products_business_name_idx ON products(business_id, name);
CREATE INDEX suppliers_business_active_idx ON suppliers(business_id, is_active);
CREATE INDEX recipes_business_active_idx ON recipes(business_id, is_active);
CREATE INDEX recipe_ingredients_recipe_idx ON recipe_ingredients(business_id, recipe_id);
CREATE INDEX bar_tables_branch_active_idx ON bar_tables(business_id, branch_id, is_active);

-- +goose Down
DROP TRIGGER IF EXISTS recipe_ingredients_dimension_guard ON recipe_ingredients;
DROP FUNCTION IF EXISTS validate_recipe_ingredient_dimension();
DROP TRIGGER IF EXISTS products_unit_dimension_guard ON products;
DROP FUNCTION IF EXISTS validate_product_unit_dimension();
DROP TRIGGER IF EXISTS unit_conversions_dimension_guard ON unit_conversions;
DROP FUNCTION IF EXISTS validate_unit_conversion_dimensions();
DROP TABLE IF EXISTS bar_tables;
DROP TABLE IF EXISTS recipe_ingredients;
DROP TABLE IF EXISTS recipes;
DROP TABLE IF EXISTS suppliers;
DROP TABLE IF EXISTS products;
DROP TABLE IF EXISTS product_categories;
DROP TABLE IF EXISTS unit_conversions;
DROP TABLE IF EXISTS units_of_measure;
DROP TABLE IF EXISTS staff_profiles;
DROP TABLE IF EXISTS role_permissions;
