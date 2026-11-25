#!/bin/sh
# End-to-end API smoke test. Requires curl and jq; API must use the mekuin_app role.
set -eu
BASE_URL=${BASE_URL:-http://127.0.0.1:8080}
WEB_ORIGIN=${WEB_ORIGIN:-http://localhost:3000}
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT
COOKIE_JAR="$TMP_DIR/cookies-owner-a.txt"
COOKIE_JAR_B="$TMP_DIR/cookies-owner-b.txt"

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

EMAIL="api-smoke-$(date +%s)-$$@example.test"
request POST /api/v1/auth/register "{\"full_name\":\"API Smoke Owner\",\"email\":\"$EMAIL\",\"password\":\"SmokePassword123\",\"business_name\":\"API Smoke Bar\",\"branch_name\":\"Main\"}" 201
verify_registration "$EMAIL" "$COOKIE_JAR"
OWNER_USER_ID=$(jq -r '.user.id' "$TMP_DIR/body")
BUSINESS_ID=$(jq -r '.business.id' "$TMP_DIR/body")
BRANCH_ID=$(jq -r '.branch.id' "$TMP_DIR/body")
[ -n "$BUSINESS_ID" ] && [ "$BUSINESS_ID" != null ]
[ -n "$BRANCH_ID" ] && [ "$BRANCH_ID" != null ]
request POST /api/v1/tenant/select "{\"business_id\":\"$BUSINESS_ID\",\"branch_id\":\"$BRANCH_ID\"}" 200
# Cash tender attribution requires an open shift.
request POST /api/v1/shifts/ '{"opening_float_minor":500000}' 201
OWNER_SHIFT_ID=$(jq -r '.shift_id' "$TMP_DIR/body")
[ -n "$OWNER_SHIFT_ID" ] && [ "$OWNER_SHIFT_ID" != null ]
request POST /api/v1/catalog/categories '{"name":"API Smoke Category","description":"CI integration fixture"}' 201
CATEGORY_ID=$(jq -r '.id' "$TMP_DIR/body")
[ -n "$CATEGORY_ID" ] && [ "$CATEGORY_ID" != null ]
request GET '/api/v1/catalog/categories?limit=10&q=Smoke' '' 200
jq -e --arg id "$CATEGORY_ID" 'any(.[]; .id == $id)' "$TMP_DIR/body" >/dev/null
request GET '/api/v1/catalog/categories?limit=101' '' 400
request GET /api/v1/staff/ '' 200
printf '%s\n' 'API catalog smoke test passed: registration, tenant selection, category create/list, pagination validation and staff read.'

# A second independently registered tenant must not select tenant A's workspace.
EMAIL_B="api-smoke-b-$(date +%s)-$$@example.test"
status=$(curl -sS -o "$TMP_DIR/body-b" -w '%{http_code}' -c "$COOKIE_JAR_B" \
  -X POST "$BASE_URL/api/v1/auth/register" -H "Origin: $WEB_ORIGIN" \
  -H 'Content-Type: application/json' \
  --data "{\"full_name\":\"API Smoke Owner B\",\"email\":\"$EMAIL_B\",\"password\":\"SmokePassword123\",\"business_name\":\"API Smoke Bar B\",\"branch_name\":\"Main\"}")
if [ "$status" != "201" ]; then
  echo "FAIL second tenant registration: expected HTTP 201, got $status" >&2
  cat "$TMP_DIR/body-b" >&2
  exit 1
fi
verify_registration "$EMAIL_B" "$COOKIE_JAR_B"
BRANCH_B_ID=$(jq -r '.branch.id' "$TMP_DIR/body-b")
status=$(curl -sS -o "$TMP_DIR/cross-tenant-body" -w '%{http_code}' -b "$COOKIE_JAR_B" -c "$COOKIE_JAR_B" \
  -X POST "$BASE_URL/api/v1/tenant/select" -H "Origin: $WEB_ORIGIN" \
  -H 'Content-Type: application/json' \
  --data "{\"business_id\":\"$BUSINESS_ID\",\"branch_id\":\"$BRANCH_ID\"}")
if [ "$status" != "403" ]; then
  echo "FAIL cross-tenant workspace selection: expected HTTP 403, got $status" >&2
  cat "$TMP_DIR/cross-tenant-body" >&2
  exit 1
fi
printf '%s\n' 'Cross-tenant workspace selection correctly denied.'

# Exact directional unit conversion + recipe-costing integration.
request POST /api/v1/catalog/units '{"code":"ML","name":"Millilitre","dimension":"volume"}' 201
UNIT_ML=$(jq -r '.id' "$TMP_DIR/body")
request POST /api/v1/catalog/units '{"code":"CL","name":"Centilitre","dimension":"volume"}' 201
UNIT_CL=$(jq -r '.id' "$TMP_DIR/body")
request POST /api/v1/catalog/unit-conversions "{\"from_unit_id\":\"$UNIT_CL\",\"to_unit_id\":\"$UNIT_ML\",\"factor\":\"10\"}" 201
CONVERSION_ID=$(jq -r '.id' "$TMP_DIR/body")
[ -n "$CONVERSION_ID" ] && [ "$CONVERSION_ID" != null ]
request GET '/api/v1/catalog/unit-conversions?limit=10&q=CL' '' 200
jq -e --arg id "$CONVERSION_ID" 'any(.[]; .id == $id and .factor == "10.00000000")' "$TMP_DIR/body" >/dev/null
request POST /api/v1/catalog/products "{\"base_unit_id\":\"$UNIT_ML\",\"sku\":\"SMOKE-INGREDIENT\",\"name\":\"Smoke Ingredient\",\"product_kind\":\"unit\",\"cost_price_minor\":1000,\"sale_price_minor\":0,\"reorder_level\":\"0\",\"is_sellable\":false}" 201
INGREDIENT_ID=$(jq -r '.id' "$TMP_DIR/body")
request POST /api/v1/catalog/products "{\"base_unit_id\":\"$UNIT_ML\",\"sku\":\"SMOKE-OUTPUT\",\"name\":\"Smoke Output\",\"product_kind\":\"unit\",\"cost_price_minor\":0,\"sale_price_minor\":200000,\"reorder_level\":\"0\",\"is_sellable\":true}" 201
OUTPUT_ID=$(jq -r '.id' "$TMP_DIR/body")
request POST /api/v1/sales/quote "{\"lines\":[{\"product_id\":\"$OUTPUT_ID\",\"quantity\":2}]}" 200
jq -e ".currency == \"KES\" and .totals.subtotal_minor == 400000 and .totals.total_minor == 400000 and .lines[0].line_total_minor == 400000" "$TMP_DIR/body" >/dev/null
# Checkout persists a server-priced sale, immutable line snapshot and cash tender.
CHECKOUT_KEY="ci-checkout-$(date +%s)-$$"
checkout_request() {
  status=$(curl -sS -o "$TMP_DIR/checkout-body" -w '%{http_code}' -b "$COOKIE_JAR" -c "$COOKIE_JAR" \
    -X POST "$BASE_URL/api/v1/sales/checkout" -H "Origin: $WEB_ORIGIN" \
    -H 'Content-Type: application/json' -H "Idempotency-Key: $CHECKOUT_KEY" \
    --data "$1")
  [ "$status" = "$2" ] || { echo "FAIL checkout expected HTTP $2, got $status" >&2; cat "$TMP_DIR/checkout-body" >&2; exit 1; }
  cp "$TMP_DIR/checkout-body" "$TMP_DIR/body"
}
CHECKOUT_BODY="{\"payment_method\":\"cash\",\"lines\":[{\"product_id\":\"$OUTPUT_ID\",\"quantity\":2}]}"
checkout_request "$CHECKOUT_BODY" 201
CHECKOUT_SALE_ID=$(jq -r '.sale_id' "$TMP_DIR/body")
jq -e '.total_minor == 400000 and .payment_status == "recorded" and .idempotent_replay == false' "$TMP_DIR/body" >/dev/null
checkout_request "$CHECKOUT_BODY" 200
jq -e --arg id "$CHECKOUT_SALE_ID" '.sale_id == $id and .idempotent_replay == true' "$TMP_DIR/body" >/dev/null
checkout_request '{"payment_method":"cash","lines":[{"product_id":"00000000-0000-4000-8000-000000000000","quantity":1}]}' 409
printf '%s\n' 'Transactional cash checkout, immutable snapshots and idempotent replay passed.'

request POST /api/v1/catalog/recipes "{\"output_product_id\":\"$OUTPUT_ID\",\"name\":\"Smoke Recipe\",\"output_quantity\":\"1\",\"ingredients\":[{\"ingredient_product_id\":\"$INGREDIENT_ID\",\"unit_id\":\"$UNIT_CL\",\"quantity\":\"10\"}]}" 201
RECIPE_ID=$(jq -r '.id' "$TMP_DIR/body")
request GET "/api/v1/catalog/recipes/$RECIPE_ID/cost" '' 200
jq -e '(.total_recipe_cost_minor | tonumber) == 100000 and (.cost_per_output_unit_minor | tonumber) == 100000 and (.gross_margin_minor | tonumber) == 100000 and .cost_basis == "current_products.cost_price_minor"' "$TMP_DIR/body" >/dev/null
printf '%s\n' 'Unit conversion and recipe-costing integration passed.'
# Recipe-backed checkout converts 10 CL per output into 100 ML, then consumes
# 200 ML for two outputs. Opening stock is seeded by the migration owner.
psql "$MIGRATION_DATABASE_URL" -v ON_ERROR_STOP=1 -c "INSERT INTO inventory_movements(business_id,branch_id,product_id,quantity_delta,movement_type,created_by) VALUES ('$BUSINESS_ID','$BRANCH_ID','$INGREDIENT_ID',1000,'opening','$OWNER_USER_ID');" >/dev/null
RECIPE_CHECKOUT_KEY="ci-recipe-checkout-$(date +%s)-$$"
status=$(curl -sS -o "$TMP_DIR/recipe-checkout-body" -w '%{http_code}' -b "$COOKIE_JAR" -c "$COOKIE_JAR" \
  -X POST "$BASE_URL/api/v1/sales/checkout" -H "Origin: $WEB_ORIGIN" \
  -H 'Content-Type: application/json' -H "Idempotency-Key: $RECIPE_CHECKOUT_KEY" \
  --data "{\"payment_method\":\"cash\",\"lines\":[{\"product_id\":\"$OUTPUT_ID\",\"quantity\":2}]}")
[ "$status" = "201" ] || { echo "FAIL recipe checkout expected HTTP 201, got $status" >&2; cat "$TMP_DIR/recipe-checkout-body" >&2; exit 1; }
jq -e '.stock_deduction.status == "recipe_backed_items_deducted" and .stock_deduction.ingredient_movements == 1' "$TMP_DIR/recipe-checkout-body" >/dev/null
RECIPE_SALE_ID=$(jq -r '.sale_id' "$TMP_DIR/recipe-checkout-body")
RECIPE_STOCK=$(psql "$MIGRATION_DATABASE_URL" -v ON_ERROR_STOP=1 -Atqc "SELECT COALESCE(sum(quantity_delta),0) FROM inventory_movements WHERE business_id='$BUSINESS_ID' AND branch_id='$BRANCH_ID' AND product_id='$INGREDIENT_ID';")
[ "$RECIPE_STOCK" = "800.00000000" ] || { echo "FAIL recipe stock deduction expected 800, got $RECIPE_STOCK" >&2; exit 1; }
RECIPE_MOVEMENTS=$(psql "$MIGRATION_DATABASE_URL" -v ON_ERROR_STOP=1 -Atqc "SELECT count(*) FROM inventory_movements WHERE business_id='$BUSINESS_ID' AND branch_id='$BRANCH_ID' AND reference_id='$RECIPE_SALE_ID' AND movement_type='sale_consumption';")
[ "$RECIPE_MOVEMENTS" = "1" ] || { echo "FAIL recipe movement count: $RECIPE_MOVEMENTS" >&2; exit 1; }
# Insufficient stock must roll back the sale, tender and idempotency row.
LOW_STOCK_KEY="ci-low-stock-$(date +%s)-$$"
status=$(curl -sS -o "$TMP_DIR/low-stock-body" -w '%{http_code}' -b "$COOKIE_JAR" -c "$COOKIE_JAR" \
  -X POST "$BASE_URL/api/v1/sales/checkout" -H "Origin: $WEB_ORIGIN" \
  -H 'Content-Type: application/json' -H "Idempotency-Key: $LOW_STOCK_KEY" \
  --data "{\"payment_method\":\"cash\",\"lines\":[{\"product_id\":\"$OUTPUT_ID\",\"quantity\":9}]}")
[ "$status" = "409" ] || { echo "FAIL insufficient stock expected HTTP 409, got $status" >&2; cat "$TMP_DIR/low-stock-body" >&2; exit 1; }
LOW_STOCK_ROWS=$(psql "$MIGRATION_DATABASE_URL" -v ON_ERROR_STOP=1 -Atqc "SELECT count(*) FROM checkout_idempotency WHERE business_id='$BUSINESS_ID' AND idempotency_key='$LOW_STOCK_KEY';")
[ "$LOW_STOCK_ROWS" = "0" ] || { echo "FAIL insufficient-stock checkout left idempotency row: $LOW_STOCK_ROWS" >&2; exit 1; }

# Invitation creation commits the invitation and its email payload to the outbox together.
INVITEE_EMAIL="api-invite-$(date +%s)-$$@example.test"
request POST /api/v1/staff/invitations "{\"email\":\"$INVITEE_EMAIL\",\"role\":\"cashier\",\"branch_ids\":[\"$BRANCH_ID\"]}" 201
INVITATION_ID=$(jq -r '.invitation_id' "$TMP_DIR/body")
[ -n "$INVITATION_ID" ] && [ "$INVITATION_ID" != null ]
OUTBOX_ROWS=$(psql "$MIGRATION_DATABASE_URL" -v ON_ERROR_STOP=1 -Atqc "SELECT count(*) FROM email_outbox WHERE business_id='$BUSINESS_ID' AND recipient='$INVITEE_EMAIL' AND status IN ('pending','sending') AND body LIKE '%accept-invitation%';")
[ "$OUTBOX_ROWS" = "1" ] || { echo "FAIL transactional invitation outbox enqueue: $OUTBOX_ROWS" >&2; exit 1; }
printf '%s\n' 'Transactional staff invitation outbox enqueue passed.'

# Manager PIN authorization over HTTP. Two independently registered users
# are attached to tenant A through the staff-management API.
EMAIL_C="api-smoke-c-$(date +%s)-$$@example.test"
COOKIE_JAR_C="$TMP_DIR/cookies-owner-c.txt"
status=$(curl -sS -o "$TMP_DIR/body-c" -w '%{http_code}' -c "$COOKIE_JAR_C" \
  -X POST "$BASE_URL/api/v1/auth/register" -H "Origin: $WEB_ORIGIN" \
  -H 'Content-Type: application/json' \
  --data "{\"full_name\":\"API Smoke Owner C\",\"email\":\"$EMAIL_C\",\"password\":\"SmokePassword123\",\"business_name\":\"API Smoke Bar C\",\"branch_name\":\"Main\"}")
if [ "$status" != "201" ]; then echo "FAIL third tenant registration: HTTP $status" >&2; cat "$TMP_DIR/body-c" >&2; exit 1; fi
verify_registration "$EMAIL_C" "$COOKIE_JAR_C"
request POST /api/v1/staff/memberships "{\"email\":\"missing-$EMAIL_B\",\"role\":\"manager\",\"branch_ids\":[\"$BRANCH_ID\"]}" 404
request POST /api/v1/staff/memberships "{\"email\":\"$EMAIL_B\",\"role\":\"manager\",\"branch_ids\":[\"$BRANCH_B_ID\"]}" 422
request POST /api/v1/staff/memberships "{\"email\":\"$EMAIL_B\",\"role\":\"manager\",\"branch_ids\":[\"$BRANCH_ID\"]}" 201
MANAGER_B_ID=$(jq -r '.membership_id' "$TMP_DIR/body")
request POST /api/v1/staff/memberships "{\"email\":\"$EMAIL_B\",\"role\":\"manager\",\"branch_ids\":[\"$BRANCH_ID\"]}" 409
request POST /api/v1/staff/memberships "{\"email\":\"$EMAIL_C\",\"role\":\"manager\",\"branch_ids\":[\"$BRANCH_ID\"]}" 201
MANAGER_C_ID=$(jq -r '.membership_id' "$TMP_DIR/body")
for manager_id in "$MANAGER_B_ID" "$MANAGER_C_ID"; do
  request POST /api/v1/staff/profiles "{\"membership_id\":\"$manager_id\",\"display_name\":\"CI Manager\"}" 201
  request PUT "/api/v1/staff/$manager_id/role" '{"role":"manager"}' 200
  request PUT "/api/v1/staff/$manager_id/branches" "{\"branch_ids\":[\"$BRANCH_ID\"]}" 200
  request PUT "/api/v1/staff/$manager_id/manager-pin" '{"pin":"2468"}' 200
done
# The manager membership is branch-scoped in tenant A; authenticate that user separately.
manager_request() {
  method=$1 path=$2 data=${3-} expected=$4
  if [ -n "$data" ]; then
    status=$(curl -sS -o "$TMP_DIR/manager-body" -w '%{http_code}' -b "$COOKIE_JAR_B" -c "$COOKIE_JAR_B" -X "$method" "$BASE_URL$path" -H "Origin: $WEB_ORIGIN" -H 'Content-Type: application/json' --data "$data")
  else
    status=$(curl -sS -o "$TMP_DIR/manager-body" -w '%{http_code}' -b "$COOKIE_JAR_B" -c "$COOKIE_JAR_B" -X "$method" "$BASE_URL$path" -H "Origin: $WEB_ORIGIN")
  fi
  if [ "$status" != "$expected" ]; then echo "FAIL manager $method $path: expected HTTP $expected, got $status" >&2; cat "$TMP_DIR/manager-body" >&2; exit 1; fi
}
manager_request POST /api/v1/auth/login "{\"email\":\"$EMAIL_B\",\"password\":\"SmokePassword123\"}" 200
manager_request POST /api/v1/tenant/select "{\"business_id\":\"$BUSINESS_ID\",\"branch_id\":\"$BRANCH_ID\"}" 200
request GET /api/v1/expenses/categories '' 200
EXPENSE_CATEGORY_ID=$(jq -r '.items[0].id' "$TMP_DIR/body")
request POST /api/v1/expenses/ "{\"category_id\":\"$EXPENSE_CATEGORY_ID\",\"description\":\"CI cash expense\",\"amount_minor\":10000,\"currency\":\"KES\",\"expense_date\":\"$(date +%F)\"}" 201
EXPENSE_ID=$(jq -r '.id' "$TMP_DIR/body")
request POST "/api/v1/expenses/$EXPENSE_ID/review" '{"decision":"approve"}' 403
manager_request POST "/api/v1/expenses/$EXPENSE_ID/review" '{"decision":"approve"}' 200
manager_request POST /api/v1/shifts/ '{"opening_float_minor":100000}' 201
EXPENSE_SHIFT_ID=$(jq -r '.shift_id' "$TMP_DIR/manager-body")
manager_request POST "/api/v1/expenses/$EXPENSE_ID/pay" '{"payment_method":"cash","payment_reference":"CI-CASH-EXPENSE"}' 200
jq -e '.status == "paid" and .payment_method == "cash" and .amount_minor == 10000' "$TMP_DIR/manager-body" >/dev/null
manager_request POST "/api/v1/shifts/$EXPENSE_SHIFT_ID/close" '{"counted_cash_minor":90000}' 200
jq -e '.expected_cash_minor == 90000 and .variance_minor == 0' "$TMP_DIR/manager-body" >/dev/null
manager_request GET "/api/v1/expenses/$EXPENSE_ID/reviews" '' 200
jq -e '.items | length == 1 and .[0].decision == "approved"' "$TMP_DIR/manager-body" >/dev/null
# A non-cash expense payment must not be converted into a drawer paid-out movement.
request POST /api/v1/expenses/ "{\"category_id\":\"$EXPENSE_CATEGORY_ID\",\"description\":\"CI bank expense\",\"amount_minor\":5000,\"currency\":\"KES\",\"expense_date\":\"$(date +%F)\"}" 201
BANK_EXPENSE_ID=$(jq -r '.id' "$TMP_DIR/body")
manager_request POST "/api/v1/expenses/$BANK_EXPENSE_ID/review" '{"decision":"approve"}' 200
manager_request POST /api/v1/shifts/ '{"opening_float_minor":0}' 201
ZERO_SHIFT_ID=$(jq -r '.shift_id' "$TMP_DIR/manager-body")
manager_request POST "/api/v1/expenses/$BANK_EXPENSE_ID/pay" '{"payment_method":"cash"}' 409
manager_request POST "/api/v1/expenses/$BANK_EXPENSE_ID/pay" '{"payment_method":"bank_transfer","payment_reference":"CI-BANK-EXPENSE"}' 200
manager_request POST "/api/v1/shifts/$ZERO_SHIFT_ID/close" '{"counted_cash_minor":0}' 200
jq -e '.expected_cash_minor == 0 and .paid_out_minor == 0 and .variance_minor == 0' "$TMP_DIR/manager-body" >/dev/null
manager_request GET "/api/v1/expenses/reports/summary?from=$(date +%F)&to=$(date +%F)" '' 200
jq -e '.items | length >= 2' "$TMP_DIR/manager-body" >/dev/null
printf '%s\n' 'Expense approval, creator separation, cash/non-cash payment, insufficient-drawer rollback, reporting and reconciliation integration passed.'
SALE_ID=$(psql "$MIGRATION_DATABASE_URL" -v ON_ERROR_STOP=1 -Atqc "INSERT INTO sales(business_id,branch_id,sale_number,total_minor,created_by) VALUES ('$BUSINESS_ID','$BRANCH_ID','CI-SALE-$(date +%s)-$$',1500,'$OWNER_USER_ID') RETURNING id;")
OTHER_SALE_ID=$(psql "$MIGRATION_DATABASE_URL" -v ON_ERROR_STOP=1 -Atqc "INSERT INTO sales(business_id,branch_id,sale_number,total_minor,created_by) VALUES ('$BUSINESS_ID','$BRANCH_ID','CI-SALE-OTHER-$(date +%s)-$$',2500,'$OWNER_USER_ID') RETURNING id;")
request POST /api/v1/staff/authorize-action "{\"manager_membership_id\":\"$MANAGER_B_ID\",\"pin\":\"2468\",\"action\":\"sales.void\",\"target_id\":\"$SALE_ID\"}" 200
APPROVAL_TOKEN=$(jq -r '.approval_token' "$TMP_DIR/body")
jq -e '.authorized == true and (.approval_token | type == "string" and length == 64) and .single_use == true' "$TMP_DIR/body" >/dev/null
# Approval is target-bound; a mismatch must not consume the token.
request POST "/api/v1/sales/$OTHER_SALE_ID/void" "{\"approval_token\":\"$APPROVAL_TOKEN\",\"reason\":\"CI target mismatch test\"}" 403
request POST "/api/v1/sales/$SALE_ID/void" "{\"approval_token\":\"$APPROVAL_TOKEN\",\"reason\":\"CI approval integration test\"}" 200
jq -e --arg id "$SALE_ID" '.sale_id == $id and .status == "voided" and .total_minor == 1500' "$TMP_DIR/body" >/dev/null
request POST "/api/v1/sales/$SALE_ID/void" "{\"approval_token\":\"$APPROVAL_TOKEN\",\"reason\":\"CI replay test\"}" 403
# Manager-approved partial refund; amount ceiling and replay are enforced transactionally.
REFUND_SALE_ID=$(psql "$MIGRATION_DATABASE_URL" -v ON_ERROR_STOP=1 -Atqc "INSERT INTO sales(business_id,branch_id,sale_number,total_minor,created_by) VALUES ('$BUSINESS_ID','$BRANCH_ID','CI-REFUND-$(date +%s)-$$',1500,'$OWNER_USER_ID') RETURNING id;")
request POST /api/v1/staff/authorize-action "{\"manager_membership_id\":\"$MANAGER_B_ID\",\"pin\":\"2468\",\"action\":\"sales.refund\",\"target_id\":\"$REFUND_SALE_ID\",\"target_amount_minor\":600}" 200
REFUND_TOKEN=$(jq -r '.approval_token' "$TMP_DIR/body")
request POST "/api/v1/sales/$REFUND_SALE_ID/refunds" "{\"approval_token\":\"$REFUND_TOKEN\",\"amount_minor\":1000,\"reason\":\"CI amount binding mismatch\"}" 403
request POST "/api/v1/sales/$REFUND_SALE_ID/refunds" "{\"approval_token\":\"$REFUND_TOKEN\",\"amount_minor\":600,\"reason\":\"CI partial refund test\"}" 201
jq -e '.amount_minor == 600 and .refunded_total_minor == 600 and .status == "recorded_pending_settlement"' "$TMP_DIR/body" >/dev/null
request POST "/api/v1/sales/$REFUND_SALE_ID/refunds" "{\"approval_token\":\"$REFUND_TOKEN\",\"amount_minor\":100,\"reason\":\"CI replay refund test\"}" 403
request POST /api/v1/staff/authorize-action "{\"manager_membership_id\":\"$MANAGER_B_ID\",\"pin\":\"2468\",\"action\":\"sales.refund\",\"target_id\":\"$REFUND_SALE_ID\",\"target_amount_minor\":1000}" 200
REFUND_TOKEN_2=$(jq -r '.approval_token' "$TMP_DIR/body")
request POST "/api/v1/sales/$REFUND_SALE_ID/refunds" "{\"approval_token\":\"$REFUND_TOKEN_2\",\"amount_minor\":1000,\"reason\":\"CI remaining refund test\"}" 201
request POST /api/v1/staff/authorize-action "{\"manager_membership_id\":\"$MANAGER_B_ID\",\"pin\":\"2468\",\"action\":\"sales.refund\",\"target_id\":\"$REFUND_SALE_ID\",\"target_amount_minor\":1}" 200
REFUND_TOKEN_3=$(jq -r '.approval_token' "$TMP_DIR/body")
request POST "/api/v1/sales/$REFUND_SALE_ID/refunds" "{\"approval_token\":\"$REFUND_TOKEN_3\",\"amount_minor\":1,\"reason\":\"CI over refund test\"}" 409

# Inventory count snapshots ledger quantity; approval consumption and adjustment movements commit atomically.
request POST /api/v1/inventory/counts/ "{\"lines\":[{\"product_id\":\"$INGREDIENT_ID\",\"counted_quantity\":\"25.5\"}]}" 201
COUNT_ID=$(jq -r '.count_id' "$TMP_DIR/body")
request POST /api/v1/staff/authorize-action "{\"manager_membership_id\":\"$MANAGER_B_ID\",\"pin\":\"2468\",\"action\":\"inventory.count.approve\",\"target_id\":\"$COUNT_ID\"}" 200
COUNT_TOKEN=$(jq -r '.approval_token' "$TMP_DIR/body")
request POST "/api/v1/inventory/counts/$COUNT_ID/reconcile" "{\"approval_token\":\"$COUNT_TOKEN\"}" 200
jq -e '.status == "reconciled" and .line_count == 1' "$TMP_DIR/body" >/dev/null
request POST "/api/v1/inventory/counts/$COUNT_ID/reconcile" "{\"approval_token\":\"$COUNT_TOKEN\"}" 409
ACTUAL_STOCK=$(psql "$MIGRATION_DATABASE_URL" -v ON_ERROR_STOP=1 -Atqc "SELECT COALESCE(sum(quantity_delta),0) FROM inventory_movements WHERE business_id='$BUSINESS_ID' AND branch_id='$BRANCH_ID' AND product_id='$INGREDIENT_ID';")
[ "$ACTUAL_STOCK" = "25.50000000" ] || { echo "FAIL inventory reconciliation balance: $ACTUAL_STOCK" >&2; exit 1; }
printf '%s\n' 'Manager-approved refunds and inventory count reconciliation passed.'

# Wrong PIN responses intentionally remain HTTP 200 so failed-attempt state commits.
for attempt in 1 2 3 4 5; do
  request POST /api/v1/staff/authorize-action "{\"manager_membership_id\":\"$MANAGER_C_ID\",\"pin\":\"9999\",\"action\":\"sales.void\"}" 200
  jq -e '.authorized == false' "$TMP_DIR/body" >/dev/null
done
request POST /api/v1/staff/authorize-action "{\"manager_membership_id\":\"$MANAGER_C_ID\",\"pin\":\"2468\",\"action\":\"sales.void\"}" 403
printf '%s\n' 'Manager PIN and sale-void HTTP smoke test passed: target binding, atomic consumption, replay rejection, five-failure lockout and locked-PIN denial.'
