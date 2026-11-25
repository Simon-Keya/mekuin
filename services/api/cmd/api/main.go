package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type server struct {
	db           *pgxpool.Pool
	log          *slog.Logger
	cookieSecure bool
	sessionTTL   time.Duration
}

type ctxKey string

const userIDKey ctxKey = "user_id"
const sessionIDKey ctxKey = "session_id"
const tenantContextKey ctxKey = "tenant_context"
const tenantTxKey ctxKey = "tenant_tx"

type registerRequest struct {
	FullName     string `json:"full_name"`
	Email        string `json:"email"`
	Password     string `json:"password"`
	BusinessName string `json:"business_name"`
	BranchName   string `json:"branch_name"`
	Phone        string `json:"phone"`
}
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
type tenantSelectionRequest struct {
	BusinessID string `json:"business_id"`
	BranchID   string `json:"branch_id"`
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		logger.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	pool, err := pgxpool.New(ctx, dsn)
	cancel()
	if err != nil {
		logger.Error("database pool initialization failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	secure := strings.EqualFold(os.Getenv("SESSION_COOKIE_SECURE"), "true")
	hours := 168
	if v := os.Getenv("SESSION_TTL_HOURS"); v != "" {
		if n, e := fmt.Sscanf(v, "%d", &hours); e != nil || n != 1 || hours < 1 || hours > 720 {
			hours = 168
		}
	}
	s := &server{db: pool, log: logger, cookieSecure: secure, sessionTTL: time.Duration(hours) * time.Hour}

	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer)
	r.Use(middleware.Timeout(20 * time.Second))
	r.Use(corsMiddleware(env("WEB_ORIGIN", "http://localhost:3000")))
	r.Get("/health/live", func(w http.ResponseWriter, _ *http.Request) { jsonOut(w, 200, map[string]string{"status": "ok"}) })
	r.Get("/health/ready", s.ready)
	r.Get("/openapi.json", s.openAPI)
	// Safaricom server-to-server callback: deliberately outside cookie auth and browser-origin checks.
	r.Post("/daraja/stk/callback", s.receiveDarajaSTKCallback)
	r.Route("/api/v1", func(api chi.Router) {
		// Enforce exact-origin checks for every state-changing endpoint, including
		// registration and login, before any session cookie can be issued.
		api.Use(s.requireSameOrigin)
		api.Post("/auth/register", s.register)
		api.Post("/auth/invitations/register", s.registerStaffInvitee)
		api.Post("/auth/login", s.login)
		api.Post("/auth/google", s.googleSignIn)
		api.Post("/auth/email/verify", s.verifyEmail)
		api.Post("/auth/email/resend", s.resendVerification)
		api.Post("/auth/password/forgot", s.requestPasswordReset)
		api.Post("/auth/password/reset", s.resetPassword)
		api.Post("/auth/mfa/verify", s.verifyLoginMFA)
		api.Group(func(authed chi.Router) {
			authed.Use(s.authenticate)
			authed.Route("/platform", func(platform chi.Router) {
				platform.Use(s.requireSuperAdmin)
				platform.Get("/businesses", s.listPlatformBusinesses)
				platform.Post("/businesses", s.provisionPlatformBusiness)
				platform.Patch("/businesses/{businessID}/status", s.setPlatformBusinessStatus)
				platform.Get("/users", s.listPlatformUsers)
				platform.Patch("/users/{userID}/status", s.setPlatformUserStatus)
				platform.Get("/audit", s.listPlatformAudit)
			})
			authed.Get("/auth/me", s.me)
			authed.Get("/auth/mfa/status", s.mfaStatus)
			authed.Post("/auth/mfa/setup", s.mfaSetup)
			authed.Post("/auth/mfa/enable", s.mfaEnable)
			authed.Post("/auth/mfa/disable", s.mfaDisable)
			authed.Post("/auth/logout", s.logout)
			authed.Post("/auth/invitations/accept", s.acceptStaffInvitation)
			authed.Get("/workspace", s.workspace)
			authed.Post("/tenant/select", s.selectTenant)
			authed.Group(func(tenant chi.Router) {
				tenant.Use(s.requireTenant)
				tenant.Get("/tenant/context", s.tenantInfo)
				tenant.Route("/branches", func(br chi.Router) {
					br.With(requirePermission("staff.read")).Get("/", s.listBranches)
					br.With(requirePermission("staff.manage")).Post("/", s.createBranch)
					br.With(requirePermission("staff.manage")).Put("/{branchID}", s.updateBranch)
				})
				tenant.Route("/staff", func(st chi.Router) {
					st.With(requirePermission("staff.read")).Get("/", s.listStaff)
					st.With(requirePermission("staff.manage")).Post("/memberships", s.createStaffMembership)
					st.With(requirePermission("staff.manage")).Post("/invitations", s.createStaffInvitation)
					st.With(requirePermission("staff.manage")).Post("/invitations/{invitationID}/revoke", s.revokeStaffInvitation)
					st.With(requirePermission("staff.manage")).Post("/profiles", s.createStaffProfile)
					st.With(requirePermission("staff.manage")).Put("/{membershipID}/branches", s.updateStaffBranches)
					st.With(requirePermission("staff.manage")).Put("/{membershipID}/role", s.updateStaffRole)
					st.With(requirePermission("staff.manage")).Put("/{membershipID}/manager-pin", s.setManagerPIN)
					st.With(requirePermission("pos.use")).Post("/authorize-action", s.authorizeManagerAction)
				})
				tenant.Route("/payments/mpesa", func(mp chi.Router) {
					mp.With(requirePermission("pos.use")).Get("/", s.listMPesaAttempts)
					mp.With(requirePermission("pos.use")).Post("/{attemptID}/review-notes", s.recordMPesaReviewNote)
					mp.With(requirePermission("pos.use")).Post("/{attemptID}/reconcile", s.reconcileDarajaAttempt)
					mp.With(requirePermission("pos.use")).Post("/{attemptID}/settle", s.settleDarajaAttempt)
				})
				tenant.Route("/sales", func(sales chi.Router) {
					sales.With(requirePermission("pos.use")).Post("/quote", s.quoteSale)
					sales.With(requirePermission("pos.use")).Post("/checkout", s.checkoutSale)
					sales.With(requirePermission("pos.use")).Post("/orders/{orderID}/checkout", s.checkoutPOSOrder)
					sales.With(requirePermission("pos.use")).Post("/orders", s.createPOSOrder)
					sales.With(requirePermission("pos.use")).Post("/orders/reconcile", s.reconcilePOSOrders)
					sales.With(requirePermission("pos.use")).Get("/orders", s.listPOSOrders)
					sales.With(requirePermission("pos.use")).Get("/orders/{orderID}", s.retrievePOSOrder)
					sales.With(requirePermission("pos.use")).Post("/orders/{orderID}/hold", s.holdPOSOrder)
					sales.With(requirePermission("pos.use")).Put("/orders/{orderID}", s.updatePOSOrder)
					sales.With(requirePermission("pos.use")).Post("/orders/{orderID}/reopen", s.reopenPOSOrder)
					sales.With(requirePermission("pos.use")).Post("/orders/{orderID}/cancel", s.cancelPOSOrder)
					sales.With(requirePermission("sales.void")).Post("/{saleID}/void", s.voidSale)
					sales.With(requirePermission("sales.refund")).Post("/{saleID}/refunds", s.createSaleRefund)
				})
				tenant.Route("/payables", func(ap chi.Router) {
					ap.With(requirePermission("finance.read")).Get("/invoices", s.listSupplierInvoices)
					ap.With(requirePermission("finance.manage")).Post("/invoices", s.createSupplierInvoice)
					ap.With(requirePermission("finance.read")).Get("/invoices/{invoiceID}", s.getSupplierInvoice)
					ap.With(requirePermission("finance.read")).Get("/invoices/{invoiceID}/match", s.getSupplierInvoiceMatch)
					ap.With(requirePermission("finance.read")).Get("/invoices/{invoiceID}/reviews", s.listSupplierInvoiceReviewEvents)
					ap.With(requirePermission("finance.manage")).Post("/invoices/{invoiceID}/submit", s.submitSupplierInvoiceForReview)
					ap.With(requirePermission("finance.approve")).Post("/invoices/{invoiceID}/review", s.reviewSupplierInvoice)
					ap.With(requirePermission("finance.read")).Get("/payments", s.listSupplierPayments)
					ap.With(requirePermission("finance.manage")).Post("/payments", s.createSupplierPayment)
					ap.With(requirePermission("finance.read")).Get("/suppliers/{supplierID}/statement", s.getSupplierStatement)
					ap.With(requirePermission("finance.read")).Get("/reports/aging", s.getSupplierAgingReport)
					ap.With(requirePermission("finance.read")).Get("/reports/reconciliation", s.getPaymentAllocationReconciliation)
				})
				tenant.Route("/purchasing/orders", func(po chi.Router) {
					po.With(requirePermission("purchasing.manage")).Get("/", s.listPurchaseOrders)
					po.With(requirePermission("purchasing.manage")).Post("/", s.createPurchaseOrder)
					po.With(requirePermission("purchasing.manage")).Get("/{orderID}", s.getPurchaseOrder)
					po.With(requirePermission("purchasing.manage")).Post("/{orderID}/order", s.orderPurchaseOrder)
					po.With(requirePermission("purchasing.manage")).Post("/{orderID}/cancel", s.cancelPurchaseOrder)
					po.With(requirePermission("purchasing.receive")).Post("/{orderID}/receive", s.receivePurchaseOrder)
				})
				tenant.Route("/settings/operational", func(st chi.Router) {
					st.With(requirePermission("settings.read")).Get("/", s.getOperationalSettings)
					st.With(requirePermission("settings.manage")).Put("/", s.updateOperationalSettings)
				})
				tenant.Get("/settings/integrations", s.listIntegrationSettings)
				tenant.Put("/settings/integrations/{provider}", s.updateIntegrationSetting)
				tenant.Route("/alerts", func(al chi.Router) {
					al.With(requirePermission("alerts.read")).Get("/", s.listOperationalAlerts)
					al.With(requirePermission("alerts.manage")).Post("/{alertID}/action", s.updateOperationalAlert)
				})
				tenant.Route("/audit", func(au chi.Router) {
					au.With(requirePermission("audit.read")).Get("/", s.listAuditLedger)
				})
				tenant.Route("/reports", func(rep chi.Router) {
					rep.With(requirePermission("expenses.read")).Get("/operational-summary", s.getOperationalReportSummary)
					rep.With(requirePermission("reports.read")).Get("/summary", s.getBuildSpecReport)
				})
				tenant.Route("/expenses", func(ex chi.Router) {
					ex.With(requirePermission("expenses.read")).Get("/categories", s.listExpenseCategories)
					ex.With(requirePermission("expenses.manage")).Post("/categories", s.createExpenseCategory)
					ex.With(requirePermission("expenses.read")).Get("/", s.listExpenses)
					ex.With(requirePermission("expenses.manage")).Post("/", s.createExpense)
					ex.With(requirePermission("expenses.read")).Get("/{expenseID}/reviews", s.listExpenseReviewEvents)
					ex.With(requirePermission("expenses.approve")).Post("/{expenseID}/review", s.reviewExpense)
					ex.With(requirePermission("expenses.manage")).Post("/{expenseID}/pay", s.payExpense)
					ex.With(requirePermission("expenses.read")).Get("/reports/summary", s.expenseSummaryReport)
				})
				tenant.Route("/shifts", func(sh chi.Router) {
					sh.With(requirePermission("shifts.own")).Get("/history", s.listCashShiftHistory)
					sh.With(requirePermission("shifts.own")).Get("/current", s.getCurrentCashShift)
					sh.With(requirePermission("shifts.own")).Post("/", s.openCashShift)
					sh.With(requirePermission("shifts.own")).Post("/{shiftID}/close", s.closeCashShift)
					sh.With(requirePermission("shifts.own")).Post("/{shiftID}/movements", s.recordCashShiftMovement)
					sh.With(requirePermission("shifts.own")).Get("/{shiftID}/summary", s.getCashShiftSummary)
				})
				tenant.Route("/inventory/counts", func(counts chi.Router) {
					counts.With(requirePermission("inventory.read")).Get("/", s.listInventoryCounts)
					counts.With(requirePermission("inventory.read")).Get("/{countID}", s.getInventoryCount)
					counts.With(requirePermission("inventory.count.create")).Post("/", s.createInventoryCount)
					counts.With(requirePermission("inventory.count.create")).Patch("/{countID}", s.updateInventoryCount)
					counts.With(requirePermission("inventory.count.create")).Post("/{countID}/submit", s.submitInventoryCount)
					counts.With(requirePermission("inventory.count.reconcile")).Post("/{countID}/reconcile", s.reconcileInventoryCount)
				})
				tenant.Route("/inventory", func(inv chi.Router) {
					inv.With(requirePermission("inventory.read")).Get("/balances", s.listInventoryBalances)
					inv.With(requirePermission("inventory.read")).Get("/valuation", s.getInventoryValuation)
					inv.With(requirePermission("inventory.read")).Get("/branches", s.listInventoryBranches)
					inv.With(requirePermission("inventory.read")).Get("/reconciliation", s.getInventoryReconciliation)
					inv.With(requirePermission("inventory.read")).Get("/movements", s.listInventoryMovements)
					inv.With(requirePermission("inventory.adjust")).Post("/adjustments", s.adjustInventory)
					inv.With(requirePermission("inventory.adjust")).Post("/losses/{kind}", s.recordInventoryLoss)
					inv.With(requirePermission("inventory.adjust")).Post("/transfers", s.transferInventory)
				})
				tenant.Route("/catalog", func(c chi.Router) {
					c.With(requirePermission("products.read")).Get("/categories", s.listCategories)
					c.With(requirePermission("products.manage")).Post("/categories", s.createCategory)
					c.With(requirePermission("products.manage")).Patch("/categories/{id}", s.updateCategory)
					c.With(requirePermission("products.manage")).Post("/categories/{id}/deactivate", s.deactivateCategory)
					c.With(requirePermission("products.read")).Get("/units", s.listUnits)
					c.With(requirePermission("products.manage")).Post("/units", s.createUnit)
					c.With(requirePermission("products.manage")).Post("/units/{id}/deactivate", s.deactivateUnit)
					c.With(requirePermission("products.read")).Get("/unit-conversions", s.listUnitConversions)
					c.With(requirePermission("products.manage")).Post("/unit-conversions", s.createUnitConversion)
					c.With(requirePermission("products.read")).Get("/products", s.listProducts)
					c.With(requirePermission("products.manage")).Post("/products", s.createProduct)
					c.With(requirePermission("products.manage")).Patch("/products/{id}", s.updateProduct)
					c.With(requirePermission("products.manage")).Post("/products/{id}/deactivate", s.deactivateProduct)
					c.With(requirePermission("suppliers.read")).Get("/suppliers", s.listSuppliers)
					c.With(requirePermission("suppliers.manage")).Post("/suppliers", s.createSupplier)
					c.With(requirePermission("suppliers.manage")).Patch("/suppliers/{id}", s.updateSupplier)
					c.With(requirePermission("suppliers.manage")).Post("/suppliers/{id}/deactivate", s.deactivateSupplier)
					c.With(requirePermission("pos.use")).Get("/tables", s.listTables)
					c.With(requirePermission("tables.manage")).Post("/tables", s.createTable)
					c.With(requirePermission("tables.manage")).Patch("/tables/{id}", s.updateTable)
					c.With(requirePermission("tables.manage")).Post("/tables/{id}/deactivate", s.deactivateTable)
					c.With(requirePermission("products.read")).Get("/recipes", s.listRecipes)
					c.With(requirePermission("products.manage")).Post("/recipes", s.createRecipe)
					c.With(requirePermission("products.read")).Get("/recipes/{id}/cost", s.recipeCost)
				})
			})
		})
	})

	port := env("PORT", "8080")
	srv := &http.Server{Addr: ":" + port, Handler: r, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		logger.Info("MEKU-In API listening", "port", port)
		if e := srv.ListenAndServe(); e != nil && !errors.Is(e, http.ErrServerClosed) {
			logger.Error("server failed", "error", e)
			os.Exit(1)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	go s.runEmailOutboxWorker(workerCtx)
	go s.runMPesaDispatchWorker(workerCtx)
	<-stop
	cancelWorker()
	shutdown, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	_ = srv.Shutdown(shutdown)
}

// requireSameOrigin protects cookie-authenticated state changes from cross-site requests.
func (s *server) requireSameOrigin(next http.Handler) http.Handler {
	allowed, err := url.Parse(env("WEB_ORIGIN", "http://localhost:3000"))
	if err != nil {
		allowed = &url.URL{Scheme: "http", Host: "localhost:3000"}
	}
	want := allowed.Scheme + "://" + allowed.Host
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("Origin") != want {
			jsonOut(w, http.StatusForbidden, map[string]string{"error": "origin_not_allowed"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.Ping(ctx); err != nil {
		jsonOut(w, 503, map[string]string{"status": "not_ready"})
		return
	}
	jsonOut(w, 200, map[string]string{"status": "ready"})
}
func (s *server) register(w http.ResponseWriter, r *http.Request) {
	var in registerRequest
	if !decode(w, r, &in) {
		return
	}
	in.FullName = strings.TrimSpace(in.FullName)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.BusinessName = strings.TrimSpace(in.BusinessName)
	in.BranchName = strings.TrimSpace(in.BranchName)
	in.Phone = strings.TrimSpace(in.Phone)
	if in.BranchName == "" {
		in.BranchName = "Main Branch"
	}
	if !validRegistration(in) {
		jsonOut(w, 400, map[string]string{"error": "Provide a valid name, email, password (12–72 bytes), business and branch name"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		jsonOut(w, 500, map[string]string{"error": "could_not_create_account"})
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		jsonOut(w, 500, map[string]string{"error": "could_not_create_workspace"})
		return
	}
	defer tx.Rollback(r.Context())
	var uid, bid, branchID string
	if _, err = tx.Exec(r.Context(), `SELECT set_config('app.registration','true',true)`); err != nil {
		jsonOut(w, 500, map[string]string{"error": "could_not_start_registration"})
		return
	}
	err = tx.QueryRow(r.Context(), `INSERT INTO app_users(full_name,email,password_hash) VALUES($1,$2,$3) RETURNING id`, in.FullName, in.Email, string(hash)).Scan(&uid)
	if err != nil {
		jsonOut(w, 409, map[string]string{"error": "account_already_exists_or_invalid"})
		return
	}
	if _, err = tx.Exec(r.Context(), `SELECT set_config('app.user_id',$1,true)`, uid); err != nil {
		jsonOut(w, 500, map[string]string{"error": "could_not_set_user_context"})
		return
	}
	err = tx.QueryRow(r.Context(), `INSERT INTO businesses(name,phone) VALUES($1,NULLIF($2,'')) RETURNING id`, in.BusinessName, in.Phone).Scan(&bid)
	if err != nil {
		jsonOut(w, 500, map[string]string{"error": "could_not_create_business"})
		return
	}
	if _, err = tx.Exec(r.Context(), `SELECT set_config('app.business_id',$1,true)`, bid); err != nil {
		jsonOut(w, 500, map[string]string{"error": "could_not_set_business_context"})
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO expense_categories(business_id,name) SELECT $1,v.name FROM (VALUES ('Rent'),('Utilities'),('Transport'),('Repairs & maintenance'),('Cleaning'),('Staff welfare'),('Office supplies'),('Marketing'),('Licences & fees'),('Other')) v(name)`, bid)
	if err != nil {
		jsonOut(w, 500, map[string]string{"error": "registration_failed"})
		return
	}
	err = tx.QueryRow(r.Context(), `INSERT INTO branches(business_id,name,phone) VALUES($1,$2,NULLIF($3,'')) RETURNING id`, bid, in.BranchName, in.Phone).Scan(&branchID)
	if err != nil {
		jsonOut(w, 500, map[string]string{"error": "could_not_create_branch"})
		return
	}
	var mid string
	err = tx.QueryRow(r.Context(), `INSERT INTO memberships(business_id,user_id,role) VALUES($1,$2,'owner') RETURNING id`, bid, uid).Scan(&mid)
	if err != nil {
		jsonOut(w, 500, map[string]string{"error": "could_not_create_owner_membership"})
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO membership_branches(business_id,membership_id,branch_id) VALUES($1,$2,$3)`, bid, mid, branchID)
	if err != nil {
		jsonOut(w, 500, map[string]string{"error": "could_not_assign_owner_branch"})
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		jsonOut(w, 500, map[string]string{"error": "could_not_commit_workspace"})
		return
	}
	if err = s.issueEmailCode(uid, in.Email, "verify_email", "Verify your MEKU-In email", "Your MEKU-In verification code is %s. It expires in 15 minutes."); err != nil {
		jsonOut(w, 503, map[string]string{"error": "account_created_email_delivery_unavailable", "next": "request_a_verification_code_after_email_is_configured"})
		return
	}
	jsonOut(w, 201, map[string]any{"user": map[string]string{"id": uid, "full_name": in.FullName, "email": in.Email}, "business": map[string]string{"id": bid, "name": in.BusinessName}, "branch": map[string]string{"id": branchID, "name": in.BranchName}, "role": "owner", "status": "verification_required"})
}
func (s *server) login(w http.ResponseWriter, r *http.Request) {
	var in loginRequest
	if !decode(w, r, &in) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if !validLogin(email, in.Password) {
		jsonOut(w, 401, map[string]string{"error": "invalid_email_or_password"})
		return
	}
	attempts, err := s.consumeLoginAttempt(r, email)
	if err != nil {
		jsonOut(w, 503, map[string]string{"error": "authentication_temporarily_unavailable"})
		return
	}
	if attempts > 10 {
		w.Header().Set("Retry-After", "900")
		jsonOut(w, 429, map[string]string{"error": "too_many_login_attempts", "retry_after_seconds": "900"})
		return
	}
	var id, hash, status string
	var verifiedAt *time.Time
	var mfaEnabled bool
	err = s.db.QueryRow(r.Context(), `SELECT user_id,password_hash,account_status,email_verified_at,mfa_enabled FROM lookup_auth_user($1)`, email).Scan(&id, &hash, &status, &verifiedAt, &mfaEnabled)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		jsonOut(w, 401, map[string]string{"error": "invalid_email_or_password"})
		return
	}
	if status != "active" {
		jsonOut(w, 403, map[string]string{"error": "account_suspended"})
		return
	}
	if verifiedAt == nil {
		jsonOut(w, 403, map[string]string{"error": "email_verification_required"})
		return
	}
	if mfaEnabled {
		var challengeID string
		err = s.db.QueryRow(r.Context(), `INSERT INTO auth_mfa_challenges(user_id,code_hash,expires_at) VALUES($1,$2,now()+interval '5 minutes') RETURNING id`, id, authCodeHash("mfa_challenge", id+time.Now().UTC().Format(time.RFC3339Nano))).Scan(&challengeID)
		if err != nil {
			jsonOut(w, 503, map[string]string{"error": "mfa_challenge_failed"})
			return
		}
		jsonOut(w, 200, map[string]string{"status": "mfa_required", "challenge_id": challengeID})
		return
	}
	if err = s.createSession(w, r, id); err != nil {
		jsonOut(w, 500, map[string]string{"error": "could_not_create_session"})
		return
	}
	if _, err = s.db.Exec(r.Context(), `DELETE FROM auth_login_limits WHERE email_key=$1`, email); err != nil {
		s.log.Warn("could not clear login throttle", "error", err)
	}
	jsonOut(w, 200, map[string]string{"status": "authenticated"})
}
func (s *server) consumeLoginAttempt(r *http.Request, email string) (int, error) {
	var attempts int
	err := s.db.QueryRow(r.Context(), `INSERT INTO auth_login_limits(email_key,attempts,window_started_at) VALUES($1,1,now()) ON CONFLICT(email_key) DO UPDATE SET attempts=CASE WHEN auth_login_limits.window_started_at <= now() - interval '15 minutes' THEN 1 ELSE auth_login_limits.attempts+1 END, window_started_at=CASE WHEN auth_login_limits.window_started_at <= now() - interval '15 minutes' THEN now() ELSE auth_login_limits.window_started_at END, updated_at=now() RETURNING attempts`, email).Scan(&attempts)
	return attempts, err
}

func (s *server) createSession(w http.ResponseWriter, r *http.Request, userID string) error {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	token := hex.EncodeToString(raw)
	digest := sha256.Sum256([]byte(token))
	expires := time.Now().Add(s.sessionTTL)
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `SELECT set_config('app.user_id',$1,true)`, userID); err != nil {
		return err
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO user_sessions(user_id,token_hash,expires_at) VALUES($1,$2,$3)`, userID, digest[:], expires); err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: "mekuin_session", Value: token, Path: "/", HttpOnly: true, Secure: s.cookieSecure, SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: int(s.sessionTTL.Seconds())})
	return nil
}
func (s *server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("mekuin_session")
		if err != nil || c.Value == "" {
			jsonOut(w, 401, map[string]string{"error": "authentication_required"})
			return
		}
		digest := sha256.Sum256([]byte(c.Value))
		var uid, status, sid string
		err = s.db.QueryRow(r.Context(), `SELECT user_id,account_status,session_id FROM lookup_session($1)`, digest[:]).Scan(&uid, &status, &sid)
		if err != nil || status != "active" {
			clearCookie(w, s.cookieSecure)
			jsonOut(w, 401, map[string]string{"error": "session_invalid_or_account_suspended"})
			return
		}
		ctx := context.WithValue(r.Context(), userIDKey, uid)
		ctx = context.WithValue(ctx, sessionIDKey, sid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
func (s *server) me(w http.ResponseWriter, r *http.Request) {
	uid := r.Context().Value(userIDKey).(string)
	var name, email string
	tx, txErr := s.db.Begin(r.Context())
	if txErr != nil {
		jsonOut(w, 503, map[string]string{"error": "account_unavailable"})
		return
	}
	defer tx.Rollback(r.Context())
	if _, txErr = tx.Exec(r.Context(), `SELECT set_config('app.user_id',$1,true)`, uid); txErr != nil {
		jsonOut(w, 503, map[string]string{"error": "account_unavailable"})
		return
	}
	if err := tx.QueryRow(r.Context(), `SELECT full_name,email FROM app_users WHERE id=$1 AND account_status='active'`, uid).Scan(&name, &email); err != nil {
		jsonOut(w, 401, map[string]string{"error": "account_unavailable"})
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		jsonOut(w, 503, map[string]string{"error": "account_unavailable"})
		return
	}
	jsonOut(w, 200, map[string]any{"id": uid, "full_name": name, "email": email})
}
func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("mekuin_session"); err == nil {
		d := sha256.Sum256([]byte(c.Value))
		uid, _ := r.Context().Value(userIDKey).(string)
		tx, err := s.db.Begin(r.Context())
		if err != nil {
			clearCookie(w, s.cookieSecure)
			jsonOut(w, 503, map[string]string{"error": "session_revocation_unavailable"})
			return
		}
		defer tx.Rollback(r.Context())
		if _, err = tx.Exec(r.Context(), `SELECT set_config('app.user_id',$1,true)`, uid); err != nil {
			clearCookie(w, s.cookieSecure)
			jsonOut(w, 503, map[string]string{"error": "session_revocation_unavailable"})
			return
		}
		if _, err = tx.Exec(r.Context(), `UPDATE user_sessions SET revoked_at=now() WHERE token_hash=$1 AND user_id=$2 AND revoked_at IS NULL`, d[:], uid); err != nil {
			clearCookie(w, s.cookieSecure)
			jsonOut(w, 503, map[string]string{"error": "session_revocation_unavailable"})
			return
		}
		if err = tx.Commit(r.Context()); err != nil {
			clearCookie(w, s.cookieSecure)
			jsonOut(w, 503, map[string]string{"error": "session_revocation_unavailable"})
			return
		}
	}
	clearCookie(w, s.cookieSecure)
	jsonOut(w, 200, map[string]string{"status": "signed_out"})
}
func (s *server) workspace(w http.ResponseWriter, r *http.Request) {
	uid := r.Context().Value(userIDKey).(string)
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		jsonOut(w, 500, map[string]string{"error": "workspace_unavailable"})
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `SELECT set_config('app.user_id',$1,true)`, uid); err != nil {
		jsonOut(w, 500, map[string]string{"error": "workspace_unavailable"})
		return
	}
	rows, err := tx.Query(r.Context(), `SELECT business_id,business_name,business_status,membership_role,branch_id,branch_name FROM list_current_user_workspaces()`)
	if err != nil {
		jsonOut(w, 500, map[string]string{"error": "workspace_unavailable"})
		return
	}
	defer rows.Close()
	type branch struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	type business struct {
		ID       string   `json:"id"`
		Name     string   `json:"name"`
		Role     string   `json:"role"`
		Branches []branch `json:"branches"`
	}
	out := []business{}
	indexes := map[string]int{}
	for rows.Next() {
		var bid, bname, bstatus, role, brid, brname string
		if err = rows.Scan(&bid, &bname, &bstatus, &role, &brid, &brname); err != nil {
			jsonOut(w, 500, map[string]string{"error": "workspace_unavailable"})
			return
		}
		if bstatus != "active" {
			continue
		}
		i, ok := indexes[bid]
		if !ok {
			i = len(out)
			indexes[bid] = i
			out = append(out, business{ID: bid, Name: bname, Role: role, Branches: []branch{}})
		}
		out[i].Branches = append(out[i].Branches, branch{ID: brid, Name: brname})
	}
	if rows.Err() != nil {
		jsonOut(w, 500, map[string]string{"error": "workspace_unavailable"})
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		jsonOut(w, 500, map[string]string{"error": "workspace_unavailable"})
		return
	}
	jsonOut(w, 200, map[string]any{"businesses": out})
}

// selectTenant persists a workspace selection against the current session only
// after the database confirms active user, business, membership and branch access.
func (s *server) selectTenant(w http.ResponseWriter, r *http.Request) {
	uid, _ := r.Context().Value(userIDKey).(string)
	sid, _ := r.Context().Value(sessionIDKey).(string)
	var in tenantSelectionRequest
	if !decode(w, r, &in) {
		return
	}
	in.BusinessID = strings.TrimSpace(in.BusinessID)
	in.BranchID = strings.TrimSpace(in.BranchID)
	if in.BusinessID == "" || in.BranchID == "" {
		jsonOut(w, 400, map[string]string{"error": "business_and_branch_selection_required"})
		return
	}
	err := withTenantTx(r.Context(), s.db, uid, in.BusinessID, in.BranchID, func(tx pgx.Tx, _ tenantContext) error {
		_, err := tx.Exec(r.Context(), `UPDATE user_sessions SET selected_business_id=$1, selected_branch_id=$2 WHERE id=$3 AND user_id=$4 AND revoked_at IS NULL AND expires_at>now()`, in.BusinessID, in.BranchID, sid, uid)
		return err
	})
	if errors.Is(err, ErrTenantAccessDenied) {
		jsonOut(w, 403, map[string]string{"error": "business_or_branch_access_denied"})
		return
	}
	if err != nil {
		jsonOut(w, 503, map[string]string{"error": "workspace_selection_unavailable"})
		return
	}
	jsonOut(w, 200, map[string]string{"status": "workspace_selected"})
}

// requireTenant reads the selected workspace from the authenticated session,
// not from client headers. It revalidates membership and account status on every
// request and holds the transaction open for handlers that use its RLS context.
func (s *server) requireTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, _ := r.Context().Value(userIDKey).(string)
		sid, _ := r.Context().Value(sessionIDKey).(string)
		tx, err := s.db.Begin(r.Context())
		if err != nil {
			jsonOut(w, 503, map[string]string{"error": "tenant_context_unavailable"})
			return
		}
		defer tx.Rollback(r.Context())
		if _, err = tx.Exec(r.Context(), `SELECT set_config('app.user_id',$1,true)`, uid); err != nil {
			jsonOut(w, 503, map[string]string{"error": "tenant_context_unavailable"})
			return
		}
		var bid, brid string
		if err = tx.QueryRow(r.Context(), `SELECT selected_business_id, selected_branch_id FROM user_sessions WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL AND expires_at>now()`, sid, uid).Scan(&bid, &brid); err != nil || bid == "" || brid == "" {
			jsonOut(w, 409, map[string]string{"error": "workspace_selection_required"})
			return
		}
		var tc tenantContext
		tc.UserID, tc.BusinessID, tc.BranchID = uid, bid, brid
		if err = tx.QueryRow(r.Context(), `SELECT membership_id, membership_role FROM resolve_branch_membership($1,$2,$3)`, uid, bid, brid).Scan(&tc.MembershipID, &tc.Role); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				jsonOut(w, 403, map[string]string{"error": "business_or_branch_access_denied"})
				return
			}
			jsonOut(w, 503, map[string]string{"error": "tenant_context_unavailable"})
			return
		}
		for key, value := range map[string]string{"app.user_id": uid, "app.business_id": bid, "app.branch_id": brid} {
			if _, err = tx.Exec(r.Context(), `SELECT set_config($1,$2,true)`, key, value); err != nil {
				jsonOut(w, 503, map[string]string{"error": "tenant_context_unavailable"})
				return
			}
		}
		ctx := context.WithValue(r.Context(), tenantContextKey, tc)
		ctx = context.WithValue(ctx, tenantTxKey, tx)
		// Buffer the handler response until the tenant transaction commits. This
		// prevents a client receiving HTTP 2xx for a write that later rolls back.
		recorder := newResponseBuffer()
		next.ServeHTTP(recorder, r.WithContext(ctx))
		if recorder.status >= http.StatusBadRequest {
			_ = tx.Rollback(r.Context())
			recorder.flushTo(w)
			return
		}
		if err = tx.Commit(r.Context()); err != nil {
			s.log.Error("tenant request transaction commit failed", "error", err)
			jsonOut(w, http.StatusServiceUnavailable, map[string]string{"error": "tenant_transaction_commit_failed"})
			return
		}
		recorder.flushTo(w)
	})
}

func (s *server) tenantInfo(w http.ResponseWriter, r *http.Request) {
	tc, ok := r.Context().Value(tenantContextKey).(tenantContext)
	if !ok {
		jsonOut(w, http.StatusInternalServerError, map[string]string{"error": "tenant_context_missing"})
		return
	}
	jsonOut(w, http.StatusOK, map[string]any{
		"user_id": tc.UserID, "business_id": tc.BusinessID, "branch_id": tc.BranchID,
		"membership_id": tc.MembershipID, "role": tc.Role,
		"permissions": map[string]bool{
			"pos.use":                hasPermission(tc.Role, "pos.use"),
			"inventory.read":         hasPermission(tc.Role, "inventory.read"),
			"inventory.adjust":       hasPermission(tc.Role, "inventory.adjust"),
			"inventory.count.create": hasPermission(tc.Role, "inventory.count.create"),
			"reports.read":           hasPermission(tc.Role, "reports.read"),
			"shifts.own":             hasPermission(tc.Role, "shifts.own"),
			"shifts.manage":          hasPermission(tc.Role, "shifts.manage"),
			"expenses.read":          hasPermission(tc.Role, "expenses.read"),
			"expenses.manage":        hasPermission(tc.Role, "expenses.manage"),
			"expenses.approve":       hasPermission(tc.Role, "expenses.approve"),
			"alerts.read":            hasPermission(tc.Role, "alerts.read"),
			"alerts.manage":          hasPermission(tc.Role, "alerts.manage"),
			"audit.read":             hasPermission(tc.Role, "audit.read"),
			"settings.read":          hasPermission(tc.Role, "settings.read"),
			"settings.manage":        hasPermission(tc.Role, "settings.manage"),
			"finance.read":           hasPermission(tc.Role, "finance.read"),
		},
	})
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		jsonOut(w, 400, map[string]string{"error": "invalid_request_body"})
		return false
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		jsonOut(w, 400, map[string]string{"error": "invalid_request_body"})
		return false
	}
	if contentType := r.Header.Get("Content-Type"); !strings.HasPrefix(strings.ToLower(contentType), "application/json") {
		jsonOut(w, 415, map[string]string{"error": "content_type_must_be_application_json"})
		return false
	}
	return true
}
func jsonOut(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func clearCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{Name: "mekuin_session", Value: "", Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0)})
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func validRegistration(in registerRequest) bool {
	return len(in.FullName) >= 2 && len(in.FullName) <= 120 && validEmail(in.Email) && len([]byte(in.Password)) >= 12 && len([]byte(in.Password)) <= 72 && len(in.BusinessName) >= 2 && len(in.BusinessName) <= 160 && len(in.BranchName) >= 2 && len(in.BranchName) <= 120 && len(in.Phone) <= 40
}
func validLogin(email, password string) bool {
	return validEmail(email) && len([]byte(password)) > 0 && len([]byte(password)) <= 72
}
func validEmail(email string) bool {
	if len(email) < 5 || len(email) > 254 || strings.Count(email, "@") != 1 {
		return false
	}
	parts := strings.SplitN(email, "@", 2)
	return len(parts[0]) > 0 && len(parts[1]) >= 3 && strings.Contains(parts[1], ".") && !strings.ContainsAny(email, " \t\r\n")
}

func corsMiddleware(origin string) func(http.Handler) http.Handler {
	allowed, err := url.Parse(origin)
	if err != nil {
		allowed = &url.URL{Scheme: "http", Host: "localhost:3000"}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestOrigin := r.Header.Get("Origin")
			allowedOrigin := allowed.Scheme + "://" + allowed.Host
			if requestOrigin != "" && requestOrigin != allowedOrigin && (r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch || r.Method == http.MethodDelete) {
				jsonOut(w, http.StatusForbidden, map[string]string{"error": "origin_not_allowed"})
				return
			}
			if requestOrigin != "" && requestOrigin == allowedOrigin {
				w.Header().Set("Access-Control-Allow-Origin", requestOrigin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
