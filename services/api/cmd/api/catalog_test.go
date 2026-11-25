package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCatalogInputValidation(t *testing.T) {
	tests := []struct {
		name, value string
		max         int
		want        bool
	}{
		{"valid", "Gin", 80, true}, {"trimmed", "  Lager  ", 80, true},
		{"empty", "   ", 80, false}, {"too long", "12345", 4, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := requiredText(tt.value, tt.max); got != tt.want {
				t.Fatalf("requiredText()=%v want %v", got, tt.want)
			}
		})
	}
}

func TestCatalogPermissions(t *testing.T) {
	tests := []struct {
		role, permission string
		want             bool
	}{
		{"owner", "products.manage", true}, {"owner", "staff.manage", true}, {"manager", "staff.read", true}, {"manager", "staff.manage", false}, {"cashier", "staff.read", false}, {"manager", "products.read", true},
		{"manager", "products.manage", false}, {"cashier", "products.read", true},
		{"cashier", "products.manage", false}, {"bartender", "tables.manage", true},
		{"stock_controller", "products.manage", true}, {"unknown", "products.read", false},
	}
	for _, tt := range tests {
		t.Run(tt.role+"/"+tt.permission, func(t *testing.T) {
			if got := hasPermission(tt.role, tt.permission); got != tt.want {
				t.Fatalf("hasPermission()=%v want %v", got, tt.want)
			}
		})
	}
}

func TestResponseBufferFlush(t *testing.T) {
	b := newResponseBuffer()
	b.Header().Set("Content-Type", "application/json")
	b.WriteHeader(201)
	_, _ = b.Write([]byte(`{"ok":true}`))
	rr := httptest.NewRecorder()
	b.flushTo(rr)
	if rr.Code != 201 {
		t.Fatalf("status=%d want 201", rr.Code)
	}
	if rr.Body.String() != `{"ok":true}` {
		t.Fatalf("body=%q", rr.Body.String())
	}
	if rr.Header().Get("Content-Type") != "application/json" {
		t.Fatal("content type was not preserved")
	}
}

func TestListBounds(t *testing.T) {
	tests := []struct {
		query                 string
		wantOK                bool
		wantLimit, wantOffset int
	}{
		{"", true, 50, 0}, {"?limit=100&offset=25&q=gin", true, 100, 25}, {"?limit=101", false, 0, 0}, {"?limit=0", false, 0, 0}, {"?offset=-1", false, 0, 0}, {"?limit=x", false, 0, 0}, {"?offset=1000001", false, 0, 0}, {"?q=" + strings.Repeat("x", 101), false, 0, 0},
	}
	for _, tt := range tests {
		r := httptest.NewRequest("GET", "/api/v1/catalog/products"+tt.query, nil)
		l, o, _, ok := listBounds(r)
		if ok != tt.wantOK || (ok && (l != tt.wantLimit || o != tt.wantOffset)) {
			t.Errorf("listBounds(%q)=(%d,%d,%v), want (%d,%d,%v)", tt.query, l, o, ok, tt.wantLimit, tt.wantOffset, tt.wantOK)
		}
	}
}

func TestManagerActionAllowlist(t *testing.T) {
	allowed := []string{"discount.approve", "sales.void", "sales.refund", "inventory.count.approve"}
	for _, action := range allowed {
		if !managerActions[action] {
			t.Errorf("expected %q to be an allowed manager action", action)
		}
	}
	for _, action := range []string{"staff.manage", "products.manage", "", "sales.delete"} {
		if managerActions[action] {
			t.Errorf("unexpectedly allowed manager action %q", action)
		}
	}
}

func TestManagerAuthorizationRequestValidation(t *testing.T) {
	valid := managerAuthorizationInput{ManagerMembershipID: "94000000-0000-4000-8000-000000000001", PIN: "1234", Action: "sales.void", TargetID: "93000000-0000-4000-8000-000000000001"}
	if !validManagerAuthorizationInput(valid) {
		t.Fatal("valid manager authorization request rejected")
	}
	invalid := []managerAuthorizationInput{
		{ManagerMembershipID: "not-a-uuid", PIN: "1234", Action: "sales.void"},
		{ManagerMembershipID: valid.ManagerMembershipID, PIN: "12", Action: "sales.void"},
		{ManagerMembershipID: valid.ManagerMembershipID, PIN: "12a4", Action: "sales.void"},
		{ManagerMembershipID: valid.ManagerMembershipID, PIN: "1234", Action: "users.promote"},
		{ManagerMembershipID: valid.ManagerMembershipID, PIN: "1234", Action: "sales.void", TargetID: "not-a-uuid"},
	}
	for i, in := range invalid {
		if validManagerAuthorizationInput(in) {
			t.Errorf("invalid authorization request %d passed validation", i)
		}
	}
}

func TestValidUUID(t *testing.T) {
	if !validUUID("94000000-0000-4000-8000-000000000001") {
		t.Fatal("canonical UUID rejected")
	}
	for _, value := range []string{"", "abc", "94000000-0000-4000-8000-00000000000z", "94000000000040008000000000000001"} {
		if validUUID(value) {
			t.Errorf("invalid UUID accepted: %q", value)
		}
	}
}

func TestManagerApprovalTokenGenerationAndValidation(t *testing.T) {
	token, digest, err := newManagerApprovalToken()
	if err != nil {
		t.Fatalf("newManagerApprovalToken: %v", err)
	}
	if len(token) != 64 || len(digest) != 32 {
		t.Fatalf("unexpected token/digest lengths: %d/%d", len(token), len(digest))
	}
	got, ok := managerApprovalTokenDigest(token)
	if !ok || string(got) != string(digest) {
		t.Fatal("generated token did not reproduce its stored digest")
	}
	if _, ok := managerApprovalTokenDigest("not-a-token"); ok {
		t.Fatal("malformed token accepted")
	}
	if _, ok := managerApprovalTokenDigest(strings.Repeat("z", 64)); ok {
		t.Fatal("non-hex token accepted")
	}
	if _, ok := managerApprovalTokenDigest(strings.Repeat("a", 63)); ok {
		t.Fatal("short token accepted")
	}
}

func TestRefundApprovalRequiresExactPositiveAmount(t *testing.T) {
	base := managerAuthorizationInput{ManagerMembershipID: "94000000-0000-4000-8000-000000000001", PIN: "1234", Action: "sales.refund", TargetID: "93000000-0000-4000-8000-000000000001"}
	if validManagerAuthorizationInput(base) {
		t.Fatal("refund approval without an amount was accepted")
	}
	amount := int64(1250)
	base.TargetAmountMinor = &amount
	if !validManagerAuthorizationInput(base) {
		t.Fatal("refund approval with a positive amount was rejected")
	}
	amount = 0
	if validManagerAuthorizationInput(base) {
		t.Fatal("zero-value refund approval was accepted")
	}
}

func TestInventoryQuantityInputIsExactDecimal(t *testing.T) {
	for _, value := range []string{"0", "25", "25.5", "0.00000001"} {
		if !inventoryQuantityPattern.MatchString(value) {
			t.Errorf("valid inventory quantity rejected: %q", value)
		}
	}
	for _, value := range []string{"", "-1", "+1", ".5", "1.", "1e3", "NaN", "1.123456789"} {
		if inventoryQuantityPattern.MatchString(value) {
			t.Errorf("invalid inventory quantity accepted: %q", value)
		}
	}
}
