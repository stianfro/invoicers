package httpapp

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/stianfro/invoicers/internal/fx"
	"github.com/stianfro/invoicers/internal/invoice"
	"github.com/stianfro/invoicers/internal/render"
	"github.com/stianfro/invoicers/web"
)

type Store interface {
	invoice.Repository
	ListInvoices(ctx context.Context) ([]invoice.Invoice, error)
	GetProfile(ctx context.Context) (invoice.Profile, error)
	UpsertProfile(ctx context.Context, profile invoice.Profile) error
	GetInvoice(ctx context.Context, id int64) (invoice.Invoice, error)
	GetCustomer(ctx context.Context, id int64) (invoice.Customer, error)
	UpdateInvoice(ctx context.Context, inv invoice.Invoice) error
	UpdateInvoiceArtifacts(ctx context.Context, invoiceID int64, htmlPath string, pdfPath string, fx *invoice.FXSnapshot) error
	UpdateInvoiceStatus(ctx context.Context, invoiceID int64, status invoice.Status) error
	ListTemplates(ctx context.Context) ([]invoice.Template, error)
	UpsertTemplate(ctx context.Context, tmpl invoice.Template) (int64, error)
}

type Options struct {
	Store          Store
	InvoiceService *invoice.Service
	Renderer       *render.Renderer
	FXClient       *fx.Client
	DataDir        string
	Now            func() time.Time
}

type Server struct {
	store          Store
	invoiceService *invoice.Service
	renderer       *render.Renderer
	fxClient       *fx.Client
	dataDir        string
	now            func() time.Time
	mux            *http.ServeMux
	dashboardTmpl  *template.Template
	settingsTmpl   *template.Template
	invoiceTmpl    *template.Template
}

func NewServer(opts Options) *Server {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	dataDir := opts.DataDir
	if dataDir == "" {
		dataDir = "data"
	}

	server := &Server{
		store:          opts.Store,
		invoiceService: opts.InvoiceService,
		renderer:       opts.Renderer,
		fxClient:       opts.FXClient,
		dataDir:        dataDir,
		now:            now,
		dashboardTmpl: template.Must(template.New("dashboard.html").Funcs(template.FuncMap{
			"money": invoice.FormatMoneyEUR,
		}).ParseFS(web.Templates, "templates/dashboard.html")),
		settingsTmpl: template.Must(template.New("settings.html").Funcs(template.FuncMap{
			"joinLines": func(lines []string) string {
				return strings.Join(lines, "\n")
			},
			"moneyInput": func(cents int64) string {
				return fmt.Sprintf("%.2f", float64(cents)/100)
			},
		}).ParseFS(web.Templates, "templates/settings.html")),
		invoiceTmpl: template.Must(template.New("invoice_detail.html").Funcs(template.FuncMap{
			"moneyInput": func(cents int64) string {
				return fmt.Sprintf("%.2f", float64(cents)/100)
			},
		}).ParseFS(web.Templates, "templates/invoice_detail.html")),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", server.handleDashboard)
	mux.HandleFunc("GET /settings", server.handleSettingsPage)
	mux.HandleFunc("POST /settings/profile", server.handleUpdateProfileHTML)
	mux.HandleFunc("POST /settings/template", server.handleUpdateTemplateHTML)
	mux.HandleFunc("POST /invoices/create-next", server.handleCreateNextInvoiceHTML)
	mux.HandleFunc("GET /invoices/{id}", server.handleInvoiceDetail)
	mux.HandleFunc("POST /invoices/{id}/update", server.handleUpdateInvoiceHTML)
	mux.HandleFunc("POST /invoices/{id}/render", server.handleRenderInvoiceHTML)
	mux.HandleFunc("POST /invoices/{id}/generate-email-draft", server.handleGenerateDraftHTML)
	mux.HandleFunc("POST /invoices/{id}/mark-sent", server.handleMarkSentHTML)
	mux.HandleFunc("GET /history", server.handleDashboard)
	mux.HandleFunc("GET /api/invoices", server.handleListInvoicesAPI)
	mux.HandleFunc("POST /api/invoices", server.handleCreateInvoiceAPI)
	mux.HandleFunc("GET /api/invoices/{id}", server.handleGetInvoiceAPI)
	mux.HandleFunc("POST /api/invoices/{id}/render", server.handleRenderInvoiceAPI)
	mux.HandleFunc("POST /api/invoices/{id}/generate-email-draft", server.handleGenerateDraftAPI)
	mux.HandleFunc("POST /api/invoices/{id}/mark-sent", server.handleMarkSentAPI)
	mux.HandleFunc("GET /api/templates", server.handleListTemplatesAPI)
	mux.HandleFunc("PUT /api/settings/profile", server.handleUpdateProfileAPI)
	mux.Handle("GET /artifacts/", http.StripPrefix("/artifacts/", http.FileServer(http.Dir(filepath.Join(dataDir, "artifacts")))))
	server.mux = mux
	return server
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	invoices, err := s.store.ListInvoices(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("list invoices: %v", err), http.StatusInternalServerError)
		return
	}

	data := struct {
		Invoices []invoice.Invoice
	}{
		Invoices: invoices,
	}
	if err := s.dashboardTmpl.Execute(w, data); err != nil {
		http.Error(w, fmt.Sprintf("render dashboard: %v", err), http.StatusInternalServerError)
		return
	}
}

func (s *Server) handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	profile, err := s.store.GetProfile(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("load profile: %v", err), http.StatusInternalServerError)
		return
	}
	tmpl, err := s.store.DefaultTemplate(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("load template: %v", err), http.StatusInternalServerError)
		return
	}

	data := struct {
		Profile  invoice.Profile
		Template invoice.Template
	}{
		Profile:  profile,
		Template: tmpl,
	}
	if err := s.settingsTmpl.Execute(w, data); err != nil {
		http.Error(w, fmt.Sprintf("render settings: %v", err), http.StatusInternalServerError)
		return
	}
}

func (s *Server) handleCreateNextInvoiceHTML(w http.ResponseWriter, r *http.Request) {
	created, err := s.invoiceService.CreateNextInvoice(r.Context(), s.now())
	if err != nil {
		http.Error(w, fmt.Sprintf("create invoice: %v", err), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/invoices/%d", created.ID), http.StatusSeeOther)
}

func (s *Server) handleInvoiceDetail(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	inv, err := s.store.GetInvoice(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	customer, err := s.store.GetCustomer(r.Context(), inv.CustomerID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := struct {
		Invoice  invoice.Invoice
		Customer invoice.Customer
	}{
		Invoice:  inv,
		Customer: customer,
	}
	if err := s.invoiceTmpl.Execute(w, data); err != nil {
		http.Error(w, fmt.Sprintf("render invoice detail: %v", err), http.StatusInternalServerError)
		return
	}
}

func (s *Server) handleListInvoicesAPI(w http.ResponseWriter, r *http.Request) {
	invoices, err := s.store.ListInvoices(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, invoices)
}

func (s *Server) handleUpdateProfileHTML(w http.ResponseWriter, r *http.Request) {
	profile := invoice.Profile{
		CompanyName:     r.FormValue("companyName"),
		CompanyAddress:  splitLines(r.FormValue("companyAddress")),
		AccountName:     r.FormValue("accountName"),
		IBAN:            r.FormValue("iban"),
		BIC:             r.FormValue("bic"),
		BankName:        r.FormValue("bankName"),
		BankAddress:     splitLines(r.FormValue("bankAddress")),
		DefaultCurrency: "EUR",
	}
	if err := s.store.UpsertProfile(r.Context(), profile); err != nil {
		http.Error(w, fmt.Sprintf("save profile: %v", err), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (s *Server) handleUpdateTemplateHTML(w http.ResponseWriter, r *http.Request) {
	customerID, err := parseOptionalInt64(r.FormValue("customerId"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	paymentTerms, err := parseOptionalInt(r.FormValue("paymentTermsDays"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	quantity, err := parseOptionalInt(r.FormValue("lineItemQuantity"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	unitPriceCents, err := parseMoneyToCents(r.FormValue("lineItemUnitPrice"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := s.store.UpsertTemplate(r.Context(), invoice.Template{
		Name:             r.FormValue("name"),
		CustomerID:       customerID,
		EmailRecipient:   r.FormValue("emailRecipient"),
		EmailSubjectTmpl: r.FormValue("emailSubjectTemplate"),
		EmailBodyTmpl:    r.FormValue("emailBodyTemplate"),
		PaymentTermsDays: paymentTerms,
		DefaultLineItems: []invoice.LineItem{{
			Name:           r.FormValue("lineItemName"),
			Description:    r.FormValue("lineItemDescription"),
			Quantity:       quantity,
			UnitPriceCents: unitPriceCents,
		}},
		OnCallEnabled:   r.FormValue("onCallEnabled") == "on",
		DefaultCurrency: "EUR",
	}); err != nil {
		http.Error(w, fmt.Sprintf("save template: %v", err), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (s *Server) handleUpdateInvoiceHTML(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	inv, err := s.store.GetInvoice(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	issueDate, err := time.Parse(time.DateOnly, r.FormValue("issueDate"))
	if err != nil {
		http.Error(w, fmt.Sprintf("parse issue date: %v", err), http.StatusBadRequest)
		return
	}
	dueDate, err := time.Parse(time.DateOnly, r.FormValue("dueDate"))
	if err != nil {
		http.Error(w, fmt.Sprintf("parse due date: %v", err), http.StatusBadRequest)
		return
	}
	onCallNok, err := parseOptionalInt(r.FormValue("onCallNok"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	lineCount, err := parseOptionalInt(r.FormValue("lineCount"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	lines := make([]invoice.LineItem, 0, lineCount)
	for i := 0; i < lineCount; i++ {
		quantity, err := parseOptionalInt(r.FormValue(fmt.Sprintf("lineQuantity%d", i)))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		unitPriceCents, err := parseMoneyToCents(r.FormValue(fmt.Sprintf("lineUnitPrice%d", i)))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		lines = append(lines, invoice.LineItem{
			Name:           r.FormValue(fmt.Sprintf("lineName%d", i)),
			Description:    r.FormValue(fmt.Sprintf("lineDescription%d", i)),
			Quantity:       quantity,
			UnitPriceCents: unitPriceCents,
		})
	}
	inv.IssueDate = issueDate
	inv.DueDate = dueDate
	inv.OnCallNOK = onCallNok
	inv.LineItems = lines
	inv.TotalAmountCents = invoice.ComputeInvoiceTotalCents(inv.LineItems, inv.FXSnapshot, inv.OnCallNOK)
	if err := s.store.UpdateInvoice(r.Context(), inv); err != nil {
		http.Error(w, fmt.Sprintf("save invoice: %v", err), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/invoices/%d", inv.ID), http.StatusSeeOther)
}

func (s *Server) handleCreateInvoiceAPI(w http.ResponseWriter, r *http.Request) {
	created, err := s.invoiceService.CreateNextInvoice(r.Context(), s.now())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleGetInvoiceAPI(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err)
		return
	}
	inv, err := s.store.GetInvoice(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, inv)
}

func (s *Server) handleRenderInvoiceAPI(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err)
		return
	}
	updated, err := s.renderInvoice(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleGenerateDraftAPI(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err)
		return
	}
	draft, err := s.invoiceService.GenerateEmailDraft(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, draft)
}

func (s *Server) handleMarkSentAPI(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.UpdateInvoiceStatus(r.Context(), id, invoice.StatusSent); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	updated, err := s.store.GetInvoice(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleRenderInvoiceHTML(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := s.renderInvoice(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/invoices/%d", id), http.StatusSeeOther)
}

func (s *Server) handleGenerateDraftHTML(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := s.invoiceService.GenerateEmailDraft(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/invoices/%d", id), http.StatusSeeOther)
}

func (s *Server) handleMarkSentHTML(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.store.UpdateInvoiceStatus(r.Context(), id, invoice.StatusSent); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/invoices/%d", id), http.StatusSeeOther)
}

func (s *Server) handleListTemplatesAPI(w http.ResponseWriter, r *http.Request) {
	templates, err := s.store.ListTemplates(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, templates)
}

func (s *Server) handleUpdateProfileAPI(w http.ResponseWriter, r *http.Request) {
	var profile invoice.Profile
	if err := json.NewDecoder(r.Body).Decode(&profile); err != nil {
		writeJSONError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.UpsertProfile(r.Context(), profile); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func parseID(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse id %q: %w", raw, err)
	}
	return id, nil
}

func parseOptionalInt(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("parse integer %q: %w", raw, err)
	}
	return value, nil
}

func parseOptionalInt64(raw string) (int64, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse integer %q: %w", raw, err)
	}
	return value, nil
}

func parseMoneyToCents(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("parse money %q: %w", raw, err)
	}
	return int64(value*100.0 + 0.5), nil
}

func splitLines(raw string) []string {
	lines := strings.Split(raw, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func (s *Server) renderInvoice(ctx context.Context, id int64) (invoice.Invoice, error) {
	if s.renderer == nil {
		return invoice.Invoice{}, fmt.Errorf("renderer is not configured")
	}

	inv, err := s.store.GetInvoice(ctx, id)
	if err != nil {
		return invoice.Invoice{}, err
	}

	if inv.FXSnapshot == nil && inv.OnCallNOK > 0 && s.fxClient != nil {
		inv.FXSnapshot, err = s.fxClient.RateForMonth(ctx, inv.InvoiceMonth)
		if err != nil {
			return invoice.Invoice{}, err
		}
	}

	inv.TotalAmountCents = invoice.ComputeInvoiceTotalCents(inv.LineItems, inv.FXSnapshot, inv.OnCallNOK)
	if err := s.store.UpdateInvoice(ctx, inv); err != nil {
		return invoice.Invoice{}, err
	}

	profile, err := s.store.GetProfile(ctx)
	if err != nil {
		return invoice.Invoice{}, err
	}
	customer, err := s.store.GetCustomer(ctx, inv.CustomerID)
	if err != nil {
		return invoice.Invoice{}, err
	}

	doc := render.Document{
		Profile:  profile,
		Customer: customer,
		Invoice:  inv,
	}
	htmlString, err := s.renderer.RenderHTML(doc)
	if err != nil {
		return invoice.Invoice{}, err
	}

	artifactDir := filepath.Join(s.dataDir, "artifacts")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return invoice.Invoice{}, err
	}

	htmlPath := filepath.Join(artifactDir, inv.Number+".html")
	pdfPath := filepath.Join(artifactDir, inv.Number+".pdf")
	if err := os.WriteFile(htmlPath, []byte(htmlString), 0o644); err != nil {
		return invoice.Invoice{}, err
	}
	if err := s.renderer.WritePDF(pdfPath, doc); err != nil {
		return invoice.Invoice{}, err
	}

	if err := s.store.UpdateInvoiceArtifacts(ctx, inv.ID, "/artifacts/"+inv.Number+".html", "/artifacts/"+inv.Number+".pdf", inv.FXSnapshot); err != nil {
		return invoice.Invoice{}, err
	}

	return s.store.GetInvoice(ctx, inv.ID)
}
