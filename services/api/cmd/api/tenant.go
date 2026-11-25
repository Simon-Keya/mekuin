package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrTenantAccessDenied = errors.New("tenant access denied")

// tenantContext is derived from an authenticated user and a database-validated
// membership. Never construct it directly from request JSON or query parameters.
type tenantContext struct {
	UserID       string
	BusinessID   string
	BranchID     string
	MembershipID string
	Role         string
}

// withTenantTx validates membership before setting transaction-local RLS
// context. SET LOCAL settings cannot leak through a pooled connection.
func withTenantTx(ctx context.Context, pool *pgxpool.Pool, userID, businessID, branchID string, fn func(pgx.Tx, tenantContext) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tenant transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var tc tenantContext
	tc.UserID, tc.BusinessID, tc.BranchID = userID, businessID, branchID
	if err := tx.QueryRow(ctx,
		`SELECT membership_id, membership_role FROM resolve_branch_membership($1,$2,$3)`,
		userID, businessID, branchID,
	).Scan(&tc.MembershipID, &tc.Role); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTenantAccessDenied
		}
		return fmt.Errorf("resolve tenant membership: %w", err)
	}

	for key, value := range map[string]string{
		"app.user_id":     userID,
		"app.business_id": businessID,
		"app.branch_id":   branchID,
	} {
		if _, err := tx.Exec(ctx, `SELECT set_config($1,$2,true)`, key, value); err != nil {
			return fmt.Errorf("set tenant context: %w", err)
		}
	}
	if err := fn(tx, tc); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tenant transaction: %w", err)
	}
	return nil
}

// hasPermission is the backend authorization source. Frontend role checks are
// presentation only and must never replace this check in handlers/services.
func hasPermission(role, permission string) bool {
	var allowed map[string]bool
	switch role {
	case "owner":
		allowed = map[string]bool{"*": true}
	case "manager":
		allowed = map[string]bool{
			"pos.use": true, "sales.read": true, "sales.void": true, "sales.refund": true,
			"discount.approve": true, "inventory.read": true, "inventory.adjust": true,
			"inventory.count.approve": true, "inventory.count.reconcile": true, "purchasing.manage": true, "purchasing.receive": true, "finance.read": true, "finance.manage": true, "finance.approve": true, "reports.read": true,
			"staff.read": true, "shifts.manage": true, "shifts.own": true, "expenses.read": true, "expenses.manage": true, "expenses.approve": true,
			"products.read": true, "suppliers.read": true, "tables.manage": true,
		}
	case "cashier":
		allowed = map[string]bool{"pos.use": true, "sales.read.own": true, "payments.take": true, "shifts.own": true, "products.read": true}
	case "bartender":
		allowed = map[string]bool{"pos.use": true, "tables.manage": true, "sales.read.own": true, "shifts.own": true, "products.read": true}
	case "stock_controller":
		allowed = map[string]bool{"inventory.read": true, "inventory.adjust": true, "inventory.count.create": true, "inventory.count.reconcile": true, "purchasing.receive": true, "purchasing.manage": true, "products.read": true, "products.manage": true, "suppliers.read": true, "suppliers.manage": true}
	default:
		return false
	}
	return allowed["*"] || allowed[permission]
}

// requirePermission rejects a request unless the selected membership's role
// grants the requested action. Use inside requireTenant-protected route groups.
func requirePermission(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tc, ok := r.Context().Value(tenantContextKey).(tenantContext)
			if !ok {
				jsonOut(w, http.StatusInternalServerError, map[string]string{"error": "tenant_context_missing"})
				return
			}
			if !hasPermission(tc.Role, permission) {
				jsonOut(w, http.StatusForbidden, map[string]string{"error": "permission_denied", "required_permission": permission})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// tenantTxFromContext returns the request-scoped transaction established by
// requireTenant. Tenant-aware handlers must use this transaction for all SQL
// so PostgreSQL RLS settings remain active on the same pooled connection.
func tenantTxFromContext(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(tenantTxKey).(pgx.Tx)
	return tx, ok
}
