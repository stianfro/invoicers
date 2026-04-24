package httpapp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stianfro/invoicers/internal/invoice"
	"github.com/stianfro/invoicers/internal/render"
	"github.com/stianfro/invoicers/internal/store"
)

func TestServerDashboardAndInvoiceAPI(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestStore(t, ctx)
	seedTestSettings(t, ctx, db)

	server := NewServer(Options{
		Store:          db,
		InvoiceService: invoice.NewService(db, nil, nil),
		Renderer:       render.NewRenderer(),
		DataDir:        t.TempDir(),
		Now: func() time.Time {
			return time.Date(2026, time.April, 2, 0, 0, 0, 0, time.UTC)
		},
	})

	dashboardReq := httptest.NewRequest(http.MethodGet, "/", nil)
	dashboardResp := httptest.NewRecorder()
	server.ServeHTTP(dashboardResp, dashboardReq)

	if dashboardResp.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want %d", dashboardResp.Code, http.StatusOK)
	}
	if !strings.Contains(dashboardResp.Body.String(), "Invoices") {
		t.Fatalf("GET / body did not contain %q", "Invoices")
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/invoices", bytes.NewReader([]byte(`{}`)))
	createResp := httptest.NewRecorder()
	server.ServeHTTP(createResp, createReq)

	if createResp.Code != http.StatusCreated {
		t.Fatalf("POST /api/invoices status = %d, want %d", createResp.Code, http.StatusCreated)
	}

	var created invoice.Invoice
	if err := json.Unmarshal(createResp.Body.Bytes(), &created); err != nil {
		t.Fatalf("Unmarshal created invoice returned error: %v", err)
	}
	if created.Number != "INV-001" {
		t.Fatalf("created invoice number = %q, want %q", created.Number, "INV-001")
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/invoices", nil)
	listResp := httptest.NewRecorder()
	server.ServeHTTP(listResp, listReq)

	if listResp.Code != http.StatusOK {
		t.Fatalf("GET /api/invoices status = %d, want %d", listResp.Code, http.StatusOK)
	}
	if !strings.Contains(listResp.Body.String(), "INV-001") {
		t.Fatalf("GET /api/invoices body did not contain %q", "INV-001")
	}
}

func TestServerUpdateProfileAPI(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestStore(t, ctx)
	seedTestSettings(t, ctx, db)

	server := NewServer(Options{
		Store:          db,
		InvoiceService: invoice.NewService(db, nil, nil),
		Renderer:       render.NewRenderer(),
		DataDir:        t.TempDir(),
		Now: func() time.Time {
			return time.Date(2026, time.April, 2, 0, 0, 0, 0, time.UTC)
		},
	})

	body := []byte(`{
		"companyName":"Froystein Consulting",
		"companyAddress":["Tokyo","Japan"],
		"accountName":"Froystein Consulting KK",
		"iban":"BE13 9670 3047 5039",
		"bic":"TRWIBEB1XXX",
		"bankName":"Wise",
		"bankAddress":["Brussels","Belgium"],
		"defaultCurrency":"EUR"
	}`)
	req := httptest.NewRequest(http.MethodPut, "/api/settings/profile", bytes.NewReader(body))
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("PUT /api/settings/profile status = %d, want %d", resp.Code, http.StatusOK)
	}

	profile, err := db.GetProfile(ctx)
	if err != nil {
		t.Fatalf("GetProfile returned error: %v", err)
	}

	if profile.CompanyName != "Froystein Consulting" {
		t.Fatalf("profile company name = %q, want %q", profile.CompanyName, "Froystein Consulting")
	}
}

func TestServerRenderDraftAndMarkSent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestStore(t, ctx)
	seedTestSettings(t, ctx, db)
	dataDir := t.TempDir()

	server := NewServer(Options{
		Store:          db,
		InvoiceService: invoice.NewService(db, nil, nil),
		Renderer:       render.NewRenderer(),
		DataDir:        dataDir,
		Now: func() time.Time {
			return time.Date(2026, time.April, 2, 0, 0, 0, 0, time.UTC)
		},
	})

	createReq := httptest.NewRequest(http.MethodPost, "/api/invoices", bytes.NewReader([]byte(`{}`)))
	createResp := httptest.NewRecorder()
	server.ServeHTTP(createResp, createReq)

	var created invoice.Invoice
	if err := json.Unmarshal(createResp.Body.Bytes(), &created); err != nil {
		t.Fatalf("Unmarshal created invoice returned error: %v", err)
	}

	renderReq := httptest.NewRequest(http.MethodPost, "/api/invoices/1/render", nil)
	renderResp := httptest.NewRecorder()
	server.ServeHTTP(renderResp, renderReq)

	if renderResp.Code != http.StatusOK {
		t.Fatalf("POST /api/invoices/1/render status = %d, want %d", renderResp.Code, http.StatusOK)
	}

	var rendered invoice.Invoice
	if err := json.Unmarshal(renderResp.Body.Bytes(), &rendered); err != nil {
		t.Fatalf("Unmarshal rendered invoice returned error: %v", err)
	}

	if rendered.PDFPath != "/artifacts/INV-001.pdf" {
		t.Fatalf("rendered invoice PDF path = %q, want %q", rendered.PDFPath, "/artifacts/INV-001.pdf")
	}

	pdfBytes, err := os.ReadFile(filepath.Join(dataDir, "artifacts", "INV-001.pdf"))
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.HasPrefix(string(pdfBytes), "%PDF-1.4") {
		t.Fatalf("PDF file prefix = %q, want %q", string(pdfBytes[:8]), "%PDF-1.4")
	}

	draftReq := httptest.NewRequest(http.MethodPost, "/api/invoices/1/generate-email-draft", nil)
	draftResp := httptest.NewRecorder()
	server.ServeHTTP(draftResp, draftReq)

	if draftResp.Code != http.StatusOK {
		t.Fatalf("POST /api/invoices/1/generate-email-draft status = %d, want %d", draftResp.Code, http.StatusOK)
	}
	if !strings.Contains(draftResp.Body.String(), "INV-001") {
		t.Fatalf("draft response did not contain %q", "INV-001")
	}

	markReq := httptest.NewRequest(http.MethodPost, "/api/invoices/1/mark-sent", nil)
	markResp := httptest.NewRecorder()
	server.ServeHTTP(markResp, markReq)

	if markResp.Code != http.StatusOK {
		t.Fatalf("POST /api/invoices/1/mark-sent status = %d, want %d", markResp.Code, http.StatusOK)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/invoices/1", nil)
	getResp := httptest.NewRecorder()
	server.ServeHTTP(getResp, getReq)

	if getResp.Code != http.StatusOK {
		t.Fatalf("GET /api/invoices/1 status = %d, want %d", getResp.Code, http.StatusOK)
	}
	if !strings.Contains(getResp.Body.String(), `"status":"sent"`) {
		t.Fatalf("GET /api/invoices/1 body did not contain sent status")
	}
}

func openTestStore(t *testing.T, ctx context.Context) *store.SQLiteStore {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "invoicers.db")
	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func seedTestSettings(t *testing.T, ctx context.Context, db *store.SQLiteStore) {
	t.Helper()

	if err := db.UpsertProfile(ctx, invoice.Profile{
		CompanyName:     "Froystein Consulting",
		CompanyAddress:  []string{"Tokyo", "Japan"},
		AccountName:     "Froystein Consulting KK",
		IBAN:            "BE13 9670 3047 5039",
		BIC:             "TRWIBEB1XXX",
		BankName:        "Wise",
		BankAddress:     []string{"Brussels", "Belgium"},
		DefaultCurrency: "EUR",
	}); err != nil {
		t.Fatalf("UpsertProfile returned error: %v", err)
	}

	customerID, err := db.UpsertCustomer(ctx, invoice.Customer{
		Name:  "Intility AS",
		Email: "accounts@intility.no",
	})
	if err != nil {
		t.Fatalf("UpsertCustomer returned error: %v", err)
	}

	if _, err := db.UpsertTemplate(ctx, invoice.Template{
		Name:             "Default",
		CustomerID:       customerID,
		EmailRecipient:   "accounts@intility.no",
		EmailSubjectTmpl: "Invoice {{invoice_number}}",
		EmailBodyTmpl:    "Attached {{invoice_number}}",
		PaymentTermsDays: 14,
		DefaultLineItems: []invoice.LineItem{
			{Name: "Consultancy work", Description: "{{month_name}}", Quantity: 1, UnitPriceCents: 740546},
		},
		OnCallEnabled:   true,
		DefaultCurrency: "EUR",
	}); err != nil {
		t.Fatalf("UpsertTemplate returned error: %v", err)
	}
}
