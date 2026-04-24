package render

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/stianfro/invoicers/internal/invoice"
	"github.com/stianfro/invoicers/web"
)

type Document struct {
	Profile  invoice.Profile
	Customer invoice.Customer
	Invoice  invoice.Invoice
}

type Renderer struct {
	documentTemplate *template.Template
}

type documentRow struct {
	Name           string
	Description    string
	Quantity       int
	UnitPriceCents int64
	TotalCents     int64
}

func NewRenderer() *Renderer {
	tmpl := template.Must(template.New("invoice_document.html").Funcs(template.FuncMap{
		"money": func(cents int64) string {
			return invoice.FormatMoneyEUR(cents)
		},
		"date": func(t time.Time) string {
			return t.Format("2006-01-02")
		},
	}).ParseFS(web.Templates, "templates/invoice_document.html"))

	return &Renderer{documentTemplate: tmpl}
}

func (r *Renderer) RenderHTML(doc Document) (string, error) {
	view := struct {
		Profile  invoice.Profile
		Customer invoice.Customer
		Invoice  invoice.Invoice
		Rows     []documentRow
	}{
		Profile:  doc.Profile,
		Customer: doc.Customer,
		Invoice:  doc.Invoice,
		Rows:     buildRows(doc.Invoice),
	}

	var buf bytes.Buffer
	if err := r.documentTemplate.Execute(&buf, view); err != nil {
		return "", fmt.Errorf("render html template: %w", err)
	}

	return buf.String(), nil
}

func (r *Renderer) WritePDF(path string, doc Document) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create pdf directory: %w", err)
	}

	lines := []string{
		fmt.Sprintf("Invoice %s", doc.Invoice.Number),
		fmt.Sprintf("Issued by: %s", doc.Profile.CompanyName),
		fmt.Sprintf("Billed to: %s", doc.Customer.Name),
		fmt.Sprintf("Issue date: %s", doc.Invoice.IssueDate.Format("2006-01-02")),
		fmt.Sprintf("Due date: %s", doc.Invoice.DueDate.Format("2006-01-02")),
	}
	for _, row := range buildRows(doc.Invoice) {
		lines = append(lines, fmt.Sprintf("%s | %s | %d | %s", row.Name, row.Description, row.Quantity, invoice.FormatMoneyEUR(row.TotalCents)))
	}
	lines = append(lines, fmt.Sprintf("Amount due: %s", invoice.FormatMoneyEUR(doc.Invoice.TotalAmountCents)))

	content := buildPDFContent(lines)
	pdfBytes := buildPDFFile(content)
	if err := os.WriteFile(path, pdfBytes, 0o644); err != nil {
		return fmt.Errorf("write pdf file: %w", err)
	}

	return nil
}

func buildRows(inv invoice.Invoice) []documentRow {
	items := invoice.ComputeLineTotals(inv.LineItems)
	rows := make([]documentRow, 0, len(items)+1)
	for _, item := range items {
		rows = append(rows, documentRow{
			Name:           item.Name,
			Description:    item.Description,
			Quantity:       item.Quantity,
			UnitPriceCents: item.UnitPriceCents,
			TotalCents:     item.TotalCents,
		})
	}

	if inv.FXSnapshot != nil && inv.OnCallNOK > 0 {
		rows = append(rows, documentRow{
			Name:           "On call",
			Description:    fmt.Sprintf("NOK %d converted using NOK/EUR rate %.4f", inv.OnCallNOK, inv.FXSnapshot.Rate),
			Quantity:       1,
			UnitPriceCents: invoice.CalculateOnCallEURCents(inv.OnCallNOK, inv.FXSnapshot.Rate),
			TotalCents:     invoice.CalculateOnCallEURCents(inv.OnCallNOK, inv.FXSnapshot.Rate),
		})
	}

	return rows
}

func buildPDFContent(lines []string) string {
	var builder strings.Builder
	builder.WriteString("BT\n/F1 12 Tf\n50 790 Td\n")
	for i, line := range lines {
		if i > 0 {
			builder.WriteString("0 -18 Td\n")
		}
		builder.WriteString("(")
		builder.WriteString(escapePDFText(line))
		builder.WriteString(") Tj\n")
	}
	builder.WriteString("ET")
	return builder.String()
}

func buildPDFFile(content string) []byte {
	var out bytes.Buffer
	offsets := []int{0}

	writeObject := func(id int, body string) {
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", id, body)
	}

	out.WriteString("%PDF-1.4\n")
	writeObject(1, "<< /Type /Catalog /Pages 2 0 R >>")
	writeObject(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	writeObject(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>")
	writeObject(4, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content))
	writeObject(5, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")

	xrefStart := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n", len(offsets))
	out.WriteString("0000000000 65535 f \n")
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xrefStart)

	return out.Bytes()
}

func escapePDFText(input string) string {
	replacer := strings.NewReplacer(
		`\\`, `\\\\`,
		`(`, `\(`,
		`)`, `\)`,
	)
	return replacer.Replace(input)
}
