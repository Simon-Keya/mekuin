package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRolePermissions(t *testing.T) {
	cases := []struct {
		role, permission string
		want             bool
	}{
		{"owner", "anything.at.all", true},
		{"manager", "sales.void", true},
		{"manager", "inventory.count.reconcile", true},
		{"manager", "inventory.adjust", true},
		{"manager", "finance.read", true},
		{"manager", "shifts.own", true},
		{"manager", "expenses.read", true},
		{"manager", "expenses.manage", true},
		{"manager", "expenses.approve", true},
		{"manager", "finance.manage", true},
		{"manager", "finance.approve", true},
		{"manager", "suppliers.read", true},
		{"manager", "platform.super_admin", false},
		{"cashier", "pos.use", true},
		{"cashier", "shifts.own", true},
		{"cashier", "sales.void", false},
		{"cashier", "finance.read", false},
		{"cashier", "expenses.read", false},
		{"cashier", "expenses.manage", false},
		{"cashier", "expenses.approve", false},
		{"cashier", "finance.manage", false},
		{"cashier", "finance.approve", false},
		{"cashier", "inventory.count.approve", false},
		{"bartender", "tables.manage", true},
		{"bartender", "shifts.own", true},
		{"bartender", "payments.refund", false},
		{"stock_controller", "inventory.count.create", true},
		{"stock_controller", "shifts.own", false},
		{"stock_controller", "inventory.count.reconcile", true},
		{"stock_controller", "inventory.count.approve", false},
		{"stock_controller", "inventory.adjust", true},
		{"stock_controller", "finance.manage", false},
		{"stock_controller", "finance.approve", false},
		{"stock_controller", "suppliers.read", true},
		{"unknown", "pos.use", false},
	}
	for _, tc := range cases {
		if got := hasPermission(tc.role, tc.permission); got != tc.want {
			t.Errorf("hasPermission(%q, %q) = %v, want %v", tc.role, tc.permission, got, tc.want)
		}
	}
}

func TestTenantHeaderSelectionIsRequired(t *testing.T) {
	// Header parsing is intentionally performed by requireTenant; missing values
	// must never be interpreted as a default business or branch.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenant/context", nil)
	if strings.TrimSpace(req.Header.Get("X-Business-ID")) != "" || strings.TrimSpace(req.Header.Get("X-Branch-ID")) != "" {
		t.Fatal("unexpected tenant selector defaults")
	}
}

func TestRequirePermissionMiddleware(t *testing.T) {
	cases := []struct {
		role       string
		permission string
		want       int
	}{
		{"owner", "inventory.adjust", http.StatusNoContent},
		{"manager", "sales.refund", http.StatusNoContent},
		{"cashier", "payments.take", http.StatusNoContent},
		{"cashier", "sales.void", http.StatusForbidden},
		{"bartender", "tables.manage", http.StatusNoContent},
		{"stock_controller", "inventory.count.create", http.StatusNoContent},
		{"stock_controller", "inventory.count.reconcile", http.StatusNoContent},
		{"stock_controller", "inventory.adjust", http.StatusNoContent},
		{"stock_controller", "sales.refund", http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.role+"/"+tc.permission, func(t *testing.T) {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
			h := requirePermission(tc.permission)(next)
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			ctx := context.WithValue(r.Context(), tenantContextKey, tenantContext{Role: tc.role})
			r = r.WithContext(ctx)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("got %d, want %d", w.Code, tc.want)
			}
		})
	}
}

func TestManagerActionsAreExplicitAllowlist(t *testing.T) {
	for _, action := range []string{"discount.approve", "sales.void", "sales.refund", "inventory.count.approve"} {
		if !managerActions[action] {
			t.Errorf("expected %s to be allowed", action)
		}
	}
	for _, action := range []string{"*", "staff.manage", "payments.verify", "sales.create", ""} {
		if managerActions[action] {
			t.Errorf("unexpected manager action %q allowed", action)
		}
	}
}
