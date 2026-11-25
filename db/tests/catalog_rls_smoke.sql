-- Run as the migration/database owner after goose up. All fixture rows roll back.
\set ON_ERROR_STOP on
BEGIN;
INSERT INTO businesses(id,name) VALUES
 ('81000000-0000-4000-8000-000000000001','Catalog Tenant A'),
 ('81000000-0000-4000-8000-000000000002','Catalog Tenant B');
INSERT INTO branches(id,business_id,name) VALUES
 ('83000000-0000-4000-8000-000000000001','81000000-0000-4000-8000-000000000001','A Main'),
 ('83000000-0000-4000-8000-000000000002','81000000-0000-4000-8000-000000000002','B Main');
INSERT INTO units_of_measure(id,business_id,code,name,dimension) VALUES
 ('84000000-0000-4000-8000-000000000001','81000000-0000-4000-8000-000000000001','ML','Millilitre','volume'),
 ('84000000-0000-4000-8000-000000000002','81000000-0000-4000-8000-000000000002','PCS','Pieces','count');
INSERT INTO product_categories(id,business_id,name) VALUES
 ('85000000-0000-4000-8000-000000000001','81000000-0000-4000-8000-000000000001','Spirits'),
 ('85000000-0000-4000-8000-000000000002','81000000-0000-4000-8000-000000000002','Food');
INSERT INTO products(id,business_id,category_id,base_unit_id,sku,name,product_kind,cost_price_minor,sale_price_minor,bottle_size,serving_size) VALUES
 ('86000000-0000-4000-8000-000000000001','81000000-0000-4000-8000-000000000001','85000000-0000-4000-8000-000000000001','84000000-0000-4000-8000-000000000001','GIN-750','Gin 750ml','liquid',120000,20000,750,30),
 ('86000000-0000-4000-8000-000000000002','81000000-0000-4000-8000-000000000002','85000000-0000-4000-8000-000000000002','84000000-0000-4000-8000-000000000002','PLATE-1','Plate','unit',1000,3000,NULL,NULL);
INSERT INTO suppliers(id,business_id,name) VALUES
 ('87000000-0000-4000-8000-000000000001','81000000-0000-4000-8000-000000000001','Supplier A');

SET LOCAL ROLE mekuin_app;
SELECT set_config('app.business_id','81000000-0000-4000-8000-000000000001',true);
DO $$ BEGIN
 IF (SELECT count(*) FROM products) <> 1 THEN RAISE EXCEPTION 'product tenant read isolation failed'; END IF;
 IF (SELECT count(*) FROM suppliers) <> 1 THEN RAISE EXCEPTION 'supplier tenant read isolation failed'; END IF;
 IF (SELECT count(*) FROM product_categories) <> 1 THEN RAISE EXCEPTION 'category tenant read isolation failed'; END IF;
 IF (SELECT count(*) FROM units_of_measure) <> 1 THEN RAISE EXCEPTION 'unit tenant read isolation failed'; END IF;
 IF (SELECT count(*) FROM role_permissions) < 20 THEN RAISE EXCEPTION 'role permission matrix unavailable'; END IF;
END $$;
DO $$ BEGIN
 BEGIN
  INSERT INTO products(business_id,base_unit_id,sku,name,product_kind,cost_price_minor,sale_price_minor)
  VALUES ('81000000-0000-4000-8000-000000000002','84000000-0000-4000-8000-000000000002','BAD','Cross tenant','unit',0,0);
  RAISE EXCEPTION 'cross-tenant product insert unexpectedly succeeded';
 EXCEPTION WHEN insufficient_privilege THEN NULL;
 END;
END $$;
DO $$ BEGIN
 BEGIN
  INSERT INTO unit_conversions(business_id,from_unit_id,to_unit_id,factor)
  VALUES ('81000000-0000-4000-8000-000000000001','84000000-0000-4000-8000-000000000001','84000000-0000-4000-8000-000000000001',0);
  RAISE EXCEPTION 'invalid conversion unexpectedly succeeded';
 EXCEPTION WHEN check_violation THEN NULL;
 END;
END $$;
DO $$ BEGIN
 BEGIN
  INSERT INTO products(business_id,base_unit_id,sku,name,product_kind,cost_price_minor,sale_price_minor,bottle_size,serving_size)
  VALUES ('81000000-0000-4000-8000-000000000001','84000000-0000-4000-8000-000000000001','BAD-LIQ','Bad liquid','liquid',0,0,30,45);
  RAISE EXCEPTION 'serving larger than bottle unexpectedly succeeded';
 EXCEPTION WHEN check_violation THEN NULL;
 END;
END $$;
DO $$ BEGIN
 BEGIN
  INSERT INTO products(business_id,base_unit_id,sku,name,product_kind,cost_price_minor,sale_price_minor)
  VALUES ('81000000-0000-4000-8000-000000000001','84000000-0000-4000-8000-000000000001','GIN-750','Duplicate SKU','unit',0,0);
  RAISE EXCEPTION 'duplicate tenant SKU unexpectedly succeeded';
 EXCEPTION WHEN unique_violation THEN NULL;
 END;
END $$;
DO $$ BEGIN
 BEGIN
  INSERT INTO role_permissions(role,permission_key) VALUES ('cashier','catalog.admin');
  RAISE EXCEPTION 'runtime role modified permission matrix unexpectedly';
 EXCEPTION WHEN insufficient_privilege THEN NULL;
 END;
END $$;
ROLLBACK;
