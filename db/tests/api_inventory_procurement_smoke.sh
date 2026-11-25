#!/bin/sh
# Authenticated inventory + procurement API integration test.
# Requires curl, jq, psql, a running API, and MIGRATION_DATABASE_URL with schema-owner access.
set -eu
BASE_URL=${BASE_URL:-http://127.0.0.1:8080}
WEB_ORIGIN=${WEB_ORIGIN:-http://localhost:3000}
: "${MIGRATION_DATABASE_URL:?MIGRATION_DATABASE_URL is required for deterministic ledger fixtures}"
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT
COOKIE_JAR="$TMP_DIR/cookies.txt"

request() {
  method=$1 path=$2 data=${3-} expected=$4
  if [ -n "$data" ]; then
    status=$(curl -sS -o "$TMP_DIR/body" -w '%{http_code}' -b "$COOKIE_JAR" -c "$COOKIE_JAR" \
      -X "$method" "$BASE_URL$path" -H "Origin: $WEB_ORIGIN" -H 'Content-Type: application/json' --data "$data")
  else
    status=$(curl -sS -o "$TMP_DIR/body" -w '%{http_code}' -b "$COOKIE_JAR" -c "$COOKIE_JAR" \
      -X "$method" "$BASE_URL$path" -H "Origin: $WEB_ORIGIN")
  fi
  if [ "$status" != "$expected" ]; then
    echo "FAIL $method $path: expected HTTP $expected, got $status" >&2
    cat "$TMP_DIR/body" >&2
    exit 1
  fi
}

MAILPIT_API_URL=${MAILPIT_API_URL:-http://127.0.0.1:8025}
verify_registration() {
  email=$1 jar=$2
  pre_status=$(curl -sS -o "$TMP_DIR/preverify-body" -w '%{http_code}' -X POST "$BASE_URL/api/v1/auth/login" -H "Origin: $WEB_ORIGIN" -H 'Content-Type: application/json' --data "{\"email\":\"$email\",\"password\":\"SmokePassword123\"}")
  if [ "$pre_status" != "403" ] || ! jq -e '.error=="email_verification_required"' "$TMP_DIR/preverify-body" >/dev/null; then echo "FAIL unverified account login was not blocked for $email" >&2; cat "$TMP_DIR/preverify-body" >&2; exit 1; fi
  message_id=""
  for attempt in $(seq 1 30); do
    messages=$(curl -fsS "$MAILPIT_API_URL/api/v1/messages?limit=50") || true
    message_id=$(printf '%s' "$messages" | jq -r --arg email "$email" '[.messages[]? | select(any(.To[]?; .Address == $email))] | sort_by(.Created) | last | .ID // empty')
    [ -n "$message_id" ] && break
    sleep 1
  done
  if [ -z "$message_id" ]; then echo "FAIL no verification email received for $email" >&2; exit 1; fi
  detail=$(curl -fsS "$MAILPIT_API_URL/api/v1/message/$message_id")
  code=$(printf '%s' "$detail" | jq -r '.Text // .text // empty' | grep -oE '[0-9]{6}' | head -n 1 || true)
  if [ -z "$code" ]; then echo "FAIL verification code missing from Mailpit message" >&2; exit 1; fi
  status=$(curl -sS -o "$TMP_DIR/verify-body" -w '%{http_code}' -b "$jar" -c "$jar" -X POST "$BASE_URL/api/v1/auth/email/verify" -H "Origin: $WEB_ORIGIN" -H 'Content-Type: application/json' --data "{\"email\":\"$email\",\"code\":\"$code\"}")
  if [ "$status" != "200" ]; then echo "FAIL email verification for $email: HTTP $status" >&2; cat "$TMP_DIR/verify-body" >&2; exit 1; fi
}

EMAIL="inventory-smoke-$(date +%s)-$$@example.test"
request POST /api/v1/auth/register "{\"full_name\":\"Inventory Smoke Owner\",\"email\":\"$EMAIL\",\"password\":\"SmokePassword123\",\"business_name\":\"Inventory Smoke Bar\",\"branch_name\":\"Main\"}" 201
verify_registration "$EMAIL" "$COOKIE_JAR"
USER_ID=$(jq -r '.user.id' "$TMP_DIR/body")
BUSINESS_ID=$(jq -r '.business.id' "$TMP_DIR/body")
SOURCE_BRANCH_ID=$(jq -r '.branch.id' "$TMP_DIR/body")
request POST /api/v1/tenant/select "{\"business_id\":\"$BUSINESS_ID\",\"branch_id\":\"$SOURCE_BRANCH_ID\"}" 200
request GET /api/v1/settings/integrations '' 200
jq -e '(.items|length)==3 and (.secrets_policy|contains("never stored")) and .capabilities.etims.invoice_transmission_enabled==false and .capabilities.card.gateway_enabled==false' "$TMP_DIR/body" >/dev/null
request PUT /api/v1/settings/integrations/etims '{"status":"pending","public_config":{"branch_name":"CI branch","environment":"test"}}' 200
request PUT /api/v1/settings/integrations/etims '{"status":"live","public_config":{}}' 409
request PUT /api/v1/settings/integrations/card '{"status":"pending","public_config":{"gateway_label":"Manual terminal"}}' 200

# Create two stock items and a supplier through the public API.
request POST /api/v1/catalog/units '{"code":"EA","name":"Each","dimension":"count"}' 201
UNIT_ID=$(jq -r '.id' "$TMP_DIR/body")
request POST /api/v1/catalog/products "{\"base_unit_id\":\"$UNIT_ID\",\"sku\":\"INV-SMOKE-A\",\"name\":\"Inventory Smoke A\",\"product_kind\":\"unit\",\"cost_price_minor\":1000,\"sale_price_minor\":1500,\"reorder_level\":\"1\",\"is_sellable\":true}" 201
PRODUCT_A=$(jq -r '.id' "$TMP_DIR/body")
request POST /api/v1/catalog/products "{\"base_unit_id\":\"$UNIT_ID\",\"sku\":\"INV-SMOKE-B\",\"name\":\"Inventory Smoke B\",\"product_kind\":\"unit\",\"cost_price_minor\":500,\"sale_price_minor\":800,\"reorder_level\":\"1\",\"is_sellable\":true}" 201
PRODUCT_B=$(jq -r '.id' "$TMP_DIR/body")
request POST /api/v1/catalog/suppliers '{"name":"Inventory Smoke Supplier","contact_name":"CI Fixture"}' 201
SUPPLIER_ID=$(jq -r '.id' "$TMP_DIR/body")

# Seed the immutable opening ledger with the migration owner, then verify live balances.
psql "$MIGRATION_DATABASE_URL" -v ON_ERROR_STOP=1 -c "INSERT INTO inventory_movements(business_id,branch_id,product_id,quantity_delta,movement_type,created_by) VALUES ('$BUSINESS_ID','$SOURCE_BRANCH_ID','$PRODUCT_A',10,'opening','$USER_ID'),('$BUSINESS_ID','$SOURCE_BRANCH_ID','$PRODUCT_B',5,'opening','$USER_ID');" >/dev/null
request GET '/api/v1/inventory/balances?limit=100' '' 200
jq -e --arg a "$PRODUCT_A" --arg b "$PRODUCT_B" 'any(.items[]; .product_id==$a and .on_hand=="10.00000000") and any(.items[]; .product_id==$b and .on_hand=="5.00000000")' "$TMP_DIR/body" >/dev/null

# Manual stock adjustments are explicit ledger entries and history is branch-scoped.
request POST /api/v1/inventory/adjustments "{\"product_id\":\"$PRODUCT_A\",\"quantity_delta\":\"2.25\",\"reason\":\"Opening correction after recount\"}" 201
ADJUSTMENT_ID=$(jq -r '.movement_id' "$TMP_DIR/body")
[ -n "$ADJUSTMENT_ID" ] && [ "$ADJUSTMENT_ID" != null ]
request GET '/api/v1/inventory/movements?limit=100&q=manual_adjustment' '' 200
jq -e --arg id "$ADJUSTMENT_ID" 'any(.items[]; .id==$id and .movement_type=="manual_adjustment" and .quantity_delta=="2.25000000")' "$TMP_DIR/body" >/dev/null
request POST /api/v1/inventory/adjustments "{\"product_id\":\"$PRODUCT_A\",\"quantity_delta\":\"-999\",\"reason\":\"Must not make negative stock\"}" 409
# A multi-line loss must roll back every line if any item is oversubscribed.
# Sort the fixture IDs exactly as the handler does, so the valid line is always
# processed before the deliberately invalid line.
FIRST_PRODUCT=$(printf '%s\n%s\n' "$PRODUCT_A" "$PRODUCT_B" | sort | head -n 1)
SECOND_PRODUCT=$(printf '%s\n%s\n' "$PRODUCT_A" "$PRODUCT_B" | sort | tail -n 1)
if [ "$FIRST_PRODUCT" = "$PRODUCT_A" ]; then FIRST_EXPECTED=12.25000000; else FIRST_EXPECTED=5.00000000; fi
request POST /api/v1/inventory/losses/wastage "{\"reason\":\"CI rollback verification\",\"lines\":[{\"product_id\":\"$FIRST_PRODUCT\",\"quantity\":\"1\"},{\"product_id\":\"$SECOND_PRODUCT\",\"quantity\":\"99\"}]}" 409
FIRST_STOCK=$(psql "$MIGRATION_DATABASE_URL" -v ON_ERROR_STOP=1 -Atqc "SELECT COALESCE(sum(quantity_delta),0)::text FROM inventory_movements WHERE business_id='$BUSINESS_ID' AND branch_id='$SOURCE_BRANCH_ID' AND product_id='$FIRST_PRODUCT';")
[ "$FIRST_STOCK" = "$FIRST_EXPECTED" ] || { echo "FAIL loss request partially committed first line: $FIRST_STOCK" >&2; exit 1; }
request POST /api/v1/inventory/losses/wastage "{\"reason\":\"Damaged stock during CI\",\"lines\":[{\"product_id\":\"$PRODUCT_A\",\"quantity\":\"1.5\"}]}" 201
jq -e '.status=="recorded" and .movement_type=="wastage" and .line_count==1' "$TMP_DIR/body" >/dev/null
request POST /api/v1/inventory/losses/breakage "{\"reason\":\"Broken stock during CI\",\"lines\":[{\"product_id\":\"$PRODUCT_B\",\"quantity\":\"1\"}]}" 201
request POST /api/v1/inventory/losses/breakage "{\"reason\":\"Cannot oversubscribe stock\",\"lines\":[{\"product_id\":\"$PRODUCT_B\",\"quantity\":\"99\"}]}" 409

# Create a branch that is initially unassigned; the API must deny it as a transfer destination.
request POST /api/v1/branches/ '{"name":"Inventory Smoke Unassigned"}' 201
UNASSIGNED_BRANCH_ID=$(jq -r '.id' "$TMP_DIR/body")
request POST /api/v1/inventory/transfers "{\"destination_branch_id\":\"$UNASSIGNED_BRANCH_ID\",\"reason\":\"Must reject unassigned branch\",\"lines\":[{\"product_id\":\"$PRODUCT_A\",\"quantity\":\"1\"}]}" 403
# Assign the owner to a second branch and verify paired transfer movements.
request POST /api/v1/branches/ '{"name":"Inventory Smoke Destination"}' 201
DEST_BRANCH_ID=$(jq -r '.id' "$TMP_DIR/body")
MEMBERSHIP_ID=$(psql "$MIGRATION_DATABASE_URL" -v ON_ERROR_STOP=1 -Atqc "SELECT id FROM memberships WHERE business_id='$BUSINESS_ID' AND user_id='$USER_ID';")
psql "$MIGRATION_DATABASE_URL" -v ON_ERROR_STOP=1 -c "INSERT INTO membership_branches(business_id,membership_id,branch_id) VALUES ('$BUSINESS_ID','$MEMBERSHIP_ID','$DEST_BRANCH_ID');" >/dev/null
request POST /api/v1/inventory/transfers "{\"destination_branch_id\":\"$DEST_BRANCH_ID\",\"reason\":\"Move stock to second branch\",\"lines\":[{\"product_id\":\"$PRODUCT_A\",\"quantity\":\"2\"}]}" 201
TRANSFER_ID=$(jq -r '.transfer_id' "$TMP_DIR/body")
[ -n "$TRANSFER_ID" ] && [ "$TRANSFER_ID" != null ]
TRANSFER_ROWS=$(psql "$MIGRATION_DATABASE_URL" -v ON_ERROR_STOP=1 -Atqc "SELECT count(*) FROM inventory_movements WHERE business_id='$BUSINESS_ID' AND reference_id='$TRANSFER_ID' AND movement_type IN ('transfer_out','transfer_in');")
[ "$TRANSFER_ROWS" = "2" ] || { echo "FAIL expected paired transfer movements, got $TRANSFER_ROWS" >&2; exit 1; }
request POST /api/v1/tenant/select "{\"business_id\":\"$BUSINESS_ID\",\"branch_id\":\"$DEST_BRANCH_ID\"}" 200
request GET '/api/v1/inventory/balances?limit=100' '' 200
jq -e --arg a "$PRODUCT_A" 'any(.items[]; .product_id==$a and .on_hand=="2.00000000")' "$TMP_DIR/body" >/dev/null
request POST /api/v1/tenant/select "{\"business_id\":\"$BUSINESS_ID\",\"branch_id\":\"$SOURCE_BRANCH_ID\"}" 200
request GET '/api/v1/inventory/balances?limit=100' '' 200
jq -e --arg a "$PRODUCT_A" 'any(.items[]; .product_id==$a and .on_hand=="8.75000000")' "$TMP_DIR/body" >/dev/null

# Purchase lifecycle: draft -> ordered -> partial receipt -> replay -> complete.
request POST /api/v1/purchasing/orders/ "{\"supplier_id\":\"$SUPPLIER_ID\",\"po_number\":\"CI-PO-$(date +%s)-$$\",\"notes\":\"Inventory procurement integration\",\"lines\":[{\"product_id\":\"$PRODUCT_A\",\"quantity\":\"5\",\"unit_cost_minor\":900}]}" 201
PO_ID=$(jq -r '.id' "$TMP_DIR/body")
request GET "/api/v1/purchasing/orders/$PO_ID" '' 200
PO_LINE_ID=$(jq -r '.lines[0].id' "$TMP_DIR/body")
request POST "/api/v1/purchasing/orders/$PO_ID/order" '{}' 200
RECEIPT_KEY="ci-receipt-$BUSINESS_ID-$(date +%s)-$$"
RECEIPT_BODY="{\"note\":\"First partial delivery\",\"lines\":[{\"purchase_order_line_id\":\"$PO_LINE_ID\",\"quantity\":\"2\"}]}"
status=$(curl -sS -o "$TMP_DIR/body" -w '%{http_code}' -b "$COOKIE_JAR" -c "$COOKIE_JAR" -X POST "$BASE_URL/api/v1/purchasing/orders/$PO_ID/receive" -H "Origin: $WEB_ORIGIN" -H 'Content-Type: application/json' -H "Idempotency-Key: $RECEIPT_KEY" --data "$RECEIPT_BODY")
[ "$status" = "201" ] || { echo "FAIL partial receipt expected HTTP 201, got $status" >&2; cat "$TMP_DIR/body" >&2; exit 1; }
RECEIPT_ID=$(jq -r '.receipt_id' "$TMP_DIR/body")
jq -e '.status=="partially_received" and .line_count==1' "$TMP_DIR/body" >/dev/null
status=$(curl -sS -o "$TMP_DIR/body" -w '%{http_code}' -b "$COOKIE_JAR" -c "$COOKIE_JAR" -X POST "$BASE_URL/api/v1/purchasing/orders/$PO_ID/receive" -H "Origin: $WEB_ORIGIN" -H 'Content-Type: application/json' -H "Idempotency-Key: $RECEIPT_KEY" --data "$RECEIPT_BODY")
[ "$status" = "200" ] || { echo "FAIL receipt replay expected HTTP 200, got $status" >&2; cat "$TMP_DIR/body" >&2; exit 1; }
jq -e --arg id "$RECEIPT_ID" '.receipt_id==$id and .replayed==true' "$TMP_DIR/body" >/dev/null
# Same idempotency key with a different payload must conflict.
status=$(curl -sS -o "$TMP_DIR/body" -w '%{http_code}' -b "$COOKIE_JAR" -c "$COOKIE_JAR" -X POST "$BASE_URL/api/v1/purchasing/orders/$PO_ID/receive" -H "Origin: $WEB_ORIGIN" -H 'Content-Type: application/json' -H "Idempotency-Key: $RECEIPT_KEY" --data "{\"lines\":[{\"purchase_order_line_id\":\"$PO_LINE_ID\",\"quantity\":\"1\"}]}")
[ "$status" = "409" ] || { echo "FAIL mismatched receipt replay expected HTTP 409, got $status" >&2; cat "$TMP_DIR/body" >&2; exit 1; }
# Over-receipt must not create a receipt or movement.
OVER_KEY="ci-over-receipt-$BUSINESS_ID-$(date +%s)-$$"
status=$(curl -sS -o "$TMP_DIR/body" -w '%{http_code}' -b "$COOKIE_JAR" -c "$COOKIE_JAR" -X POST "$BASE_URL/api/v1/purchasing/orders/$PO_ID/receive" -H "Origin: $WEB_ORIGIN" -H 'Content-Type: application/json' -H "Idempotency-Key: $OVER_KEY" --data "{\"lines\":[{\"purchase_order_line_id\":\"$PO_LINE_ID\",\"quantity\":\"99\"}]}")
[ "$status" = "409" ] || { echo "FAIL over-receipt expected HTTP 409, got $status" >&2; cat "$TMP_DIR/body" >&2; exit 1; }
# Missing idempotency key is rejected before any write.
request POST "/api/v1/purchasing/orders/$PO_ID/receive" "{\"lines\":[{\"purchase_order_line_id\":\"$PO_LINE_ID\",\"quantity\":\"3\"}]}" 400
request GET "/api/v1/purchasing/orders/$PO_ID" '' 200
jq -e '.order.status=="partially_received" and .lines[0].received_quantity=="2.00000000" and (.receipts|length)==1' "$TMP_DIR/body" >/dev/null
# Correctly keyed final receipt completes the PO and increments stock by exactly three.
FINAL_KEY="ci-final-receipt-$BUSINESS_ID-$(date +%s)-$$"
status=$(curl -sS -o "$TMP_DIR/body" -w '%{http_code}' -b "$COOKIE_JAR" -c "$COOKIE_JAR" -X POST "$BASE_URL/api/v1/purchasing/orders/$PO_ID/receive" -H "Origin: $WEB_ORIGIN" -H 'Content-Type: application/json' -H "Idempotency-Key: $FINAL_KEY" --data "{\"note\":\"Final delivery\",\"lines\":[{\"purchase_order_line_id\":\"$PO_LINE_ID\",\"quantity\":\"3\"}]}")
[ "$status" = "201" ] || { echo "FAIL final receipt expected HTTP 201, got $status" >&2; cat "$TMP_DIR/body" >&2; exit 1; }
jq -e '.status=="received"' "$TMP_DIR/body" >/dev/null
request POST "/api/v1/purchasing/orders/$PO_ID/cancel" '{}' 409
request GET "/api/v1/purchasing/orders/$PO_ID" '' 200
jq -e '.order.status=="received" and .lines[0].received_quantity=="5.00000000" and (.receipts|length)==2' "$TMP_DIR/body" >/dev/null
request GET '/api/v1/inventory/valuation' '' 200
jq -e '.valuation_method=="per-branch moving weighted-average cost" and (.stock_value_cost_minor|tonumber)>0' "$TMP_DIR/body" >/dev/null
WAC=$(psql "$MIGRATION_DATABASE_URL" -v ON_ERROR_STOP=1 -Atqc "SELECT round(average_unit_cost_minor,4)::text FROM inventory_valuation_balances WHERE business_id='$BUSINESS_ID' AND branch_id='$SOURCE_BRANCH_ID' AND product_id='$PRODUCT_A';")
[ "$WAC" = "963.6364" ] || { echo "FAIL weighted-average cost expected 963.6364, got $WAC" >&2; exit 1; }
SOURCE_BALANCE=$(psql "$MIGRATION_DATABASE_URL" -v ON_ERROR_STOP=1 -Atqc "SELECT COALESCE(sum(quantity_delta),0)::text FROM inventory_movements WHERE business_id='$BUSINESS_ID' AND branch_id='$SOURCE_BRANCH_ID' AND product_id='$PRODUCT_A';")
[ "$SOURCE_BALANCE" = "13.75000000" ] || { echo "FAIL receipts did not add exactly five units to source stock: $SOURCE_BALANCE" >&2; exit 1; }

# A draft PO remains cancellable and does not generate inventory movements.
request POST /api/v1/purchasing/orders/ "{\"supplier_id\":\"$SUPPLIER_ID\",\"po_number\":\"CI-CANCEL-$(date +%s)-$$\",\"lines\":[{\"product_id\":\"$PRODUCT_B\",\"quantity\":\"1\",\"unit_cost_minor\":400}]}" 201
DRAFT_PO_ID=$(jq -r '.id' "$TMP_DIR/body")
request POST "/api/v1/purchasing/orders/$DRAFT_PO_ID/cancel" '{}' 200
jq -e '.status=="cancelled"' "$TMP_DIR/body" >/dev/null

# Blind stock counts retain expected values privately but do not expose them to the counter API.
request POST /api/v1/inventory/counts/ "{\"count_mode\":\"blind\",\"area\":\"Main bar fridge\",\"lines\":[{\"product_id\":\"$PRODUCT_A\",\"counted_quantity\":\"13.75\"}]}" 201
COUNT_ID=$(jq -r '.count_id' "$TMP_DIR/body")
request GET "/api/v1/inventory/counts/$COUNT_ID" '' 200
jq -e '.count_mode=="blind" and .area=="Main bar fridge" and .expected_values_visible==false and .lines[0].expected_quantity==null and .lines[0].variance==null' "$TMP_DIR/body" >/dev/null
request PATCH "/api/v1/inventory/counts/$COUNT_ID" "{\"lines\":[{\"product_id\":\"$PRODUCT_A\",\"counted_quantity\":\"13.75\"}]}" 200
jq -e '.status=="draft" and .line_count==1' "$TMP_DIR/body" >/dev/null
request GET '/api/v1/inventory/counts/?limit=20' '' 200
jq -e --arg id "$COUNT_ID" 'any(.items[]; .id==$id and .count_mode=="blind")' "$TMP_DIR/body" >/dev/null
request GET "/api/v1/inventory/reconciliation?from=$(date +%F)&to=$(date +%F)" '' 200
jq -e --arg a "$PRODUCT_A" '.theoretical_basis|type=="string"' "$TMP_DIR/body" >/dev/null
jq -e --arg a "$PRODUCT_A" 'any(.items[]; .product_id==$a and .has_opening_count==false and .has_closing_count==false and .actual_use==null)' "$TMP_DIR/body" >/dev/null

# Void and refund must reverse recipe-driven stock movements atomically.
request POST /api/v1/catalog/recipes "{\"output_product_id\":\"$PRODUCT_B\",\"name\":\"Inventory Reversal Recipe\",\"output_quantity\":\"1\",\"ingredients\":[{\"ingredient_product_id\":\"$PRODUCT_A\",\"unit_id\":\"$UNIT_ID\",\"quantity\":\"1\"}]}" 201
request POST /api/v1/shifts/ '{"opening_float_minor":0}' 201
checkout_inventory_sale() {
  key=$1 payload=$2 expected=$3
  status=$(curl -sS -o "$TMP_DIR/body" -w '%{http_code}' -b "$COOKIE_JAR" -c "$COOKIE_JAR" -X POST "$BASE_URL/api/v1/sales/checkout" -H "Origin: $WEB_ORIGIN" -H 'Content-Type: application/json' -H "Idempotency-Key: $key" --data "$payload")
  [ "$status" = "$expected" ] || { echo "FAIL inventory checkout expected HTTP $expected, got $status" >&2; cat "$TMP_DIR/body" >&2; exit 1; }
}
checkout_inventory_sale "ci-direct-stock-sale-$BUSINESS_ID-$$" "{\"payment_method\":\"cash\",\"lines\":[{\"product_id\":\"$PRODUCT_A\",\"quantity\":1}]}" 201
DIRECT_SALE_ID=$(jq -r '.sale_id' "$TMP_DIR/body")
DIRECT_SALE_MOVEMENT=$(psql "$MIGRATION_DATABASE_URL" -v ON_ERROR_STOP=1 -Atqc "SELECT COALESCE(sum(quantity_delta),0)::text FROM inventory_movements WHERE business_id='$BUSINESS_ID' AND branch_id='$SOURCE_BRANCH_ID' AND reference_id='$DIRECT_SALE_ID' AND movement_type='sale_consumption' AND product_id='$PRODUCT_A';")
[ "$DIRECT_SALE_MOVEMENT" = "-1.00000000" ] || { echo "FAIL tracked direct-sale product was not deducted from stock: $DIRECT_SALE_MOVEMENT" >&2; exit 1; }
checkout_inventory_sale "ci-refund-sale-$BUSINESS_ID-$$" "{\"payment_method\":\"cash\",\"lines\":[{\"product_id\":\"$PRODUCT_B\",\"quantity\":2}]}" 201
REFUND_SALE_ID=$(jq -r '.sale_id' "$TMP_DIR/body")
# Create a second account, then attach it as an independently-held manager approver.
MANAGER_EMAIL="inventory-manager-$(date +%s)-$$@example.test"
MANAGER_JAR="$TMP_DIR/manager-cookies.txt"
status=$(curl -sS -o "$TMP_DIR/manager-register" -w '%{http_code}' -c "$MANAGER_JAR" -X POST "$BASE_URL/api/v1/auth/register" -H "Origin: $WEB_ORIGIN" -H 'Content-Type: application/json' --data "{\"full_name\":\"Inventory Approver\",\"email\":\"$MANAGER_EMAIL\",\"password\":\"SmokePassword123\",\"business_name\":\"Approver Tenant\",\"branch_name\":\"Main\"}")
[ "$status" = "201" ] || { echo "FAIL manager fixture registration: HTTP $status" >&2; cat "$TMP_DIR/manager-register" >&2; exit 1; }
verify_registration "$MANAGER_EMAIL" "$MANAGER_JAR"
request POST /api/v1/staff/memberships "{\"email\":\"$MANAGER_EMAIL\",\"role\":\"manager\",\"branch_ids\":[\"$SOURCE_BRANCH_ID\"]}" 201
MANAGER_ID=$(jq -r '.membership_id' "$TMP_DIR/body")
request POST /api/v1/staff/profiles "{\"membership_id\":\"$MANAGER_ID\",\"display_name\":\"Independent Inventory Manager\"}" 201
request PUT "/api/v1/staff/$MANAGER_ID/manager-pin" '{"pin":"8642"}' 200
request POST "/api/v1/inventory/counts/$COUNT_ID/submit" '{}' 200
jq -e '.status=="submitted" and .line_count==1' "$TMP_DIR/body" >/dev/null
request GET "/api/v1/inventory/counts/$COUNT_ID" '' 200
jq -e '.status=="submitted" and .expected_values_visible==false and .lines[0].expected_quantity==null' "$TMP_DIR/body" >/dev/null
request POST /api/v1/staff/authorize-action "{\"manager_membership_id\":\"$MANAGER_ID\",\"pin\":\"8642\",\"action\":\"inventory.count.approve\",\"target_id\":\"$COUNT_ID\"}" 200
COUNT_APPROVAL=$(jq -r '.approval_token' "$TMP_DIR/body")
request POST "/api/v1/inventory/counts/$COUNT_ID/reconcile" "{\"approval_token\":\"$COUNT_APPROVAL\"}" 200
jq -e '.status=="reconciled"' "$TMP_DIR/body" >/dev/null
request POST /api/v1/staff/authorize-action "{\"manager_membership_id\":\"$MANAGER_ID\",\"pin\":\"8642\",\"action\":\"sales.refund\",\"target_id\":\"$REFUND_SALE_ID\",\"target_amount_minor\":800}" 200
REFUND_TOKEN=$(jq -r '.approval_token' "$TMP_DIR/body")
request POST "/api/v1/sales/$REFUND_SALE_ID/refunds" "{\"approval_token\":\"$REFUND_TOKEN\",\"amount_minor\":800,\"reason\":\"Half order returned to stock\"}" 201
REFUND_REVERSED=$(psql "$MIGRATION_DATABASE_URL" -v ON_ERROR_STOP=1 -Atqc "SELECT COALESCE(sum(quantity_delta),0)::text FROM inventory_movements WHERE business_id='$BUSINESS_ID' AND branch_id='$SOURCE_BRANCH_ID' AND reference_id IN (SELECT id FROM sale_refunds WHERE business_id='$BUSINESS_ID' AND sale_id='$REFUND_SALE_ID') AND movement_type='sale_refund_reversal' AND product_id='$PRODUCT_A';")
[ "$REFUND_REVERSED" = "1.00000000" ] || { echo "FAIL partial refund did not reverse proportional recipe stock: $REFUND_REVERSED" >&2; exit 1; }
# A separate full void restores the remaining consumed ingredient quantity.
checkout_inventory_sale "ci-void-sale-$BUSINESS_ID-$$" "{\"payment_method\":\"cash\",\"lines\":[{\"product_id\":\"$PRODUCT_B\",\"quantity\":1}]}" 201
VOID_SALE_ID=$(jq -r '.sale_id' "$TMP_DIR/body")
request POST /api/v1/staff/authorize-action "{\"manager_membership_id\":\"$MANAGER_ID\",\"pin\":\"8642\",\"action\":\"sales.void\",\"target_id\":\"$VOID_SALE_ID\"}" 200
VOID_TOKEN=$(jq -r '.approval_token' "$TMP_DIR/body")
request POST "/api/v1/sales/$VOID_SALE_ID/void" "{\"approval_token\":\"$VOID_TOKEN\",\"reason\":\"CI void restores recipe ingredients\"}" 200
VOID_REVERSED=$(psql "$MIGRATION_DATABASE_URL" -v ON_ERROR_STOP=1 -Atqc "SELECT COALESCE(sum(quantity_delta),0)::text FROM inventory_movements WHERE business_id='$BUSINESS_ID' AND branch_id='$SOURCE_BRANCH_ID' AND reference_id='$VOID_SALE_ID' AND movement_type='sale_void_reversal' AND product_id='$PRODUCT_A';")
[ "$VOID_REVERSED" = "1.00000000" ] || { echo "FAIL void did not reverse recipe stock: $VOID_REVERSED" >&2; exit 1; }
SNAPSHOT_ROWS=$(psql "$MIGRATION_DATABASE_URL" -v ON_ERROR_STOP=1 -Atqc "SELECT count(*) FROM sale_recipe_consumption_snapshots WHERE business_id='$BUSINESS_ID' AND branch_id='$SOURCE_BRANCH_ID' AND sale_id IN ('$REFUND_SALE_ID','$VOID_SALE_ID');")
[ "$SNAPSHOT_ROWS" -ge 2 ] || { echo "FAIL checkout did not persist immutable historical recipe snapshots: $SNAPSHOT_ROWS" >&2; exit 1; }
TODAY=$(date +%F)
for TAB in sales profit stock-value staff category-mix; do
  request GET "/api/v1/reports/summary?tab=$TAB&from=$TODAY&to=$TODAY" '' 200
  jq -e --arg tab "$TAB" '.tab==$tab' "$TMP_DIR/body" >/dev/null
done
request GET "/api/v1/inventory/reconciliation?from=$TODAY&to=$TODAY" '' 200
jq -e '.theoretical_basis|contains("immutable SaleItem")' "$TMP_DIR/body" >/dev/null

printf '%s\n' 'API inventory/procurement smoke test passed: balances, adjustment ledger/history, blind count privacy, stock leakage report baseline guard, proportional refund/void stock reversal, atomic losses, paired transfers, PO lifecycle, partial receiving, idempotency and cancellation.'
