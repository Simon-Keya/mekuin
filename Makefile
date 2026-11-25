.PHONY: check-go test vet build fmt-check db-up db-rls-smoke db-catalog-smoke db-manager-auth-smoke db-staff-invitation-smoke db-finance-inventory-smoke db-all-smoke

check-go:
	@command -v go >/dev/null || (echo "Go 1.24.13 is required. Run: mise install" && exit 1)
	@go version | grep -Eq 'go1\.24\.13([.]| )' || (echo "Expected Go 1.24.13; found: $$(go version)" && exit 1)

test: check-go
	cd services/api && go test ./...

vet: check-go
	cd services/api && go vet ./...

build: check-go
	cd services/api && CGO_ENABLED=0 go build -trimpath ./cmd/api

fmt-check: check-go
	@test -z "$$(gofmt -l services/api/cmd/api)" || (gofmt -l services/api/cmd/api; exit 1)

db-up:
	docker compose run --rm migrate

db-rls-smoke:
	psql "$$MIGRATION_DATABASE_URL" -v ON_ERROR_STOP=1 -f db/tests/tenant_rls_smoke.sql

db-catalog-smoke:
	psql "$$MIGRATION_DATABASE_URL" -v ON_ERROR_STOP=1 -f db/tests/catalog_rls_smoke.sql

db-manager-auth-smoke:
	psql "$$MIGRATION_DATABASE_URL" -v ON_ERROR_STOP=1 -f db/tests/manager_authorization_rls_smoke.sql

db-staff-invitation-smoke:
	psql "$$MIGRATION_DATABASE_URL" -v ON_ERROR_STOP=1 -f db/tests/staff_invitation_rls_smoke.sql

db-finance-inventory-smoke:
	psql "$$MIGRATION_DATABASE_URL" -v ON_ERROR_STOP=1 -f db/tests/refunds_inventory_outbox_rls_smoke.sql

db-all-smoke: db-rls-smoke db-catalog-smoke db-manager-auth-smoke db-staff-invitation-smoke db-finance-inventory-smoke
