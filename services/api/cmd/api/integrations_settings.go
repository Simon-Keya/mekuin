package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

type integrationSettingInput struct {
	Status       string            `json:"status"`
	PublicConfig map[string]string `json:"public_config"`
}

func (s *server) listIntegrationSettings(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	if tc.Role != "owner" {
		jsonOut(w, 403, map[string]string{"error": "owner_permission_required"})
		return
	}
	rows, err := tx.Query(r.Context(), `SELECT provider,status,public_config,updated_at FROM business_integration_settings WHERE business_id=$1 ORDER BY provider`, tc.BusinessID)
	if err != nil {
		catalogError(w, err)
		return
	}
	defer rows.Close()
	type item struct {
		Provider     string            `json:"provider"`
		Status       string            `json:"status"`
		PublicConfig map[string]string `json:"public_config"`
		UpdatedAt    any               `json:"updated_at"`
	}
	found := map[string]item{}
	for rows.Next() {
		var v item
		var raw []byte
		if err = rows.Scan(&v.Provider, &v.Status, &raw, &v.UpdatedAt); err != nil {
			catalogError(w, err)
			return
		}
		_ = json.Unmarshal(raw, &v.PublicConfig)
		found[v.Provider] = v
	}
	if err = rows.Err(); err != nil {
		catalogError(w, err)
		return
	}
	darajaConfigured := false
	if _, e := newDarajaClientFromEnv(); e == nil {
		darajaConfigured = true
	}
	providers := []item{}
	for _, p := range []string{"mpesa", "etims", "card"} {
		v, exists := found[p]
		if !exists {
			v = item{Provider: p, Status: "not_configured", PublicConfig: map[string]string{}}
		}
		providers = append(providers, v)
	}
	jsonOut(w, 200, map[string]any{"items": providers, "capabilities": map[string]any{"mpesa": map[string]any{"provider": "Safaricom Daraja", "provider_credentials_available_in_deployment": darajaConfigured, "payment_api": "STK initiation and callback settlement are available only when deployment credentials and callback URL are configured"}, "etims": map[string]any{"configuration_only": true, "invoice_transmission_enabled": false}, "card": map[string]any{"manual_reference_only": true, "gateway_enabled": false}}, "secrets_policy": "Credential values are never stored or returned by this endpoint; use deployment secret configuration."})
}
func (s *server) updateIntegrationSetting(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	if tc.Role != "owner" {
		jsonOut(w, 403, map[string]string{"error": "owner_permission_required"})
		return
	}
	provider := strings.TrimSpace(r.PathValue("provider"))
	if provider == "" {
		provider = strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/v1/settings/integrations/"))
	}
	if provider != "mpesa" && provider != "etims" && provider != "card" {
		jsonOut(w, 404, map[string]string{"error": "integration_provider_not_found"})
		return
	}
	var in integrationSettingInput
	if !decode(w, r, &in) {
		return
	}
	in.Status = strings.TrimSpace(in.Status)
	if in.Status != "not_configured" && in.Status != "pending" && in.Status != "live" {
		jsonOut(w, 400, map[string]string{"error": "invalid_integration_status"})
		return
	}
	if in.PublicConfig == nil {
		in.PublicConfig = map[string]string{}
	}
	allowed := map[string]map[string]bool{"mpesa": {"shortcode": true, "account_reference": true, "environment": true}, "etims": {"branch_name": true, "environment": true}, "card": {"gateway_label": true, "terminal_reference": true}}
	clean := map[string]string{}
	for k, v := range in.PublicConfig {
		if !allowed[provider][k] || len(v) > 120 {
			jsonOut(w, 400, map[string]string{"error": "invalid_or_secret_integration_config_field"})
			return
		}
		clean[k] = strings.TrimSpace(v)
	}
	if in.Status == "live" {
		if provider != "mpesa" {
			jsonOut(w, 409, map[string]string{"error": "provider_is_configuration_only"})
			return
		}
		if _, err := newDarajaClientFromEnv(); err != nil {
			jsonOut(w, 409, map[string]string{"error": "daraja_deployment_credentials_not_configured"})
			return
		}
	}
	raw, err := json.Marshal(clean)
	if err != nil {
		jsonOut(w, 400, map[string]string{"error": "invalid_integration_config"})
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO business_integration_settings(business_id,provider,status,public_config,updated_by,updated_at) VALUES($1,$2,$3,$4::jsonb,$5,now()) ON CONFLICT(business_id,provider) DO UPDATE SET status=EXCLUDED.status,public_config=EXCLUDED.public_config,updated_by=EXCLUDED.updated_by,updated_at=now()`, tc.BusinessID, provider, in.Status, string(raw), tc.UserID)
	if err != nil {
		catalogError(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,summary,metadata) VALUES($1,$2,'settings.integration.updated','business_integration_settings','Business integration settings updated',jsonb_build_object('provider',$3,'status',$4))`, tc.BusinessID, tc.UserID, provider, in.Status); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"provider": provider, "status": in.Status, "public_config": clean})
}
