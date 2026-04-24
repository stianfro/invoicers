package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stianfro/invoicers/internal/invoice"
)

func TestRendererRenderHTML(t *testing.T) {
	t.Parallel()

	renderer := NewRenderer()
	doc := Document{
		Profile: invoice.Profile{
			CompanyName:    "Froystein Consulting",
			CompanyAddress: []string{"Tokyo", "Japan"},
			AccountName:    "Froystein Consulting KK",
			IBAN:           "BE13 9670 3047 5039",
			BIC:            "TRWIBEB1XXX",
			BankName:       "Wise",
			BankAddress:    []string{"Brussels", "Belgium"},
		},
		Customer: invoice.Customer{
			Name: "Intility AS",
		},
		Invoice: invoice.Invoice{
			Number:           "INV-026",
			InvoiceMonth:     time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC),
			IssueDate:        time.Date(2026, time.April, 2, 0, 0, 0, 0, time.UTC),
			DueDate:          time.Date(2026, time.April, 16, 0, 0, 0, 0, time.UTC),
			LineItems:        []invoice.LineItem{{Name: "Consultancy work", Description: "April", Quantity: 1, UnitPriceCents: 740546}},
			OnCallNOK:        13650,
			FXSnapshot:       &invoice.FXSnapshot{Rate: 11.5, Currency: "NOK", Base: "EUR"},
			TotalAmountCents: 837937,
		},
	}

	html, err := renderer.RenderHTML(doc)
	if err != nil {
		t.Fatalf("RenderHTML returned error: %v", err)
	}

	for _, want := range []string{"INV-026", "Froystein Consulting", "Intility AS", "Consultancy work", "Amount due"} {
		if !strings.Contains(html, want) {
			t.Fatalf("RenderHTML output did not contain %q", want)
		}
	}
}

func TestRendererWritePDF(t *testing.T) {
	t.Parallel()

	renderer := NewRenderer()
	doc := Document{
		Profile:  invoice.Profile{CompanyName: "Froystein Consulting"},
		Customer: invoice.Customer{Name: "Intility AS"},
		Invoice: invoice.Invoice{
			Number:           "INV-026",
			InvoiceMonth:     time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC),
			IssueDate:        time.Date(2026, time.April, 2, 0, 0, 0, 0, time.UTC),
			DueDate:          time.Date(2026, time.April, 16, 0, 0, 0, 0, time.UTC),
			TotalAmountCents: 837937,
		},
	}

	pdfPath := filepath.Join(t.TempDir(), "invoice.pdf")
	if err := renderer.WritePDF(pdfPath, doc); err != nil {
		t.Fatalf("WritePDF returned error: %v", err)
	}

	pdfBytes, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}

	if !strings.HasPrefix(string(pdfBytes), "%PDF-1.4") {
		t.Fatalf("PDF output prefix = %q, want %q", string(pdfBytes[:8]), "%PDF-1.4")
	}
}
