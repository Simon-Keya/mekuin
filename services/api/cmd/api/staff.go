package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

type staffMembershipInput struct {
	Email     string   `json:"email"`
	Role      string   `json:"role"`
	BranchIDs []string `json:"branch_ids"`
}

// createStaffMembership attaches an already-registered account to the current
// business. The account must exist first; the user then sees this business in
// their workspace list. Branches are assigned atomically with the membership.
func (s *server) createStaffMembership(w http.ResponseWriter, r *http.Request) {
	tx, _, ok := catalogTx(r)
	if !ok {
		jsonOut(w, http.StatusInternalServerError, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in staffMembershipInput
	if !decode(w, r, &in) {
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.Role = strings.TrimSpace(in.Role)
	roles := map[string]bool{"manager": true, "cashier": true, "bartender": true, "stock_controller": true}
	if !requiredText(in.Email, 254) || !strings.Contains(in.Email, "@") || !roles[in.Role] || len(in.BranchIDs) == 0 || len(in.BranchIDs) > 100 {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "invalid_staff_membership"})
		return
	}
	seen := map[string]bool{}
	for _, id := range in.BranchIDs {
		if !validUUID(id) || seen[id] {
			jsonOut(w, http.StatusBadRequest, map[string]string{"error": "invalid_staff_branch_assignments"})
			return
		}
		seen[id] = true
	}
	for _, branchID := range in.BranchIDs {
		var branchExists bool
		if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM branches WHERE business_id=$1 AND id=$2 AND is_active=true)`, tenantBusiness(r), branchID).Scan(&branchExists); err != nil {
			catalogError(w, err)
			return
		}
		if !branchExists {
			jsonOut(w, http.StatusUnprocessableEntity, map[string]string{"error": "branch_not_available_for_business"})
			return
		}
	}
	var userID string
	if err := tx.QueryRow(r.Context(), `SELECT user_id FROM lookup_staff_user($1)`, in.Email).Scan(&userID); err != nil {
		if err == pgx.ErrNoRows {
			jsonOut(w, http.StatusNotFound, map[string]string{"error": "registered_user_not_found"})
			return
		}
		catalogError(w, err)
		return
	}
	var membershipID string
	if err := tx.QueryRow(r.Context(), `INSERT INTO memberships(business_id,user_id,role,status) VALUES($1,$2,$3,'active') RETURNING id`, tenantBusiness(r), userID, in.Role).Scan(&membershipID); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			jsonOut(w, http.StatusConflict, map[string]string{"error": "user_already_has_business_membership"})
			return
		}
		catalogError(w, err)
		return
	}
	for _, branchID := range in.BranchIDs {
		if _, err := tx.Exec(r.Context(), `INSERT INTO membership_branches(business_id,membership_id,branch_id) VALUES($1,$2,$3)`, tenantBusiness(r), membershipID, branchID); err != nil {
			catalogError(w, err)
			return
		}
	}
	if _, err := tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary) VALUES($1,$2,'staff.membership.created','membership',$3,'Existing user added to business')`, tenantBusiness(r), r.Context().Value(userIDKey), membershipID); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, http.StatusCreated, map[string]string{"membership_id": membershipID, "user_id": userID, "role": in.Role, "status": "active"})
}

type staffProfileInput struct {
	MembershipID string `json:"membership_id"`
	DisplayName  string `json:"display_name"`
	Phone        string `json:"phone"`
}

func (s *server) listStaff(w http.ResponseWriter, r *http.Request) {
	tx, _, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	limit, offset, search, valid := listBounds(r)
	if !valid {
		jsonOut(w, 400, map[string]string{"error": "invalid_pagination_or_search"})
		return
	}
	rows, err := tx.Query(r.Context(), `SELECT m.id,m.user_id,u.email,u.full_name,m.role,m.status,sp.id,sp.display_name,sp.phone,sp.is_active,
		COALESCE((SELECT jsonb_agg(jsonb_build_object('id',b.id,'name',b.name) ORDER BY b.name) FROM membership_branches mb JOIN branches b ON b.id=mb.branch_id AND b.business_id=mb.business_id WHERE mb.business_id=m.business_id AND mb.membership_id=m.id),'[]'::jsonb)
		FROM memberships m JOIN app_users u ON u.id=m.user_id LEFT JOIN staff_profiles sp ON sp.business_id=m.business_id AND sp.membership_id=m.id
		WHERE m.business_id=$1 AND ($2='' OR u.full_name ILIKE '%'||$2||'%' OR u.email ILIKE '%'||$2||'%') ORDER BY u.full_name,m.id LIMIT $3 OFFSET $4`, tenantBusiness(r), search, limit, offset)
	if err != nil {
		catalogError(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var mid, uid, email, fullName, role, status string
		var pid, display, phone *string
		var active *bool
		var branches json.RawMessage
		if err = rows.Scan(&mid, &uid, &email, &fullName, &role, &status, &pid, &display, &phone, &active, &branches); err != nil {
			catalogError(w, err)
			return
		}
		out = append(out, map[string]any{"membership_id": mid, "user_id": uid, "email": email, "full_name": fullName, "role": role, "membership_status": status, "profile_id": pid, "display_name": display, "phone": phone, "is_active": active, "branches": branches})
	}
	if rows.Err() != nil {
		catalogError(w, rows.Err())
		return
	}
	jsonOut(w, 200, out)
}

func (s *server) createStaffProfile(w http.ResponseWriter, r *http.Request) {
	tx, _, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in staffProfileInput
	if !decode(w, r, &in) {
		return
	}
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	in.Phone = strings.TrimSpace(in.Phone)
	if !requiredText(in.MembershipID, 64) || !requiredText(in.DisplayName, 160) {
		jsonOut(w, 400, map[string]string{"error": "membership_and_display_name_required"})
		return
	}
	var id string
	err := tx.QueryRow(r.Context(), `INSERT INTO staff_profiles(business_id,membership_id,display_name,phone) VALUES($1,$2,$3,NULLIF($4,'')) RETURNING id`, tenantBusiness(r), in.MembershipID, in.DisplayName, in.Phone).Scan(&id)
	if err != nil {
		catalogError(w, err)
		return
	}
	if err = auditStaff(r, tx, "staff.profile.created", id, "Staff profile created"); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 201, map[string]string{"id": id})
}

func (s *server) updateStaffBranches(w http.ResponseWriter, r *http.Request) {
	tx, _, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in struct {
		BranchIDs []string `json:"branch_ids"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.BranchIDs) == 0 || len(in.BranchIDs) > 100 {
		jsonOut(w, 400, map[string]string{"error": "branch_ids_required"})
		return
	}
	seen := map[string]bool{}
	for _, bid := range in.BranchIDs {
		if !validUUID(bid) || seen[bid] {
			jsonOut(w, 400, map[string]string{"error": "invalid_staff_branch_assignments"})
			return
		}
		seen[bid] = true
		var active bool
		if err := tx.QueryRow(r.Context(), `SELECT is_active FROM branches WHERE business_id=$1 AND id=$2`, tenantBusiness(r), bid).Scan(&active); err != nil {
			catalogError(w, err)
			return
		}
		if !active {
			jsonOut(w, 422, map[string]string{"error": "inactive_branch_cannot_be_assigned"})
			return
		}
	}
	mid := chi.URLParam(r, "membershipID")
	var exists bool
	if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM memberships WHERE business_id=$1 AND id=$2)`, tenantBusiness(r), mid).Scan(&exists); err != nil {
		catalogError(w, err)
		return
	}
	if !exists {
		jsonOut(w, 404, map[string]string{"error": "membership_not_found"})
		return
	}
	if _, err := tx.Exec(r.Context(), `DELETE FROM membership_branches WHERE business_id=$1 AND membership_id=$2`, tenantBusiness(r), mid); err != nil {
		catalogError(w, err)
		return
	}
	for _, bid := range in.BranchIDs {
		if _, err := tx.Exec(r.Context(), `INSERT INTO membership_branches(business_id,membership_id,branch_id) VALUES($1,$2,$3)`, tenantBusiness(r), mid, bid); err != nil {
			catalogError(w, err)
			return
		}
	}
	if err := auditStaff(r, tx, "staff.branches.updated", mid, "Staff branch assignments updated"); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"membership_id": mid, "branch_ids": in.BranchIDs})
}

func (s *server) updateStaffRole(w http.ResponseWriter, r *http.Request) {
	tx, _, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in struct {
		Role string `json:"role"`
	}
	if !decode(w, r, &in) {
		return
	}
	valid := map[string]bool{"manager": true, "cashier": true, "bartender": true, "stock_controller": true, "owner": true}
	if !valid[in.Role] {
		jsonOut(w, 400, map[string]string{"error": "invalid_staff_role"})
		return
	}
	// Serialize role changes per business so two concurrent requests cannot both
	// observe two owners and demote one each, leaving the tenant ownerless.
	var lockedBusinessID string
	if err := tx.QueryRow(r.Context(), `SELECT id FROM businesses WHERE id=$1 FOR UPDATE`, tenantBusiness(r)).Scan(&lockedBusinessID); err != nil {
		catalogError(w, err)
		return
	}
	mid := chi.URLParam(r, "membershipID")
	var oldRole string
	err := tx.QueryRow(r.Context(), `SELECT role FROM memberships WHERE business_id=$1 AND id=$2 FOR UPDATE`, tenantBusiness(r), mid).Scan(&oldRole)
	if err == pgx.ErrNoRows {
		jsonOut(w, 404, map[string]string{"error": "membership_not_found"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	if oldRole == "owner" && in.Role != "owner" {
		var count int
		if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM memberships WHERE business_id=$1 AND role='owner' AND status='active'`, tenantBusiness(r)).Scan(&count); err != nil {
			catalogError(w, err)
			return
		}
		if count <= 1 {
			jsonOut(w, 409, map[string]string{"error": "cannot_remove_last_active_owner"})
			return
		}
	}
	if _, err = tx.Exec(r.Context(), `UPDATE memberships SET role=$3 WHERE business_id=$1 AND id=$2`, tenantBusiness(r), mid, in.Role); err != nil {
		catalogError(w, err)
		return
	}
	if err = auditStaff(r, tx, "staff.role.updated", mid, "Staff role changed"); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]string{"membership_id": mid, "role": in.Role})
}

func (s *server) setManagerPIN(w http.ResponseWriter, r *http.Request) {
	tx, _, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in struct {
		PIN string `json:"pin"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.PIN) < 4 || len(in.PIN) > 8 {
		jsonOut(w, 400, map[string]string{"error": "pin_must_be_4_to_8_digits"})
		return
	}
	for _, c := range in.PIN {
		if c < '0' || c > '9' {
			jsonOut(w, 400, map[string]string{"error": "pin_must_be_4_to_8_digits"})
			return
		}
	}
	mid := chi.URLParam(r, "membershipID")
	var role string
	err := tx.QueryRow(r.Context(), `SELECT role FROM memberships WHERE business_id=$1 AND id=$2`, tenantBusiness(r), mid).Scan(&role)
	if err == pgx.ErrNoRows {
		jsonOut(w, 404, map[string]string{"error": "membership_not_found"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	if role != "manager" && role != "owner" {
		jsonOut(w, 422, map[string]string{"error": "pin_only_allowed_for_manager_or_owner"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.PIN), bcrypt.DefaultCost)
	if err != nil {
		catalogError(w, err)
		return
	}
	tag, err := tx.Exec(r.Context(), `UPDATE staff_profiles SET manager_pin_hash=$3,updated_at=now() WHERE business_id=$1 AND membership_id=$2`, tenantBusiness(r), mid, string(hash))
	if err != nil {
		catalogError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		jsonOut(w, 409, map[string]string{"error": "staff_profile_required_before_pin"})
		return
	}
	if err = auditStaff(r, tx, "staff.manager_pin.set", mid, "Manager authorisation PIN updated"); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]string{"status": "updated"})
}

func auditStaff(r *http.Request, tx pgx.Tx, action, id, summary string) error {
	_, err := tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary) VALUES($1,$2,$3,'staff_profile',$4,$5)`, tenantBusiness(r), r.Context().Value(userIDKey), action, id, summary)
	return err
}

type managerAuthorizationInput struct {
	ManagerMembershipID string `json:"manager_membership_id"`
	PIN                 string `json:"pin"`
	Action              string `json:"action"`
	TargetID            string `json:"target_id"`
	TargetAmountMinor   *int64 `json:"target_amount_minor"`
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	compact := strings.ReplaceAll(value, "-", "")
	if len(compact) != 32 {
		return false
	}
	_, err := hex.DecodeString(compact)
	return err == nil
}

func validManagerAuthorizationInput(in managerAuthorizationInput) bool {
	if !validUUID(in.ManagerMembershipID) || !managerActions[in.Action] || len(in.PIN) < 4 || len(in.PIN) > 8 {
		return false
	}
	for _, c := range in.PIN {
		if c < '0' || c > '9' {
			return false
		}
	}
	if in.TargetID != "" && !validUUID(in.TargetID) {
		return false
	}
	if in.Action == "sales.refund" {
		return in.TargetID != "" && in.TargetAmountMinor != nil && *in.TargetAmountMinor > 0
	}
	return in.TargetAmountMinor == nil
}

var managerActions = map[string]bool{"discount.approve": true, "sales.void": true, "sales.refund": true, "inventory.count.approve": true}

// authorizeManagerAction issues a two-minute, single-use token scoped to the
// selected business, branch, requesting user, action and optional target.
func (s *server) authorizeManagerAction(w http.ResponseWriter, r *http.Request) {
	tx, _, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in managerAuthorizationInput
	if !decode(w, r, &in) {
		return
	}
	if !validManagerAuthorizationInput(in) {
		jsonOut(w, 400, map[string]string{"error": "invalid_authorization_request"})
		return
	}
	tc, ok := r.Context().Value(tenantContextKey).(tenantContext)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var approverID, hash string
	var lockedUntil *time.Time
	err := tx.QueryRow(r.Context(), `SELECT m.id,sp.manager_pin_hash,sp.manager_pin_locked_until FROM memberships m JOIN staff_profiles sp ON sp.business_id=m.business_id AND sp.membership_id=m.id WHERE m.business_id=$1 AND m.id=$2 AND m.status='active' AND m.role IN ('owner','manager') AND sp.is_active=true AND sp.manager_pin_hash IS NOT NULL AND m.user_id<>$3 AND EXISTS (SELECT 1 FROM membership_branches mb WHERE mb.business_id=m.business_id AND mb.membership_id=m.id AND mb.branch_id=$4) FOR UPDATE OF sp`, tc.BusinessID, in.ManagerMembershipID, tc.UserID, tc.BranchID).Scan(&approverID, &hash, &lockedUntil)
	if err != nil || lockedUntil != nil && lockedUntil.After(time.Now()) {
		jsonOut(w, 403, map[string]string{"error": "manager_authorization_unavailable"})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.PIN)) != nil {
		var attempts int
		if err = tx.QueryRow(r.Context(), `UPDATE staff_profiles SET manager_pin_failed_attempts=CASE WHEN manager_pin_window_started_at IS NULL OR manager_pin_window_started_at < now()-interval '15 minutes' THEN 1 ELSE manager_pin_failed_attempts+1 END, manager_pin_window_started_at=CASE WHEN manager_pin_window_started_at IS NULL OR manager_pin_window_started_at < now()-interval '15 minutes' THEN now() ELSE manager_pin_window_started_at END, manager_pin_locked_until=CASE WHEN (CASE WHEN manager_pin_window_started_at IS NULL OR manager_pin_window_started_at < now()-interval '15 minutes' THEN 1 ELSE manager_pin_failed_attempts+1 END)>=5 THEN now()+interval '15 minutes' ELSE manager_pin_locked_until END WHERE business_id=$1 AND membership_id=$2 RETURNING manager_pin_failed_attempts`, tc.BusinessID, approverID).Scan(&attempts); err != nil {
			catalogError(w, err)
			return
		}
		if auditErr := auditStaff(r, tx, "manager_pin.failed", approverID, "Manager PIN verification failed"); auditErr != nil {
			catalogError(w, auditErr)
			return
		}
		jsonOut(w, 200, map[string]any{"authorized": false, "error": "manager_authorization_failed"})
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE staff_profiles SET manager_pin_failed_attempts=0,manager_pin_window_started_at=NULL,manager_pin_locked_until=NULL WHERE business_id=$1 AND membership_id=$2`, tc.BusinessID, approverID); err != nil {
		catalogError(w, err)
		return
	}
	token, digest, tokenErr := newManagerApprovalToken()
	if tokenErr != nil {
		catalogError(w, tokenErr)
		return
	}
	var target any
	if in.TargetID != "" {
		target = in.TargetID
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO manager_action_approvals(business_id,branch_id,requester_user_id,approver_membership_id,action,target_id,target_amount_minor,token_hash,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,now()+interval '2 minutes')`, tc.BusinessID, tc.BranchID, tc.UserID, approverID, in.Action, target, in.TargetAmountMinor, digest)
	if err != nil {
		catalogError(w, err)
		return
	}
	if err = auditStaff(r, tx, "manager_action.authorized", approverID, "Manager action authorization issued: "+in.Action); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"authorized": true, "approval_token": token, "action": in.Action, "expires_in_seconds": 120, "single_use": true})
}

// consumeManagerApproval must be called inside the same transaction as the
// sensitive operation. UPDATE ... RETURNING makes consumption atomic.
func newManagerApprovalToken() (string, []byte, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	token := hex.EncodeToString(raw)
	digest := sha256.Sum256([]byte(token))
	return token, digest[:], nil
}

func managerApprovalTokenDigest(token string) ([]byte, bool) {
	if len(token) != 64 {
		return nil, false
	}
	if _, err := hex.DecodeString(token); err != nil {
		return nil, false
	}
	digest := sha256.Sum256([]byte(strings.ToLower(token)))
	return digest[:], true
}

func consumeManagerApproval(ctx context.Context, tx pgx.Tx, tc tenantContext, token, action, targetID string, targetAmount *int64) error {
	digest, valid := managerApprovalTokenDigest(token)
	if !valid {
		return pgx.ErrNoRows
	}
	var id string
	err := tx.QueryRow(ctx, `UPDATE manager_action_approvals SET consumed_at=now() WHERE token_hash=$1 AND business_id=$2 AND branch_id=$3 AND requester_user_id=$4 AND action=$5 AND target_id IS NOT DISTINCT FROM NULLIF($6,'')::uuid AND target_amount_minor IS NOT DISTINCT FROM $7 AND consumed_at IS NULL AND expires_at>now() RETURNING id`, digest, tc.BusinessID, tc.BranchID, tc.UserID, action, targetID, targetAmount).Scan(&id)
	return err
}
