package invoice

import (
	"context"
	"testing"
	"time"
)

func TestFormatMoneyEUR(t *testing.T) {
	t.Parallel()

	got := FormatMoneyEUR(863542)
	if got != "8635.42 EUR" {
		t.Fatalf("FormatMoneyEUR = %q, want %q", got, "8635.42 EUR")
	}
}

func TestServiceGenerateEmailDraftPersistsDraft(t *testing.T) {
	t.Parallel()

	repo := &fakeRepository{
		defaultTemplate: Template{
			ID:               5,
			EmailRecipient:   "accounts@intility.no",
			EmailSubjectTmpl: "Invoice {{invoice_number}}",
			EmailBodyTmpl:    "Total {{total_amount}}",
		},
		invoiceByID: map[int64]Invoice{
			4: {
				ID:               4,
				Number:           "INV-027",
				InvoiceMonth:     time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC),
				TemplateID:       5,
				TotalAmountCents: 740546,
				PDFPath:          "/data/artifacts/INV-027.pdf",
			},
		},
	}

	service := NewService(repo, nil, nil)
	draft, err := service.GenerateEmailDraft(context.Background(), 4)
	if err != nil {
		t.Fatalf("GenerateEmailDraft returned error: %v", err)
	}

	if repo.savedEmailDraft.InvoiceID != 4 {
		t.Fatalf("saved draft invoice id = %d, want %d", repo.savedEmailDraft.InvoiceID, 4)
	}

	if draft.ID == 0 {
		t.Fatalf("draft id = %d, want non-zero persisted ID", draft.ID)
	}
}
