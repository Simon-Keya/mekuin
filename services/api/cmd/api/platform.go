package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

func parsePlatformPage(r *http.Request) (int, int, bool) {
	limit, offset := 50, 0
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			return 0, 0, false
		}
		limit = n
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || n > 1000000 {
			return 0, 0, false
		}
		offset = n
	}
	return limit, offset, true
}

// requireSuperAdmin resolves the platform role from the database on every
// request. It deliberately does not trust a client claim or tenant membership.
func (s *server) requireSuperAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, _ := r.Context().Value(userIDKey).(string)
		if uid == "" {
			jsonOut(w, http.StatusUnauthorized, map[string]string{"error": "authentication_required"})
			return
		}
		var role, status string
		err := s.db.QueryRow(r.Context(), `SELECT platform_role,account_status FROM app_users WHERE id=$1`, uid).Scan(&role, &status)
		if err != nil || !isActiveSuperAdmin(role, status) {
			jsonOut(w, http.StatusForbidden, map[string]string{"error": "platform_permission_denied"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isActiveSuperAdmin(role, status string) bool {
	return role == "super_admin" && status == "active"
}

// listPlatformBusinesses intentionally returns operationally minimal metadata.
// It does not expose customer PII, financial data, or tenant records.
func (s *server) listPlatformBusinesses(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			jsonOut(w, http.StatusBadRequest, map[string]string{"error": "invalid_limit"})
			return
		}
		limit = n
	}
	offset := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || n > 1000000 {
			jsonOut(w, http.StatusBadRequest, map[string]string{"error": "invalid_offset"})
			return
		}
		offset = n
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" && status != "active" && status != "suspended" {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "invalid_business_status"})
		return
	}
	rows, err := s.db.Query(r.Context(), `
		SELECT b.id::text,b.name,b.currency,b.account_status,b.created_at,
		       (SELECT count(*)::bigint FROM branches br WHERE br.business_id=b.id) AS branch_count,
		       (SELECT count(*)::bigint FROM memberships m WHERE m.business_id=b.id AND m.status='active') AS active_membership_count
		FROM businesses b WHERE ($1='' OR b.account_status=$1)
		ORDER BY b.created_at DESC,b.id DESC LIMIT $2 OFFSET $3`, status, limit, offset)
	if err != nil {
		jsonOut(w, http.StatusServiceUnavailable, map[string]string{"error": "platform_business_list_unavailable"})
		return
	}
	defer rows.Close()
	type businessSummary struct {
		ID                    string `json:"id"`
		Name                  string `json:"name"`
		Currency              string `json:"currency"`
		Status                string `json:"status"`
		CreatedAt             any    `json:"created_at"`
		BranchCount           int64  `json:"branch_count"`
		ActiveMembershipCount int64  `json:"active_membership_count"`
	}
	items := make([]businessSummary, 0)
	for rows.Next() {
		var item businessSummary
		if err := rows.Scan(&item.ID, &item.Name, &item.Currency, &item.Status, &item.CreatedAt, &item.BranchCount, &item.ActiveMembershipCount); err != nil {
			jsonOut(w, http.StatusServiceUnavailable, map[string]string{"error": "platform_business_list_unavailable"})
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		jsonOut(w, http.StatusServiceUnavailable, map[string]string{"error": "platform_business_list_unavailable"})
		return
	}
	jsonOut(w, http.StatusOK, map[string]any{"items": items, "limit": limit, "offset": offset})
}

// setPlatformBusinessStatus changes only the platform-level business access
// flag. Tenant data and memberships are retained for reversible suspension.
func (s *server) setPlatformBusinessStatus(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "businessID"))
	if !validUUID(id) {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "invalid_business_id"})
		return
	}
	var input struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	if !decode(w, r, &input) {
		return
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if (input.Status != "active" && input.Status != "suspended") || len(input.Reason) < 8 || len(input.Reason) > 500 {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "invalid_status_or_reason"})
		return
	}
	actor, _ := r.Context().Value(userIDKey).(string)
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		jsonOut(w, 503, map[string]string{"error": "platform_operation_unavailable"})
		return
	}
	defer tx.Rollback(r.Context())
	var old string
	if err = tx.QueryRow(r.Context(), `SELECT account_status FROM businesses WHERE id=$1 FOR UPDATE`, id).Scan(&old); err != nil {
		jsonOut(w, 404, map[string]string{"error": "business_not_found"})
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE businesses SET account_status=$2,updated_at=now() WHERE id=$1`, id, input.Status); err != nil {
		jsonOut(w, 503, map[string]string{"error": "platform_operation_unavailable"})
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO platform_audit_logs(actor_user_id,action,entity_type,entity_id,reason,metadata) VALUES($1,$2,'business',$3,$4,jsonb_build_object('previous_status',$5,'new_status',$6))`, actor, "business.status_changed", id, input.Reason, old, input.Status); err != nil {
		jsonOut(w, 503, map[string]string{"error": "platform_audit_write_failed"})
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		jsonOut(w, 503, map[string]string{"error": "platform_operation_unavailable"})
		return
	}
	jsonOut(w, 200, map[string]string{"id": id, "status": input.Status})
}

// setPlatformUserStatus revokes all existing sessions on suspension. It
// refuses to suspend a user who is an active owner of any business, avoiding
// orphaning a tenant without an owner.
func (s *server) setPlatformUserStatus(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "userID"))
	if !validUUID(id) {
		jsonOut(w, 400, map[string]string{"error": "invalid_user_id"})
		return
	}
	var input struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	if !decode(w, r, &input) {
		return
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if (input.Status != "active" && input.Status != "suspended") || len(input.Reason) < 8 || len(input.Reason) > 500 {
		jsonOut(w, 400, map[string]string{"error": "invalid_status_or_reason"})
		return
	}
	actor, _ := r.Context().Value(userIDKey).(string)
	if actor == id && input.Status == "suspended" {
		jsonOut(w, 400, map[string]string{"error": "cannot_suspend_self"})
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		jsonOut(w, 503, map[string]string{"error": "platform_operation_unavailable"})
		return
	}
	defer tx.Rollback(r.Context())
	var old string
	if err = tx.QueryRow(r.Context(), `SELECT account_status FROM app_users WHERE id=$1 FOR UPDATE`, id).Scan(&old); err != nil {
		jsonOut(w, 404, map[string]string{"error": "user_not_found"})
		return
	}
	if input.Status == "suspended" {
		var owners int
		if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM memberships WHERE user_id=$1 AND role='owner' AND status='active'`, id).Scan(&owners); err != nil {
			jsonOut(w, 503, map[string]string{"error": "platform_operation_unavailable"})
			return
		}
		if owners > 0 {
			jsonOut(w, 409, map[string]string{"error": "active_business_owner_cannot_be_suspended"})
			return
		}
	}
	if _, err = tx.Exec(r.Context(), `UPDATE app_users SET account_status=$2,updated_at=now() WHERE id=$1`, id, input.Status); err != nil {
		jsonOut(w, 503, map[string]string{"error": "platform_operation_unavailable"})
		return
	}
	if input.Status == "suspended" {
		if _, err = tx.Exec(r.Context(), `UPDATE user_sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE user_id=$1`, id); err != nil {
			jsonOut(w, 503, map[string]string{"error": "session_revocation_failed"})
			return
		}
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO platform_audit_logs(actor_user_id,action,entity_type,entity_id,reason,metadata) VALUES($1,$2,'user',$3,$4,jsonb_build_object('previous_status',$5,'new_status',$6))`, actor, "user.status_changed", id, input.Reason, old, input.Status); err != nil {
		jsonOut(w, 503, map[string]string{"error": "platform_audit_write_failed"})
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		jsonOut(w, 503, map[string]string{"error": "platform_operation_unavailable"})
		return
	}
	jsonOut(w, 200, map[string]string{"id": id, "status": input.Status})
}

func (s *server) listPlatformUsers(w http.ResponseWriter, r *http.Request) {
	limit, offset, ok := parsePlatformPage(r)
	if !ok {
		jsonOut(w, 400, map[string]string{"error": "invalid_pagination"})
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" && status != "active" && status != "suspended" {
		jsonOut(w, 400, map[string]string{"error": "invalid_user_status"})
		return
	}

	rows, err := s.db.Query(r.Context(), `SELECT u.id::text,u.full_name,u.email,u.account_status,u.platform_role,u.created_at,(SELECT count(*) FROM memberships m WHERE m.user_id=u.id AND m.status='active') FROM app_users u WHERE ($1='' OR u.account_status=$1) ORDER BY u.created_at DESC,u.id LIMIT $2 OFFSET $3`, status, limit, offset)
	if err != nil {
		jsonOut(w, 503, map[string]string{"error": "platform_user_list_unavailable"})
		return
	}
	defer rows.Close()
	type item struct {
		ID                string `json:"id"`
		Name              string `json:"name"`
		Email             string `json:"email"`
		Status            string `json:"status"`
		PlatformRole      string `json:"platform_role"`
		CreatedAt         any    `json:"created_at"`
		ActiveMemberships int64  `json:"active_memberships"`
	}
	items := []item{}
	for rows.Next() {
		var x item
		if err = rows.Scan(&x.ID, &x.Name, &x.Email, &x.Status, &x.PlatformRole, &x.CreatedAt, &x.ActiveMemberships); err != nil {
			jsonOut(w, 503, map[string]string{"error": "platform_user_list_unavailable"})
			return
		}
		items = append(items, x)
	}
	if rows.Err() != nil {
		jsonOut(w, 503, map[string]string{"error": "platform_user_list_unavailable"})
		return
	}
	jsonOut(w, 200, map[string]any{"items": items, "limit": limit, "offset": offset})
}

func (s *server) listPlatformAudit(w http.ResponseWriter, r *http.Request) {
	limit, offset, ok := parsePlatformPage(r)
	if !ok {
		jsonOut(w, 400, map[string]string{"error": "invalid_pagination"})
		return
	}
	entityType := strings.TrimSpace(r.URL.Query().Get("entity_type"))
	if entityType != "" && entityType != "business" && entityType != "user" {
		jsonOut(w, 400, map[string]string{"error": "invalid_entity_type"})
		return
	}

	rows, err := s.db.Query(r.Context(), `SELECT id::text,actor_user_id::text,action,entity_type,entity_id::text,reason,metadata,created_at FROM platform_audit_logs WHERE ($1='' OR entity_type=$1) ORDER BY created_at DESC,id DESC LIMIT $2 OFFSET $3`, entityType, limit, offset)
	if err != nil {
		jsonOut(w, 503, map[string]string{"error": "platform_audit_unavailable"})
		return
	}
	defer rows.Close()
	type event struct {
		ID         string          `json:"id"`
		ActorID    *string         `json:"actor_user_id"`
		Action     string          `json:"action"`
		EntityType string          `json:"entity_type"`
		EntityID   string          `json:"entity_id"`
		Reason     string          `json:"reason"`
		Metadata   json.RawMessage `json:"metadata"`
		CreatedAt  any             `json:"created_at"`
	}
	items := []event{}
	for rows.Next() {
		var e event
		if err = rows.Scan(&e.ID, &e.ActorID, &e.Action, &e.EntityType, &e.EntityID, &e.Reason, &e.Metadata, &e.CreatedAt); err != nil {
			jsonOut(w, 503, map[string]string{"error": "platform_audit_unavailable"})
			return
		}
		items = append(items, e)
	}
	if rows.Err() != nil {
		jsonOut(w, 503, map[string]string{"error": "platform_audit_unavailable"})
		return
	}
	jsonOut(w, 200, map[string]any{"items": items, "limit": limit, "offset": offset})
}

// provisionPlatformBusiness creates a tenant workspace for an already-registered,
// active user. It never creates credentials or elevates platform privileges.
func (s *server) provisionPlatformBusiness(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name       string `json:"name"`
		BranchName string `json:"branch_name"`
		OwnerEmail string `json:"owner_email"`
		Currency   string `json:"currency"`
		Reason     string `json:"reason"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.BranchName = strings.TrimSpace(in.BranchName)
	in.OwnerEmail = strings.ToLower(strings.TrimSpace(in.OwnerEmail))
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	in.Reason = strings.TrimSpace(in.Reason)
	if in.BranchName == "" {
		in.BranchName = "Main Branch"
	}
	if !validPlatformProvisionInput(in.Name, in.BranchName, in.OwnerEmail, in.Currency, in.Reason) {
		jsonOut(w, 400, map[string]string{"error": "invalid_business_provisioning_details"})
		return
	}
	actor, _ := r.Context().Value(userIDKey).(string)
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		jsonOut(w, 503, map[string]string{"error": "platform_operation_unavailable"})
		return
	}
	defer tx.Rollback(r.Context())
	var ownerID string
	var ownerStatus string
	if err = tx.QueryRow(r.Context(), `SELECT user_id::text,account_status FROM lookup_auth_user($1)`, in.OwnerEmail).Scan(&ownerID, &ownerStatus); err != nil {
		jsonOut(w, 404, map[string]string{"error": "owner_account_not_found"})
		return
	}
	if ownerStatus != "active" {
		jsonOut(w, 409, map[string]string{"error": "owner_account_not_active"})
		return
	}
	if _, err = tx.Exec(r.Context(), `SELECT set_config('app.registration','true',true)`); err != nil {
		jsonOut(w, 503, map[string]string{"error": "platform_operation_unavailable"})
		return
	}
	if _, err = tx.Exec(r.Context(), `SELECT set_config('app.user_id',$1,true)`, ownerID); err != nil {
		jsonOut(w, 503, map[string]string{"error": "platform_operation_unavailable"})
		return
	}
	if err = tx.QueryRow(r.Context(), `SELECT account_status FROM app_users WHERE id=$1 FOR UPDATE`, ownerID).Scan(&ownerStatus); err != nil || ownerStatus != "active" {
		jsonOut(w, 409, map[string]string{"error": "owner_account_not_active"})
		return
	}
	var businessID, branchID, membershipID string
	if err = tx.QueryRow(r.Context(), `INSERT INTO businesses(name,currency) VALUES($1,$2) RETURNING id::text`, in.Name, in.Currency).Scan(&businessID); err != nil {
		jsonOut(w, 503, map[string]string{"error": "business_provisioning_failed"})
		return
	}
	if _, err = tx.Exec(r.Context(), `SELECT set_config('app.business_id',$1,true)`, businessID); err != nil {
		jsonOut(w, 503, map[string]string{"error": "platform_operation_unavailable"})
		return
	}
	if err = tx.QueryRow(r.Context(), `INSERT INTO branches(business_id,name) VALUES($1,$2) RETURNING id::text`, businessID, in.BranchName).Scan(&branchID); err != nil {
		jsonOut(w, 503, map[string]string{"error": "business_provisioning_failed"})
		return
	}
	if err = tx.QueryRow(r.Context(), `INSERT INTO memberships(business_id,user_id,role,status) VALUES($1,$2,'owner','active') RETURNING id::text`, businessID, ownerID).Scan(&membershipID); err != nil {
		jsonOut(w, 503, map[string]string{"error": "business_owner_assignment_failed"})
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO membership_branches(business_id,membership_id,branch_id) VALUES($1,$2,$3)`, businessID, membershipID, branchID); err != nil {
		jsonOut(w, 503, map[string]string{"error": "business_owner_branch_assignment_failed"})
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO expense_categories(business_id,name) SELECT $1,v.name FROM (VALUES ('Rent'),('Utilities'),('Transport'),('Repairs & maintenance'),('Cleaning'),('Staff welfare'),('Office supplies'),('Marketing'),('Licences & fees'),('Other')) v(name)`, businessID); err != nil {
		jsonOut(w, 503, map[string]string{"error": "business_defaults_creation_failed"})
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO platform_audit_logs(actor_user_id,action,entity_type,entity_id,reason,metadata) VALUES($1,'business.provisioned','business',$2,$3,jsonb_build_object('owner_user_id',$4,'branch_id',$5,'currency',$6))`, actor, businessID, in.Reason, ownerID, branchID, in.Currency); err != nil {
		jsonOut(w, 503, map[string]string{"error": "platform_audit_write_failed"})
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		jsonOut(w, 503, map[string]string{"error": "platform_operation_unavailable"})
		return
	}
	jsonOut(w, 201, map[string]any{"business_id": businessID, "branch_id": branchID, "owner_user_id": ownerID, "status": "active"})
}

func validPlatformProvisionInput(name, branch, ownerEmail, currency, reason string) bool {
	if len(name) < 2 || len(name) > 160 || len(branch) < 2 || len(branch) > 120 || !validEmail(ownerEmail) || len(currency) != 3 || len(reason) < 8 || len(reason) > 500 {
		return false
	}
	for _, ch := range currency {
		if ch < 'A' || ch > 'Z' {
			return false
		}
	}
	return true
}
