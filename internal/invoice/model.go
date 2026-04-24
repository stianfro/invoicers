package invoice

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

type Status string

const (
	StatusDraft Status = "draft"
	StatusReady Status = "ready"
	StatusSent  Status = "sent"
	StatusPaid  Status = "paid"
)

type Profile struct {
	ID              int64    `json:"id"`
	CompanyName     string   `json:"companyName"`
	CompanyAddress  []string `json:"companyAddress"`
	AccountName     string   `json:"accountName"`
	IBAN            string   `json:"iban"`
	BIC             string   `json:"bic"`
	BankName        string   `json:"bankName"`
	BankAddress     []string `json:"bankAddress"`
	DefaultCurrency string   `json:"defaultCurrency"`
}

type Customer struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type Template struct {
	ID                int64      `json:"id"`
	Name              string     `json:"name"`
	CustomerID        int64      `json:"customerId"`
	EmailRecipient    string     `json:"emailRecipient"`
	EmailSubjectTmpl  string     `json:"emailSubjectTemplate"`
	EmailBodyTmpl     string     `json:"emailBodyTemplate"`
	PaymentTermsDays  int        `json:"paymentTermsDays"`
	DefaultLineItems  []LineItem `json:"defaultLineItems"`
	OnCallEnabled     bool       `json:"onCallEnabled"`
	DefaultCurrency   string     `json:"defaultCurrency"`
	DefaultCustomerPO string     `json:"defaultCustomerPo"`
}

type LineItem struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	Quantity       int    `json:"quantity"`
	UnitPriceCents int64  `json:"unitPriceCents"`
	TotalCents     int64  `json:"totalCents"`
}

type FXSnapshot struct {
	Rate       float64   `json:"rate"`
	Currency   string    `json:"currency"`
	Base       string    `json:"base"`
	ObservedAt time.Time `json:"observedAt"`
	Source     string    `json:"source"`
}

type EmailDraft struct {
	ID             int64     `json:"id"`
	InvoiceID      int64     `json:"invoiceId"`
	Recipient      string    `json:"recipient"`
	Subject        string    `json:"subject"`
	Body           string    `json:"body"`
	AttachmentPath string    `json:"attachmentPath"`
	CreatedAt      time.Time `json:"createdAt"`
}

type Invoice struct {
	ID               int64       `json:"id"`
	Number           string      `json:"number"`
	InvoiceMonth     time.Time   `json:"invoiceMonth"`
	IssueDate        time.Time   `json:"issueDate"`
	DueDate          time.Time   `json:"dueDate"`
	Status           Status      `json:"status"`
	CustomerID       int64       `json:"customerId"`
	TemplateID       int64       `json:"templateId"`
	LineItems        []LineItem  `json:"lineItems"`
	OnCallNOK        int         `json:"onCallNok"`
	FXSnapshot       *FXSnapshot `json:"fxSnapshot,omitempty"`
	HTMLPath         string      `json:"htmlPath"`
	PDFPath          string      `json:"pdfPath"`
	EmailDraft       *EmailDraft `json:"emailDraft,omitempty"`
	CustomerName     string      `json:"customerName"`
	CustomerEmail    string      `json:"customerEmail"`
	TemplateName     string      `json:"templateName"`
	EmailRecipient   string      `json:"emailRecipient"`
	TotalAmountCents int64       `json:"totalAmountCents"`
	CreatedAt        time.Time   `json:"createdAt"`
	UpdatedAt        time.Time   `json:"updatedAt"`
}

func NextInvoiceNumber(current string) (string, error) {
	parts := strings.Split(strings.TrimSpace(current), "-")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid invoice number %q", current)
	}

	value, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", fmt.Errorf("parse invoice number: %w", err)
	}

	return fmt.Sprintf("%s-%03d", parts[0], value+1), nil
}

func CalculateOnCallEURCents(nokAmount int, rate float64) int64 {
	if nokAmount <= 0 || rate <= 0 {
		return 0
	}

	base := float64(nokAmount) / rate
	withMarkup := base + (base * 0.12)
	return int64(math.Round(withMarkup * 100))
}

func FillTemplatePlaceholders(input string, month time.Time) string {
	replacer := strings.NewReplacer(
		"{{month_name}}", month.Month().String(),
		"{{year}}", fmt.Sprintf("%d", month.Year()),
		"{{month_number}}", fmt.Sprintf("%02d", month.Month()),
		"{{month_label}}", month.Format("2006-01"),
	)
	return replacer.Replace(input)
}

func ComputeLineTotals(lines []LineItem) []LineItem {
	result := make([]LineItem, len(lines))
	for i, item := range lines {
		item.TotalCents = int64(item.Quantity) * item.UnitPriceCents
		result[i] = item
	}
	return result
}

func ComputeInvoiceTotalCents(lines []LineItem, fx *FXSnapshot, onCallNOK int) int64 {
	var total int64
	for _, item := range ComputeLineTotals(lines) {
		total += item.TotalCents
	}

	if fx != nil {
		total += CalculateOnCallEURCents(onCallNOK, fx.Rate)
	}

	return total
}

func MonthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}
