package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stianfro/invoicers/internal/invoice"
)

func TestSQLiteStoreCreateAndListInvoices(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "invoicers.db")

	db, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}

	customerID, err := db.UpsertCustomer(ctx, invoice.Customer{
		Name:  "Intility AS",
		Email: "accounts@intility.no",
	})
	if err != nil {
		t.Fatalf("UpsertCustomer returned error: %v", err)
	}

	templateID, err := db.UpsertTemplate(ctx, invoice.Template{
		Name:              "Default",
		CustomerID:        customerID,
		EmailRecipient:    "accounts@intility.no",
		EmailSubjectTmpl:  "Invoice {{invoice_number}}",
		EmailBodyTmpl:     "Please find attached {{invoice_number}}",
		PaymentTermsDays:  14,
		DefaultLineItems:  []invoice.LineItem{{Name: "Consultancy work", Description: "{{month_name}}", Quantity: 1, UnitPriceCents: 740546}},
		OnCallEnabled:     true,
		DefaultCurrency:   "EUR",
		DefaultCustomerPO: "",
	})
	if err != nil {
		t.Fatalf("UpsertTemplate returned error: %v", err)
	}

	created, err := db.CreateInvoice(ctx, invoice.Invoice{
		Number:       "INV-026",
		InvoiceMonth: time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC),
		IssueDate:    time.Date(2026, time.April, 2, 0, 0, 0, 0, time.UTC),
		DueDate:      time.Date(2026, time.April, 16, 0, 0, 0, 0, time.UTC),
		Status:       invoice.StatusDraft,
		CustomerID:   customerID,
		TemplateID:   templateID,
		LineItems:    []invoice.LineItem{{Name: "Consultancy work", Description: "April", Quantity: 1, UnitPriceCents: 740546}},
		OnCallNOK:    13650,
	})
	if err != nil {
		t.Fatalf("CreateInvoice returned error: %v", err)
	}

	invoices, err := db.ListInvoices(ctx)
	if err != nil {
		t.Fatalf("ListInvoices returned error: %v", err)
	}

	if len(invoices) != 1 {
		t.Fatalf("ListInvoices length = %d, want %d", len(invoices), 1)
	}

	if invoices[0].ID != created.ID {
		t.Fatalf("ListInvoices[0].ID = %d, want %d", invoices[0].ID, created.ID)
	}

	if invoices[0].Number != "INV-026" {
		t.Fatalf("ListInvoices[0].Number = %q, want %q", invoices[0].Number, "INV-026")
	}
}

func TestSQLiteStoreLookupMethods(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "invoicers.db")

	db, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}

	customerID, err := db.UpsertCustomer(ctx, invoice.Customer{
		Name:  "Intility AS",
		Email: "accounts@intility.no",
	})
	if err != nil {
		t.Fatalf("UpsertCustomer returned error: %v", err)
	}

	templateID, err := db.UpsertTemplate(ctx, invoice.Template{
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
	})
	if err != nil {
		t.Fatalf("UpsertTemplate returned error: %v", err)
	}

	created, err := db.CreateInvoice(ctx, invoice.Invoice{
		Number:         "INV-026",
		InvoiceMonth:   time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC),
		IssueDate:      time.Date(2026, time.April, 2, 0, 0, 0, 0, time.UTC),
		DueDate:        time.Date(2026, time.April, 16, 0, 0, 0, 0, time.UTC),
		Status:         invoice.StatusDraft,
		CustomerID:     customerID,
		TemplateID:     templateID,
		EmailRecipient: "accounts@intility.no",
		LineItems: []invoice.LineItem{
			{Name: "Consultancy work", Description: "April", Quantity: 1, UnitPriceCents: 740546},
		},
		OnCallNOK: 13650,
		FXSnapshot: &invoice.FXSnapshot{
			Rate:       11.5,
			Currency:   "NOK",
			Base:       "EUR",
			ObservedAt: time.Date(2026, time.April, 15, 0, 0, 0, 0, time.UTC),
			Source:     "test",
		},
		PDFPath: "/data/artifacts/INV-026.pdf",
	})
	if err != nil {
		t.Fatalf("CreateInvoice returned error: %v", err)
	}

	latest, err := db.LatestInvoice(ctx)
	if err != nil {
		t.Fatalf("LatestInvoice returned error: %v", err)
	}

	if latest.Number != "INV-026" {
		t.Fatalf("LatestInvoice number = %q, want %q", latest.Number, "INV-026")
	}

	tmpl, err := db.DefaultTemplate(ctx)
	if err != nil {
		t.Fatalf("DefaultTemplate returned error: %v", err)
	}

	if tmpl.ID != templateID {
		t.Fatalf("DefaultTemplate id = %d, want %d", tmpl.ID, templateID)
	}

	loaded, err := db.GetInvoice(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetInvoice returned error: %v", err)
	}

	if len(loaded.LineItems) != 1 {
		t.Fatalf("GetInvoice line items = %d, want %d", len(loaded.LineItems), 1)
	}

	draft, err := db.SaveEmailDraft(ctx, invoice.EmailDraft{
		InvoiceID:      created.ID,
		Recipient:      "accounts@intility.no",
		Subject:        "Invoice INV-026",
		Body:           "Attached",
		AttachmentPath: "/data/artifacts/INV-026.pdf",
	})
	if err != nil {
		t.Fatalf("SaveEmailDraft returned error: %v", err)
	}

	if draft.ID == 0 {
		t.Fatalf("SaveEmailDraft id = %d, want non-zero id", draft.ID)
	}
}

func TestSQLiteStoreProfileRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "invoicers.db")

	db, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}

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

	profile, err := db.GetProfile(ctx)
	if err != nil {
		t.Fatalf("GetProfile returned error: %v", err)
	}

	if profile.CompanyName != "Froystein Consulting" {
		t.Fatalf("GetProfile company name = %q, want %q", profile.CompanyName, "Froystein Consulting")
	}
}

func TestSQLiteStoreUpdateInvoice(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "invoicers.db")

	db, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}

	customerID, err := db.UpsertCustomer(ctx, invoice.Customer{Name: "Intility AS", Email: "accounts@intility.no"})
	if err != nil {
		t.Fatalf("UpsertCustomer returned error: %v", err)
	}

	templateID, err := db.UpsertTemplate(ctx, invoice.Template{
		Name:             "Default",
		CustomerID:       customerID,
		EmailRecipient:   "accounts@intility.no",
		EmailSubjectTmpl: "Invoice {{invoice_number}}",
		EmailBodyTmpl:    "Attached {{invoice_number}}",
		PaymentTermsDays: 14,
		DefaultLineItems: []invoice.LineItem{{Name: "Consultancy work", Description: "{{month_name}}", Quantity: 1, UnitPriceCents: 740546}},
		OnCallEnabled:    true,
		DefaultCurrency:  "EUR",
	})
	if err != nil {
		t.Fatalf("UpsertTemplate returned error: %v", err)
	}

	created, err := db.CreateInvoice(ctx, invoice.Invoice{
		Number:       "INV-026",
		InvoiceMonth: time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC),
		IssueDate:    time.Date(2026, time.April, 2, 0, 0, 0, 0, time.UTC),
		DueDate:      time.Date(2026, time.April, 16, 0, 0, 0, 0, time.UTC),
		Status:       invoice.StatusDraft,
		CustomerID:   customerID,
		TemplateID:   templateID,
		LineItems:    []invoice.LineItem{{Name: "Consultancy work", Description: "April", Quantity: 1, UnitPriceCents: 740546}},
	})
	if err != nil {
		t.Fatalf("CreateInvoice returned error: %v", err)
	}

	created.LineItems = []invoice.LineItem{{Name: "Consultancy work", Description: "April updated", Quantity: 1, UnitPriceCents: 800000}}
	created.OnCallNOK = 15000

	if err := db.UpdateInvoice(ctx, created); err != nil {
		t.Fatalf("UpdateInvoice returned error: %v", err)
	}

	if err := db.UpdateInvoiceArtifacts(ctx, created.ID, "/artifacts/INV-026.html", "/artifacts/INV-026.pdf", &invoice.FXSnapshot{
		Rate:       11.5,
		Currency:   "NOK",
		Base:       "EUR",
		ObservedAt: time.Date(2026, time.April, 15, 0, 0, 0, 0, time.UTC),
		Source:     "test",
	}); err != nil {
		t.Fatalf("UpdateInvoiceArtifacts returned error: %v", err)
	}

	if err := db.UpdateInvoiceStatus(ctx, created.ID, invoice.StatusSent); err != nil {
		t.Fatalf("UpdateInvoiceStatus returned error: %v", err)
	}

	loaded, err := db.GetInvoice(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetInvoice returned error: %v", err)
	}

	if loaded.Status != invoice.StatusSent {
		t.Fatalf("GetInvoice status = %q, want %q", loaded.Status, invoice.StatusSent)
	}
	if loaded.LineItems[0].Description != "April updated" {
		t.Fatalf("GetInvoice line description = %q, want %q", loaded.LineItems[0].Description, "April updated")
	}
	if loaded.PDFPath != "/artifacts/INV-026.pdf" {
		t.Fatalf("GetInvoice pdf path = %q, want %q", loaded.PDFPath, "/artifacts/INV-026.pdf")
	}
}
