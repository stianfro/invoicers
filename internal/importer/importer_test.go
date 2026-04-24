package importer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stianfro/invoicers/internal/store"
)

func TestImporterImport(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sourceDir := t.TempDir()
	dataDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(sourceDir, "config.yaml"), []byte(`companyName: Froystein Consulting
companyAddress:
  - Tokyo
  - Japan
iban: BE13 9670 3047 5039
bic: TRWIBEB1XXX
accountName: Froystein Consulting KK
bankName: Wise
bankAddress:
  - Brussels
  - Belgium
`), 0o644); err != nil {
		t.Fatalf("WriteFile config returned error: %v", err)
	}

	monthDir := filepath.Join(sourceDir, "2603_March")
	if err := os.MkdirAll(monthDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}

	if err := os.WriteFile(filepath.Join(monthDir, "INV-025.yaml"), []byte(`name: INV-025
customerName: Intility AS
services:
  - name: Consultancy work
    description: March
    quantity: 1
    price: 7405.46
onCallNOK: 13650
invoiceMonth: "March"
`), 0o644); err != nil {
		t.Fatalf("WriteFile invoice yaml returned error: %v", err)
	}

	if err := os.WriteFile(filepath.Join(monthDir, "INV-025.html"), []byte("<html>INV-025</html>"), 0o644); err != nil {
		t.Fatalf("WriteFile html returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(monthDir, "INV-025.pdf"), []byte("%PDF-1.4 test"), 0o644); err != nil {
		t.Fatalf("WriteFile pdf returned error: %v", err)
	}

	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "invoicers.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}

	importer := New(db, dataDir, nil)
	result, err := importer.Import(ctx, sourceDir)
	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}

	if result.ImportedInvoices != 1 {
		t.Fatalf("ImportedInvoices = %d, want %d", result.ImportedInvoices, 1)
	}

	profile, err := db.GetProfile(ctx)
	if err != nil {
		t.Fatalf("GetProfile returned error: %v", err)
	}
	if profile.CompanyName != "Froystein Consulting" {
		t.Fatalf("GetProfile company name = %q, want %q", profile.CompanyName, "Froystein Consulting")
	}

	invoices, err := db.ListInvoices(ctx)
	if err != nil {
		t.Fatalf("ListInvoices returned error: %v", err)
	}
	if len(invoices) != 1 {
		t.Fatalf("ListInvoices length = %d, want %d", len(invoices), 1)
	}
	if invoices[0].Number != "INV-025" {
		t.Fatalf("ListInvoices[0].Number = %q, want %q", invoices[0].Number, "INV-025")
	}

	if _, err := os.Stat(filepath.Join(dataDir, "artifacts", "imported", "INV-025.pdf")); err != nil {
		t.Fatalf("Stat imported pdf returned error: %v", err)
	}
}
