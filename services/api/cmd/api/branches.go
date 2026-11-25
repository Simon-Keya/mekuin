package main

import (
	"net/http"

	"github.com/jackc/pgx/v5"
	"strings"

	"github.com/go-chi/chi/v5"
)

type branchInput struct {
	Name   string `json:"name"`
	Phone  string `json:"phone"`
	Active *bool  `json:"is_active"`
}

func (s *server) listBranches(w http.ResponseWriter, r *http.Request) {
	tx, _, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	rows, err := tx.Query(r.Context(), `SELECT id,name,phone,is_active FROM branches WHERE business_id=$1 ORDER BY name,id`, tenantBusiness(r))
	if err != nil {
		catalogError(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name string
		var phone *string
		var active bool
		if err = rows.Scan(&id, &name, &phone, &active); err != nil {
			catalogError(w, err)
			return
		}
		out = append(out, map[string]any{"id": id, "name": name, "phone": phone, "is_active": active})
	}
	if rows.Err() != nil {
		catalogError(w, rows.Err())
		return
	}
	jsonOut(w, 200, out)
}
func (s *server) createBranch(w http.ResponseWriter, r *http.Request) {
	tx, _, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in branchInput
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Phone = strings.TrimSpace(in.Phone)
	if !requiredText(in.Name, 120) || len(in.Phone) > 40 {
		jsonOut(w, 400, map[string]string{"error": "invalid_branch"})
		return
	}
	var id string
	err := tx.QueryRow(r.Context(), `INSERT INTO branches(business_id,name,phone) VALUES($1,$2,NULLIF($3,'')) RETURNING id`, tenantBusiness(r), in.Name, in.Phone).Scan(&id)
	if err != nil {
		catalogError(w, err)
		return
	}
	if err = auditBranch(r, tx, "branch.created", id, "Branch created"); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 201, map[string]any{"id": id, "name": in.Name, "phone": in.Phone, "is_active": true})
}
func (s *server) updateBranch(w http.ResponseWriter, r *http.Request) {
	tx, _, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in branchInput
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Phone = strings.TrimSpace(in.Phone)
	if !requiredText(in.Name, 120) || len(in.Phone) > 40 || in.Active == nil {
		jsonOut(w, 400, map[string]string{"error": "name_phone_and_active_required"})
		return
	}
	id := chi.URLParam(r, "branchID")
	tag, err := tx.Exec(r.Context(), `UPDATE branches SET name=$3,phone=NULLIF($4,''),is_active=$5 WHERE business_id=$1 AND id=$2`, tenantBusiness(r), id, in.Name, in.Phone, *in.Active)
	if err != nil {
		catalogError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		jsonOut(w, 404, map[string]string{"error": "branch_not_found"})
		return
	}
	if err = auditBranch(r, tx, "branch.updated", id, "Branch details or active status updated"); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"id": id, "name": in.Name, "phone": in.Phone, "is_active": *in.Active})
}

func auditBranch(r *http.Request, tx pgx.Tx, action, id, summary string) error {
	_, err := tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary) VALUES($1,$2,$3,'branch',$4,$5)`, tenantBusiness(r), r.Context().Value(userIDKey), action, id, summary)
	return err
}
