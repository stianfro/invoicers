package invoice

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNextInvoiceNumber(t *testing.T) {
	t.Parallel()

	got, err := NextInvoiceNumber("INV-025")
	if err != nil {
		t.Fatalf("NextInvoiceNumber returned error: %v", err)
	}

	if got != "INV-026" {
		t.Fatalf("NextInvoiceNumber = %q, want %q", got, "INV-026")
	}
}

func TestCalculateOnCallEURCents(t *testing.T) {
	t.Parallel()

	got := CalculateOnCallEURCents(10000, 11.5)
	if got != 97391 {
		t.Fatalf("CalculateOnCallEURCents = %d, want %d", got, 97391)
	}
}

func TestFillTemplatePlaceholders(t *testing.T) {
	t.Parallel()

	month := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	got := FillTemplatePlaceholders("Consultancy work for {{month_name}} {{year}}", month)

	if got != "Consultancy work for March 2026" {
		t.Fatalf("FillTemplatePlaceholders = %q, want %q", got, "Consultancy work for March 2026")
	}
}

func TestServiceCreateNextInvoice(t *testing.T) {
	t.Parallel()

	repo := &fakeRepository{
		latestInvoice: Invoice{
			ID:           10,
			Number:       "INV-025",
			InvoiceMonth: time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
		},
		defaultTemplate: Template{
			ID:               3,
			Name:             "Default",
			CustomerID:       2,
			EmailRecipient:   "accounts@intility.no",
			EmailSubjectTmpl: "Invoice {{invoice_number}}",
			EmailBodyTmpl:    "Attached is {{invoice_number}} for {{month_name}} {{year}}.",
			PaymentTermsDays: 14,
			DefaultLineItems: []LineItem{
				{Name: "Consultancy work", Description: "{{month_name}}", Quantity: 1, UnitPriceCents: 740546},
			},
			OnCallEnabled:   true,
			DefaultCurrency: "EUR",
		},
	}

	service := NewService(repo, nil, nil)
	now := time.Date(2026, time.April, 2, 0, 0, 0, 0, time.UTC)

	created, err := service.CreateNextInvoice(context.Background(), now)
	if err != nil {
		t.Fatalf("CreateNextInvoice returned error: %v", err)
	}

	if created.Number != "INV-026" {
		t.Fatalf("CreateNextInvoice number = %q, want %q", created.Number, "INV-026")
	}

	if created.InvoiceMonth.Format("2006-01") != "2026-04" {
		t.Fatalf("CreateNextInvoice invoice month = %q, want %q", created.InvoiceMonth.Format("2006-01"), "2026-04")
	}

	if len(created.LineItems) != 1 || created.LineItems[0].Description != "April" {
		t.Fatalf("CreateNextInvoice line items = %+v, want April-filled template line item", created.LineItems)
	}

	if created.DueDate.Format(time.DateOnly) != "2026-04-16" {
		t.Fatalf("CreateNextInvoice due date = %q, want %q", created.DueDate.Format(time.DateOnly), "2026-04-16")
	}
}

func TestServiceGenerateEmailDraft(t *testing.T) {
	t.Parallel()

	repo := &fakeRepository{
		defaultTemplate: Template{
			ID:               3,
			Name:             "Default",
			CustomerID:       2,
			EmailRecipient:   "accounts@intility.no",
			EmailSubjectTmpl: "Invoice {{invoice_number}} for {{month_name}} {{year}}",
			EmailBodyTmpl:    "Hi,\n\nAttached is invoice {{invoice_number}} totaling {{total_amount}}.\n",
		},
		invoiceByID: map[int64]Invoice{
			42: {
				ID:               42,
				Number:           "INV-026",
				InvoiceMonth:     time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC),
				TemplateID:       3,
				TotalAmountCents: 863542,
				PDFPath:          "/data/artifacts/INV-026.pdf",
			},
		},
	}

	service := NewService(repo, nil, nil)

	draft, err := service.GenerateEmailDraft(context.Background(), 42)
	if err != nil {
		t.Fatalf("GenerateEmailDraft returned error: %v", err)
	}

	if draft.Subject != "Invoice INV-026 for April 2026" {
		t.Fatalf("GenerateEmailDraft subject = %q, want %q", draft.Subject, "Invoice INV-026 for April 2026")
	}

	if draft.AttachmentPath != "/data/artifacts/INV-026.pdf" {
		t.Fatalf("GenerateEmailDraft attachment = %q, want %q", draft.AttachmentPath, "/data/artifacts/INV-026.pdf")
	}
}

type fakeRepository struct {
	latestInvoice   Invoice
	defaultTemplate Template
	createdInvoice  Invoice
	invoiceByID     map[int64]Invoice
	savedEmailDraft EmailDraft
}

func (f *fakeRepository) LatestInvoice(context.Context) (Invoice, error) {
	if f.latestInvoice.Number == "" {
		return Invoice{}, errors.New("latest invoice not found")
	}
	return f.latestInvoice, nil
}

func (f *fakeRepository) DefaultTemplate(context.Context) (Template, error) {
	if f.defaultTemplate.ID == 0 {
		return Template{}, errors.New("template not found")
	}
	return f.defaultTemplate, nil
}

func (f *fakeRepository) GetTemplate(_ context.Context, id int64) (Template, error) {
	if f.defaultTemplate.ID != id {
		return Template{}, errors.New("template not found")
	}
	return f.defaultTemplate, nil
}

func (f *fakeRepository) CreateInvoice(_ context.Context, inv Invoice) (Invoice, error) {
	inv.ID = 99
	f.createdInvoice = inv
	return inv, nil
}

func (f *fakeRepository) GetInvoice(_ context.Context, id int64) (Invoice, error) {
	inv, ok := f.invoiceByID[id]
	if !ok {
		return Invoice{}, errors.New("invoice not found")
	}
	return inv, nil
}

func (f *fakeRepository) SaveEmailDraft(_ context.Context, draft EmailDraft) (EmailDraft, error) {
	draft.ID = 7
	draft.CreatedAt = time.Date(2026, time.April, 2, 0, 0, 0, 0, time.UTC)
	f.savedEmailDraft = draft
	return draft, nil
}
