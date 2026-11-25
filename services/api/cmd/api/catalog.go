package main

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type responseBuffer struct {
	header http.Header
	status int
	body   []byte
}

func newResponseBuffer() *responseBuffer {
	return &responseBuffer{header: make(http.Header), status: http.StatusOK}
}
func (b *responseBuffer) Header() http.Header { return b.header }
func (b *responseBuffer) WriteHeader(status int) {
	if b.status == http.StatusOK {
		b.status = status
	}
}
func (b *responseBuffer) Write(p []byte) (int, error) {
	b.body = append(b.body, p...)
	return len(p), nil
}
func (b *responseBuffer) flushTo(w http.ResponseWriter) {
	for k, vs := range b.header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(b.status)
	_, _ = w.Write(b.body)
}

func catalogTx(r *http.Request) (pgx.Tx, tenantContext, bool) {
	tx, ok := tenantTxFromContext(r.Context())
	tc, tok := r.Context().Value(tenantContextKey).(tenantContext)
	return tx, tc, ok && tok
}
func catalogError(w http.ResponseWriter, err error) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			jsonOut(w, 409, map[string]string{"error": "duplicate_catalog_value"})
			return
		case "23503":
			jsonOut(w, 422, map[string]string{"error": "referenced_record_not_found"})
			return
		case "23514", "22003", "22P02":
			jsonOut(w, 422, map[string]string{"error": "catalog_validation_failed"})
			return
		}
	}
	jsonOut(w, 500, map[string]string{"error": "catalog_operation_failed"})
}

// listBounds applies a conservative default and maximum to collection endpoints.
// Search text is trimmed and bounded before it is passed as a SQL parameter.
func listBounds(r *http.Request) (limit, offset int, search string, ok bool) {
	limit = 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			return 0, 0, "", false
		}
		limit = n
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || n > 1000000 {
			return 0, 0, "", false
		}
		offset = n
	}
	search = strings.TrimSpace(r.URL.Query().Get("q"))
	if len(search) > 100 {
		return 0, 0, "", false
	}
	return limit, offset, search, true
}

func requiredText(v string, max int) bool {
	v = strings.TrimSpace(v)
	return len(v) > 0 && len(v) <= max
}

type categoryInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (s *server) listCategories(w http.ResponseWriter, r *http.Request) {
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
	rows, e := tx.Query(r.Context(), `SELECT id,name,description,is_active,created_at,updated_at FROM product_categories WHERE ($1='' OR name ILIKE '%'||$1||'%') ORDER BY name LIMIT $2 OFFSET $3`, search, limit, offset)
	if e != nil {
		catalogError(w, e)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name string
		var desc *string
		var active bool
		var created, updated any
		if e = rows.Scan(&id, &name, &desc, &active, &created, &updated); e != nil {
			catalogError(w, e)
			return
		}
		out = append(out, map[string]any{"id": id, "name": name, "description": desc, "is_active": active, "created_at": created, "updated_at": updated})
	}
	if rows.Err() != nil {
		catalogError(w, rows.Err())
		return
	}
	jsonOut(w, 200, out)
}
func (s *server) createCategory(w http.ResponseWriter, r *http.Request) {
	tx, _, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in categoryInput
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if !requiredText(in.Name, 100) {
		jsonOut(w, 400, map[string]string{"error": "category_name_required"})
		return
	}
	var id string
	e := tx.QueryRow(r.Context(), `INSERT INTO product_categories(business_id,name,description) VALUES($1,$2,NULLIF($3,'')) RETURNING id`, tenantBusiness(r), in.Name, strings.TrimSpace(in.Description)).Scan(&id)
	if e != nil {
		catalogError(w, e)
		return
	}
	jsonOut(w, 201, map[string]string{"id": id, "name": in.Name})
}
func tenantBusiness(r *http.Request) string {
	tc, _ := r.Context().Value(tenantContextKey).(tenantContext)
	return tc.BusinessID
}

type unitInput struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	Dimension string `json:"dimension"`
}

func (s *server) listUnits(w http.ResponseWriter, r *http.Request) {
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
	rows, e := tx.Query(r.Context(), `SELECT id,code,name,dimension,is_active FROM units_of_measure WHERE ($1='' OR code ILIKE '%'||$1||'%' OR name ILIKE '%'||$1||'%') ORDER BY code LIMIT $2 OFFSET $3`, search, limit, offset)
	if e != nil {
		catalogError(w, e)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, code, name, dim string
		var active bool
		if e = rows.Scan(&id, &code, &name, &dim, &active); e != nil {
			catalogError(w, e)
			return
		}
		out = append(out, map[string]any{"id": id, "code": code, "name": name, "dimension": dim, "is_active": active})
	}
	if rows.Err() != nil {
		catalogError(w, rows.Err())
		return
	}
	jsonOut(w, 200, out)
}
func (s *server) createUnit(w http.ResponseWriter, r *http.Request) {
	tx, _, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in unitInput
	if !decode(w, r, &in) {
		return
	}
	in.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	in.Name = strings.TrimSpace(in.Name)
	in.Dimension = strings.TrimSpace(in.Dimension)
	if !requiredText(in.Code, 24) || !requiredText(in.Name, 80) || (in.Dimension != "count" && in.Dimension != "mass" && in.Dimension != "volume") {
		jsonOut(w, 400, map[string]string{"error": "invalid_unit_definition"})
		return
	}
	var id string
	e := tx.QueryRow(r.Context(), `INSERT INTO units_of_measure(business_id,code,name,dimension) VALUES($1,$2,$3,$4) RETURNING id`, tenantBusiness(r), in.Code, in.Name, in.Dimension).Scan(&id)
	if e != nil {
		catalogError(w, e)
		return
	}
	jsonOut(w, 201, map[string]string{"id": id, "code": in.Code, "name": in.Name, "dimension": in.Dimension})
}

type unitConversionInput struct {
	FromUnitID string `json:"from_unit_id"`
	ToUnitID   string `json:"to_unit_id"`
	Factor     string `json:"factor"`
}

func (s *server) listUnitConversions(w http.ResponseWriter, r *http.Request) {
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
	rows, err := tx.Query(r.Context(), `
		SELECT c.id, c.from_unit_id, fu.code, c.to_unit_id, tu.code, c.factor::text
		FROM unit_conversions c
		JOIN units_of_measure fu ON fu.business_id=c.business_id AND fu.id=c.from_unit_id
		JOIN units_of_measure tu ON tu.business_id=c.business_id AND tu.id=c.to_unit_id
		WHERE ($1='' OR fu.code ILIKE '%'||$1||'%' OR tu.code ILIKE '%'||$1||'%')
		ORDER BY fu.code, tu.code LIMIT $2 OFFSET $3`, search, limit, offset)
	if err != nil {
		catalogError(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, fromID, fromCode, toID, toCode, factor string
		if err = rows.Scan(&id, &fromID, &fromCode, &toID, &toCode, &factor); err != nil {
			catalogError(w, err)
			return
		}
		out = append(out, map[string]any{"id": id, "from_unit_id": fromID, "from_unit_code": fromCode, "to_unit_id": toID, "to_unit_code": toCode, "factor": factor})
	}
	if rows.Err() != nil {
		catalogError(w, rows.Err())
		return
	}
	jsonOut(w, 200, out)
}

func (s *server) createUnitConversion(w http.ResponseWriter, r *http.Request) {
	tx, _, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in unitConversionInput
	if !decode(w, r, &in) {
		return
	}
	in.FromUnitID = strings.TrimSpace(in.FromUnitID)
	in.ToUnitID = strings.TrimSpace(in.ToUnitID)
	in.Factor = strings.TrimSpace(in.Factor)
	if !requiredText(in.FromUnitID, 64) || !requiredText(in.ToUnitID, 64) || !requiredText(in.Factor, 32) || in.FromUnitID == in.ToUnitID {
		jsonOut(w, 400, map[string]string{"error": "invalid_unit_conversion"})
		return
	}
	var id string
	err := tx.QueryRow(r.Context(), `INSERT INTO unit_conversions(business_id,from_unit_id,to_unit_id,factor) VALUES($1,$2,$3,$4::numeric) RETURNING id`, tenantBusiness(r), in.FromUnitID, in.ToUnitID, in.Factor).Scan(&id)
	if err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 201, map[string]string{"id": id, "from_unit_id": in.FromUnitID, "to_unit_id": in.ToUnitID, "factor": in.Factor})
}

type productInput struct {
	CategoryID     *string `json:"category_id"`
	BaseUnitID     string  `json:"base_unit_id"`
	SKU            string  `json:"sku"`
	Name           string  `json:"name"`
	Description    string  `json:"description"`
	Kind           string  `json:"product_kind"`
	CostPriceMinor int64   `json:"cost_price_minor"`
	SalePriceMinor int64   `json:"sale_price_minor"`
	ReorderLevel   string  `json:"reorder_level"`
	BottleSize     *string `json:"bottle_size"`
	ServingSize    *string `json:"serving_size"`
	Sellable       bool    `json:"is_sellable"`
}

func (s *server) listProducts(w http.ResponseWriter, r *http.Request) {
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
	rows, e := tx.Query(r.Context(), `SELECT id,category_id,base_unit_id,sku,name,description,product_kind,cost_price_minor,sale_price_minor,reorder_level::text,bottle_size::text,serving_size::text,is_sellable,is_active FROM products WHERE ($1='' OR name ILIKE '%'||$1||'%' OR sku ILIKE '%'||$1||'%') ORDER BY name,id LIMIT $2 OFFSET $3`, search, limit, offset)
	if e != nil {
		catalogError(w, e)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, unit, sku, name, kind, reorder string
		var category, desc, bottle, serving *string
		var cost, sale int64
		var sell, active bool
		if e = rows.Scan(&id, &category, &unit, &sku, &name, &desc, &kind, &cost, &sale, &reorder, &bottle, &serving, &sell, &active); e != nil {
			catalogError(w, e)
			return
		}
		out = append(out, map[string]any{"id": id, "category_id": category, "base_unit_id": unit, "sku": sku, "name": name, "description": desc, "product_kind": kind, "cost_price_minor": cost, "sale_price_minor": sale, "reorder_level": reorder, "bottle_size": bottle, "serving_size": serving, "is_sellable": sell, "is_active": active})
	}
	if rows.Err() != nil {
		catalogError(w, rows.Err())
		return
	}
	jsonOut(w, 200, out)
}
func (s *server) createProduct(w http.ResponseWriter, r *http.Request) {
	tx, _, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in productInput
	if !decode(w, r, &in) {
		return
	}
	in.SKU = strings.TrimSpace(in.SKU)
	in.Name = strings.TrimSpace(in.Name)
	if !requiredText(in.SKU, 80) || !requiredText(in.Name, 160) || !requiredText(in.BaseUnitID, 64) || (in.Kind != "unit" && in.Kind != "liquid") || in.CostPriceMinor < 0 || in.SalePriceMinor < 0 {
		jsonOut(w, 400, map[string]string{"error": "invalid_product_definition"})
		return
	}
	if in.Kind == "liquid" && (in.BottleSize == nil || in.ServingSize == nil) {
		jsonOut(w, 400, map[string]string{"error": "liquid_product_sizes_required"})
		return
	}
	if in.Kind == "unit" && (in.BottleSize != nil || in.ServingSize != nil) {
		jsonOut(w, 400, map[string]string{"error": "unit_product_cannot_have_liquid_sizes"})
		return
	}
	var id string
	e := tx.QueryRow(r.Context(), `INSERT INTO products(business_id,category_id,base_unit_id,sku,name,description,product_kind,cost_price_minor,sale_price_minor,reorder_level,bottle_size,serving_size,is_sellable) VALUES($1,$2,$3,$4,$5,NULLIF($6,''),$7,$8,$9,COALESCE(NULLIF($10,''),'0')::numeric,$11::numeric,$12::numeric,$13) RETURNING id`, tenantBusiness(r), in.CategoryID, in.BaseUnitID, in.SKU, in.Name, in.Description, in.Kind, in.CostPriceMinor, in.SalePriceMinor, in.ReorderLevel, in.BottleSize, in.ServingSize, in.Sellable).Scan(&id)
	if e != nil {
		catalogError(w, e)
		return
	}
	jsonOut(w, 201, map[string]string{"id": id, "sku": in.SKU, "name": in.Name})
}

type supplierInput struct {
	Name        string `json:"name"`
	ContactName string `json:"contact_name"`
	Phone       string `json:"phone"`
	Email       string `json:"email"`
	Address     string `json:"address"`
	Notes       string `json:"notes"`
}

func (s *server) listSuppliers(w http.ResponseWriter, r *http.Request) {
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
	rows, e := tx.Query(r.Context(), `SELECT id,name,contact_name,phone,email,address,notes,is_active FROM suppliers WHERE ($1='' OR name ILIKE '%'||$1||'%' OR COALESCE(phone,'') ILIKE '%'||$1||'%' OR COALESCE(email,'') ILIKE '%'||$1||'%') ORDER BY name LIMIT $2 OFFSET $3`, search, limit, offset)
	if e != nil {
		catalogError(w, e)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name string
		var contact, phone, email, address, notes *string
		var active bool
		if e = rows.Scan(&id, &name, &contact, &phone, &email, &address, &notes, &active); e != nil {
			catalogError(w, e)
			return
		}
		out = append(out, map[string]any{"id": id, "name": name, "contact_name": contact, "phone": phone, "email": email, "address": address, "notes": notes, "is_active": active})
	}
	if rows.Err() != nil {
		catalogError(w, rows.Err())
		return
	}
	jsonOut(w, 200, out)
}
func (s *server) createSupplier(w http.ResponseWriter, r *http.Request) {
	tx, _, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in supplierInput
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if !requiredText(in.Name, 160) {
		jsonOut(w, 400, map[string]string{"error": "supplier_name_required"})
		return
	}
	var id string
	e := tx.QueryRow(r.Context(), `INSERT INTO suppliers(business_id,name,contact_name,phone,email,address,notes) VALUES($1,$2,NULLIF($3,''),NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,'')) RETURNING id`, tenantBusiness(r), in.Name, strings.TrimSpace(in.ContactName), strings.TrimSpace(in.Phone), strings.TrimSpace(in.Email), strings.TrimSpace(in.Address), strings.TrimSpace(in.Notes)).Scan(&id)
	if e != nil {
		catalogError(w, e)
		return
	}
	jsonOut(w, 201, map[string]string{"id": id, "name": in.Name})
}

type tableInput struct {
	Code     string `json:"table_code"`
	Name     string `json:"display_name"`
	Capacity int    `json:"capacity"`
	Area     string `json:"area"`
}

func (s *server) listTables(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	limit, offset, search, valid := listBounds(r)
	if !valid {
		jsonOut(w, 400, map[string]string{"error": "invalid_pagination_or_search"})
		return
	}
	rows, e := tx.Query(r.Context(), `SELECT id,table_code,display_name,capacity,area,is_active FROM bar_tables WHERE branch_id=$1 AND ($2='' OR table_code ILIKE '%'||$2||'%' OR display_name ILIKE '%'||$2||'%') ORDER BY table_code LIMIT $3 OFFSET $4`, tc.BranchID, search, limit, offset)
	if e != nil {
		catalogError(w, e)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, code, name string
		var area *string
		var capacity int
		var active bool
		if e = rows.Scan(&id, &code, &name, &capacity, &area, &active); e != nil {
			catalogError(w, e)
			return
		}
		out = append(out, map[string]any{"id": id, "table_code": code, "display_name": name, "capacity": capacity, "area": area, "is_active": active})
	}
	if rows.Err() != nil {
		catalogError(w, rows.Err())
		return
	}
	jsonOut(w, 200, out)
}
func (s *server) createTable(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in tableInput
	if !decode(w, r, &in) {
		return
	}
	in.Code = strings.TrimSpace(in.Code)
	in.Name = strings.TrimSpace(in.Name)
	if !requiredText(in.Code, 40) || !requiredText(in.Name, 80) || in.Capacity < 1 || in.Capacity > 1000 {
		jsonOut(w, 400, map[string]string{"error": "invalid_table_definition"})
		return
	}
	var id string
	e := tx.QueryRow(r.Context(), `INSERT INTO bar_tables(business_id,branch_id,table_code,display_name,capacity,area) VALUES($1,$2,$3,$4,$5,NULLIF($6,'')) RETURNING id`, tc.BusinessID, tc.BranchID, in.Code, in.Name, in.Capacity, strings.TrimSpace(in.Area)).Scan(&id)
	if e != nil {
		catalogError(w, e)
		return
	}
	jsonOut(w, 201, map[string]string{"id": id, "table_code": in.Code, "display_name": in.Name})
}

type recipeIngredientInput struct {
	ProductID string `json:"ingredient_product_id"`
	UnitID    string `json:"unit_id"`
	Quantity  string `json:"quantity"`
}
type recipeInput struct {
	OutputProductID string                  `json:"output_product_id"`
	Name            string                  `json:"name"`
	OutputQuantity  string                  `json:"output_quantity"`
	Instructions    string                  `json:"instructions"`
	Ingredients     []recipeIngredientInput `json:"ingredients"`
}

func (s *server) listRecipes(w http.ResponseWriter, r *http.Request) {
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
	rows, e := tx.Query(r.Context(), `SELECT id,output_product_id,name,output_quantity::text,instructions,is_active FROM recipes WHERE ($1='' OR name ILIKE '%'||$1||'%') ORDER BY name LIMIT $2 OFFSET $3`, search, limit, offset)
	if e != nil {
		catalogError(w, e)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, product, name, qty string
		var instructions *string
		var active bool
		if e = rows.Scan(&id, &product, &name, &qty, &instructions, &active); e != nil {
			catalogError(w, e)
			return
		}
		out = append(out, map[string]any{"id": id, "output_product_id": product, "name": name, "output_quantity": qty, "instructions": instructions, "is_active": active})
	}
	if rows.Err() != nil {
		catalogError(w, rows.Err())
		return
	}
	jsonOut(w, 200, out)
}
func (s *server) createRecipe(w http.ResponseWriter, r *http.Request) {
	tx, _, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in recipeInput
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if !requiredText(in.Name, 160) || !requiredText(in.OutputProductID, 64) || len(in.Ingredients) == 0 || len(in.Ingredients) > 100 {
		jsonOut(w, 400, map[string]string{"error": "invalid_recipe_definition"})
		return
	}
	var id string
	e := tx.QueryRow(r.Context(), `INSERT INTO recipes(business_id,output_product_id,name,output_quantity,instructions) VALUES($1,$2,$3,COALESCE(NULLIF($4,''),'1')::numeric,NULLIF($5,'')) RETURNING id`, tenantBusiness(r), in.OutputProductID, in.Name, in.OutputQuantity, in.Instructions).Scan(&id)
	if e != nil {
		catalogError(w, e)
		return
	}
	for _, ingredient := range in.Ingredients {
		if !requiredText(ingredient.ProductID, 64) || !requiredText(ingredient.UnitID, 64) || !requiredText(ingredient.Quantity, 32) {
			jsonOut(w, 400, map[string]string{"error": "invalid_recipe_ingredient"})
			return
		}
		if _, e = tx.Exec(r.Context(), `INSERT INTO recipe_ingredients(business_id,recipe_id,ingredient_product_id,unit_id,quantity) VALUES($1,$2,$3,$4,$5::numeric)`, tenantBusiness(r), id, ingredient.ProductID, ingredient.UnitID, ingredient.Quantity); e != nil {
			catalogError(w, e)
			return
		}
	}
	jsonOut(w, 201, map[string]string{"id": id, "name": in.Name})
}

// Phase 2 catalog maintenance endpoints deliberately deactivate records rather
// than deleting them, preserving references and historical reporting.
func (s *server) updateCategory(w http.ResponseWriter, r *http.Request) {
	tx, _, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in categoryInput
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if !requiredText(in.Name, 100) {
		jsonOut(w, 400, map[string]string{"error": "category_name_required"})
		return
	}
	tag, err := tx.Exec(r.Context(), `UPDATE product_categories SET name=$3,description=NULLIF($4,''),updated_at=now() WHERE business_id=$1 AND id=$2`, tenantBusiness(r), chi.URLParam(r, "id"), in.Name, strings.TrimSpace(in.Description))
	if err != nil {
		catalogError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		jsonOut(w, 404, map[string]string{"error": "category_not_found"})
		return
	}
	jsonOut(w, 200, map[string]string{"status": "updated"})
}
func (s *server) deactivateCategory(w http.ResponseWriter, r *http.Request) {
	s.setCatalogActive(w, r, "product_categories", false)
}
func (s *server) deactivateUnit(w http.ResponseWriter, r *http.Request) {
	s.setCatalogActive(w, r, "units_of_measure", false)
}
func (s *server) deactivateSupplier(w http.ResponseWriter, r *http.Request) {
	s.setCatalogActive(w, r, "suppliers", false)
}
func (s *server) deactivateTable(w http.ResponseWriter, r *http.Request) {
	s.setCatalogActive(w, r, "bar_tables", false)
}
func (s *server) deactivateProduct(w http.ResponseWriter, r *http.Request) {
	s.setCatalogActive(w, r, "products", false)
}
func (s *server) setCatalogActive(w http.ResponseWriter, r *http.Request, table string, active bool) {
	tx, _, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	// Table names are selected only by the fixed internal wrappers above.
	q := `UPDATE ` + table + ` SET is_active=$3,updated_at=now() WHERE business_id=$1 AND id=$2`
	tag, err := tx.Exec(r.Context(), q, tenantBusiness(r), chi.URLParam(r, "id"), active)
	if err != nil {
		catalogError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		jsonOut(w, 404, map[string]string{"error": "catalog_record_not_found"})
		return
	}
	jsonOut(w, 200, map[string]string{"status": "deactivated"})
}

type productUpdateInput struct {
	CategoryID     *string `json:"category_id"`
	Name           string  `json:"name"`
	Description    string  `json:"description"`
	CostPriceMinor int64   `json:"cost_price_minor"`
	SalePriceMinor int64   `json:"sale_price_minor"`
	ReorderLevel   string  `json:"reorder_level"`
	Sellable       bool    `json:"is_sellable"`
}

func (s *server) updateProduct(w http.ResponseWriter, r *http.Request) {
	tx, _, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in productUpdateInput
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if !requiredText(in.Name, 160) || in.CostPriceMinor < 0 || in.SalePriceMinor < 0 {
		jsonOut(w, 400, map[string]string{"error": "invalid_product_update"})
		return
	}
	tag, err := tx.Exec(r.Context(), `UPDATE products SET category_id=$3,name=$4,description=NULLIF($5,''),cost_price_minor=$6,sale_price_minor=$7,reorder_level=$8::numeric,is_sellable=$9,updated_at=now() WHERE business_id=$1 AND id=$2`, tenantBusiness(r), chi.URLParam(r, "id"), in.CategoryID, in.Name, strings.TrimSpace(in.Description), in.CostPriceMinor, in.SalePriceMinor, strings.TrimSpace(in.ReorderLevel), in.Sellable)
	if err != nil {
		catalogError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		jsonOut(w, 404, map[string]string{"error": "product_not_found"})
		return
	}
	jsonOut(w, 200, map[string]string{"status": "updated"})
}

type supplierUpdateInput struct {
	Name        string `json:"name"`
	ContactName string `json:"contact_name"`
	Phone       string `json:"phone"`
	Email       string `json:"email"`
	Address     string `json:"address"`
	Notes       string `json:"notes"`
}

func (s *server) updateSupplier(w http.ResponseWriter, r *http.Request) {
	tx, _, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in supplierUpdateInput
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if !requiredText(in.Name, 160) {
		jsonOut(w, 400, map[string]string{"error": "supplier_name_required"})
		return
	}
	tag, err := tx.Exec(r.Context(), `UPDATE suppliers SET name=$3,contact_name=NULLIF($4,''),phone=NULLIF($5,''),email=NULLIF($6,''),address=NULLIF($7,''),notes=NULLIF($8,''),updated_at=now() WHERE business_id=$1 AND id=$2`, tenantBusiness(r), chi.URLParam(r, "id"), in.Name, strings.TrimSpace(in.ContactName), strings.TrimSpace(in.Phone), strings.TrimSpace(in.Email), strings.TrimSpace(in.Address), strings.TrimSpace(in.Notes))
	if err != nil {
		catalogError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		jsonOut(w, 404, map[string]string{"error": "supplier_not_found"})
		return
	}
	jsonOut(w, 200, map[string]string{"status": "updated"})
}

type tableUpdateInput struct {
	Code     string `json:"table_code"`
	Name     string `json:"display_name"`
	Capacity int    `json:"capacity"`
	Area     string `json:"area"`
}

func (s *server) updateTable(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in tableUpdateInput
	if !decode(w, r, &in) {
		return
	}
	in.Code = strings.TrimSpace(in.Code)
	in.Name = strings.TrimSpace(in.Name)
	if !requiredText(in.Code, 40) || !requiredText(in.Name, 80) || in.Capacity < 1 || in.Capacity > 1000 {
		jsonOut(w, 400, map[string]string{"error": "invalid_table_definition"})
		return
	}
	tag, err := tx.Exec(r.Context(), `UPDATE bar_tables SET table_code=$4,display_name=$5,capacity=$6,area=NULLIF($7,''),updated_at=now() WHERE business_id=$1 AND branch_id=$2 AND id=$3`, tc.BusinessID, tc.BranchID, chi.URLParam(r, "id"), in.Code, in.Name, in.Capacity, strings.TrimSpace(in.Area))
	if err != nil {
		catalogError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		jsonOut(w, 404, map[string]string{"error": "table_not_found"})
		return
	}
	jsonOut(w, 200, map[string]string{"status": "updated"})
}

// recipeCost reports a live estimate using each ingredient product's current
// catalog cost_price_minor. It is not an inventory valuation or historical
// sale snapshot. Conversion factors are directional: from_unit -> to_unit.
// No currency rounding is applied; amounts are returned as decimal minor units.
func (s *server) recipeCost(w http.ResponseWriter, r *http.Request) {
	tx, _, ok := catalogTx(r)
	if !ok {
		jsonOut(w, http.StatusInternalServerError, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var result struct {
		RecipeID       string `json:"recipe_id"`
		Name           string `json:"name"`
		OutputQuantity string `json:"output_quantity"`
		OutputPrice    int64  `json:"output_sale_price_minor"`
		TotalCost      string `json:"total_recipe_cost_minor"`
		UnitCost       string `json:"cost_per_output_unit_minor"`
		Margin         string `json:"gross_margin_minor"`
		MarginPercent  string `json:"gross_margin_percent"`
	}
	err := tx.QueryRow(r.Context(), `
		WITH ingredient_costs AS (
		 SELECT ri.quantity * p.cost_price_minor::numeric *
		   CASE WHEN ri.unit_id = p.base_unit_id THEN 1::numeric
		        ELSE uc.factor END AS cost_minor
		 FROM recipe_ingredients ri
		 JOIN products p ON p.business_id=ri.business_id AND p.id=ri.ingredient_product_id
		 LEFT JOIN unit_conversions uc ON uc.business_id=ri.business_id
		   AND uc.from_unit_id=ri.unit_id AND uc.to_unit_id=p.base_unit_id
		 WHERE ri.business_id=$1 AND ri.recipe_id=$2
		), totals AS (
		 SELECT count(*) AS ingredient_count, count(cost_minor) AS priced_count,
		        COALESCE(sum(cost_minor),0)::numeric AS total_cost
		 FROM ingredient_costs
		)
		SELECT r.id,r.name,r.output_quantity::text,p.sale_price_minor,
		       t.total_cost::text,
		       (t.total_cost / r.output_quantity)::text,
		       (p.sale_price_minor::numeric - t.total_cost/r.output_quantity)::text,
		       CASE WHEN p.sale_price_minor=0 THEN NULL
		            ELSE ((p.sale_price_minor::numeric - t.total_cost/r.output_quantity) * 100 / p.sale_price_minor::numeric)::text END,
		       t.ingredient_count,t.priced_count
		FROM recipes r JOIN products p ON p.business_id=r.business_id AND p.id=r.output_product_id
		CROSS JOIN totals t
		WHERE r.business_id=$1 AND r.id=$2 AND r.is_active=true`, tenantBusiness(r), chi.URLParam(r, "id")).Scan(
		&result.RecipeID, &result.Name, &result.OutputQuantity, &result.OutputPrice,
		&result.TotalCost, &result.UnitCost, &result.Margin, &result.MarginPercent,
		new(int), new(int))
	if err == pgx.ErrNoRows {
		jsonOut(w, http.StatusNotFound, map[string]string{"error": "recipe_not_found"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	// Check conversions separately so a missing conversion is never mistaken
	// for a zero-cost ingredient (the LEFT JOIN above otherwise yields NULL).
	var missing int
	if err = tx.QueryRow(r.Context(), `
		SELECT count(*) FROM recipe_ingredients ri
		JOIN products p ON p.business_id=ri.business_id AND p.id=ri.ingredient_product_id
		WHERE ri.business_id=$1 AND ri.recipe_id=$2 AND ri.unit_id<>p.base_unit_id
		AND NOT EXISTS (SELECT 1 FROM unit_conversions uc WHERE uc.business_id=ri.business_id
		 AND uc.from_unit_id=ri.unit_id AND uc.to_unit_id=p.base_unit_id)`, tenantBusiness(r), result.RecipeID).Scan(&missing); err != nil {
		catalogError(w, err)
		return
	}
	if missing > 0 {
		jsonOut(w, http.StatusUnprocessableEntity, map[string]string{"error": "missing_directional_unit_conversion", "detail": "Add a conversion from each recipe ingredient unit to that product's base unit."})
		return
	}
	jsonOut(w, http.StatusOK, map[string]any{
		"recipe_id": result.RecipeID, "name": result.Name,
		"output_quantity":            result.OutputQuantity,
		"output_sale_price_minor":    result.OutputPrice,
		"total_recipe_cost_minor":    result.TotalCost,
		"cost_per_output_unit_minor": result.UnitCost,
		"gross_margin_minor":         result.Margin,
		"gross_margin_percent":       result.MarginPercent,
		"cost_basis":                 "current_products.cost_price_minor",
		"valuation_policy":           "not_applied",
		"rounding":                   "none; decimal minor-unit values are returned",
	})
}
