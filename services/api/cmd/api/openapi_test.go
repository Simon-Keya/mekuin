package main

import (
	"encoding/json"
	"strings"
	"testing"
)

type openAPIDocument struct {
	Paths map[string]map[string]struct {
		Security    []map[string][]string `json:"security"`
		RequestBody *struct {
			Required bool `json:"required"`
			Content  map[string]struct {
				Schema map[string]any `json:"schema"`
			} `json:"content"`
		} `json:"requestBody"`
		Responses map[string]struct {
			Content map[string]struct {
				Schema map[string]any `json:"schema"`
			} `json:"content"`
		} `json:"responses"`
	} `json:"paths"`
	Components struct {
		Schemas map[string]json.RawMessage `json:"schemas"`
	} `json:"components"`
}

func TestOpenAPIContractCoversRegisteredPhaseTwoRoutes(t *testing.T) {
	var doc openAPIDocument
	if err := json.Unmarshal(phase2OpenAPI, &doc); err != nil {
		t.Fatalf("OpenAPI JSON: %v", err)
	}
	required := map[string][]string{
		"/auth/register": {"post"}, "/auth/login": {"post"}, "/auth/google": {"post"}, "/auth/email/verify": {"post"}, "/auth/email/resend": {"post"}, "/auth/password/forgot": {"post"}, "/auth/password/reset": {"post"}, "/auth/mfa/verify": {"post"}, "/auth/mfa/status": {"get"}, "/auth/mfa/setup": {"post"}, "/auth/mfa/enable": {"post"}, "/auth/mfa/disable": {"post"}, "/auth/me": {"get"}, "/auth/logout": {"post"},
		"/workspace": {"get"}, "/tenant/select": {"post"}, "/branches/": {"get", "post"}, "/branches/{branchID}": {"put"}, "/staff/": {"get"},
		"/staff/memberships": {"post"}, "/staff/profiles": {"post"},
		"/staff/invitations": {"post"}, "/staff/invitations/{invitationID}/revoke": {"post"},
		"/auth/invitations/register": {"post"}, "/auth/invitations/accept": {"post"},
		"/staff/{membershipID}/branches": {"put"}, "/staff/{membershipID}/role": {"put"},
		"/staff/{membershipID}/manager-pin": {"put"}, "/staff/authorize-action": {"post"}, "/sales/{saleID}/void": {"post"},
		"/sales/{saleID}/refunds": {"post"}, "/payments/mpesa/": {"get"}, "/payments/mpesa/{attemptID}/review-notes": {"post"}, "/payments/mpesa/{attemptID}/reconcile": {"post"}, "/payments/mpesa/{attemptID}/settle": {"post"}, "/sales/quote": {"post"}, "/sales/checkout": {"post"},
		"/sales/orders": {"get", "post"}, "/sales/orders/{orderID}/checkout": {"post"}, "/sales/orders/{orderID}": {"get"}, "/sales/orders/{orderID}/hold": {"post"}, "/sales/orders/{orderID}/reopen": {"post"}, "/sales/orders/{orderID}/cancel": {"post"}, "/inventory/counts/": {"get", "post"}, "/inventory/counts/{countID}": {"get", "patch"}, "/inventory/counts/{countID}/submit": {"post"}, "/inventory/counts/{countID}/reconcile": {"post"},
		"/inventory/valuation": {"get"}, "/reports/summary": {"get"}, "/settings/integrations": {"get"}, "/settings/integrations/{provider}": {"put"},
		"/inventory/balances": {"get"}, "/inventory/branches": {"get"}, "/inventory/reconciliation": {"get"}, "/inventory/movements": {"get"}, "/inventory/adjustments": {"post"}, "/inventory/losses/{kind}": {"post"}, "/inventory/transfers": {"post"},
		"/expenses/categories": {"get", "post"}, "/expenses/": {"get", "post"}, "/expenses/{expenseID}/reviews": {"get"}, "/expenses/{expenseID}/review": {"post"}, "/expenses/{expenseID}/pay": {"post"}, "/expenses/reports/summary": {"get"},
		"/alerts/": {"get"}, "/alerts/{alertID}/action": {"post"}, "/audit/": {"get"},
		"/shifts/current": {"get"}, "/shifts/": {"post"}, "/shifts/{shiftID}/close": {"post"}, "/shifts/{shiftID}/movements": {"post"}, "/shifts/{shiftID}/summary": {"get"},
		"/catalog/categories": {"get", "post"}, "/catalog/categories/{id}": {"patch"},
		"/catalog/categories/{id}/deactivate": {"post"}, "/catalog/units": {"get", "post"},
		"/catalog/units/{id}/deactivate": {"post"}, "/catalog/unit-conversions": {"get", "post"},
		"/catalog/products": {"get", "post"}, "/catalog/products/{id}": {"patch"},
		"/catalog/products/{id}/deactivate": {"post"}, "/catalog/suppliers": {"get", "post"},
		"/catalog/suppliers/{id}": {"patch"}, "/catalog/suppliers/{id}/deactivate": {"post"},
		"/catalog/tables": {"get", "post"}, "/catalog/tables/{id}": {"patch"},
		"/catalog/tables/{id}/deactivate": {"post"}, "/catalog/recipes": {"get", "post"},
		"/catalog/recipes/{id}/cost": {"get"},
	}
	for path, methods := range required {
		for _, method := range methods {
			op, ok := doc.Paths[path][method]
			if !ok {
				t.Errorf("OpenAPI is missing %s %s", strings.ToUpper(method), path)
				continue
			}
			if len(op.Responses) == 0 {
				t.Errorf("%s %s has no response contracts", strings.ToUpper(method), path)
			}
			if path != "/auth/register" && path != "/auth/login" && path != "/auth/google" && path != "/auth/email/verify" && path != "/auth/email/resend" && path != "/auth/password/forgot" && path != "/auth/password/reset" && path != "/auth/mfa/verify" && path != "/auth/invitations/register" && len(op.Security) == 0 {
				t.Errorf("%s %s is missing cookie authentication", strings.ToUpper(method), path)
			}
			if op.RequestBody != nil {
				if !op.RequestBody.Required {
					t.Errorf("%s %s request body must be marked required", strings.ToUpper(method), path)
				}
				for media, body := range op.RequestBody.Content {
					if media != "application/json" {
						continue
					}
					if ref, ok := body.Schema["$ref"].(string); ok {
						name := strings.TrimPrefix(ref, "#/components/schemas/")
						if _, exists := doc.Components.Schemas[name]; !exists {
							t.Errorf("%s %s references missing schema %s", method, path, name)
						}
					}
				}
			}
			for code, response := range op.Responses {
				if code == "default" || strings.HasPrefix(code, "4") || strings.HasPrefix(code, "5") {
					content, ok := response.Content["application/json"]
					if !ok {
						t.Errorf("%s %s error response %s is missing application/json content", method, path, code)
						continue
					}
					if ref, ok := content.Schema["$ref"].(string); !ok || ref != "#/components/schemas/Error" {
						t.Errorf("%s %s error response %s must use Error schema", method, path, code)
					}
				}
			}
		}
	}
	for path := range doc.Paths {
		if len(doc.Paths[path]) == 0 {
			t.Errorf("OpenAPI path %s has no operations", path)
		}
	}
	if _, ok := doc.Components.Schemas["Error"]; !ok {
		t.Fatal("common Error schema is missing")
	}
}
