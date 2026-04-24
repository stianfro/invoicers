package invoice

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Repository interface {
	LatestInvoice(ctx context.Context) (Invoice, error)
	DefaultTemplate(ctx context.Context) (Template, error)
	GetTemplate(ctx context.Context, id int64) (Template, error)
	CreateInvoice(ctx context.Context, inv Invoice) (Invoice, error)
	GetInvoice(ctx context.Context, id int64) (Invoice, error)
	SaveEmailDraft(ctx context.Context, draft EmailDraft) (EmailDraft, error)
}

type Service struct {
	repo Repository
	fx   any
	rndr any
}

func NewService(repo Repository, fx any, renderer any) *Service {
	return &Service{
		repo: repo,
		fx:   fx,
		rndr: renderer,
	}
}

func (s *Service) CreateNextInvoice(ctx context.Context, now time.Time) (Invoice, error) {
	tmpl, err := s.repo.DefaultTemplate(ctx)
	if err != nil {
		return Invoice{}, fmt.Errorf("load default template: %w", err)
	}

	latest, err := s.repo.LatestInvoice(ctx)
	number := "INV-001"
	month := MonthStart(now)
	if err == nil {
		number, err = NextInvoiceNumber(latest.Number)
		if err != nil {
			return Invoice{}, fmt.Errorf("calculate next invoice number: %w", err)
		}
		month = MonthStart(latest.InvoiceMonth.AddDate(0, 1, 0))
	}

	lineItems := make([]LineItem, len(tmpl.DefaultLineItems))
	for i, item := range tmpl.DefaultLineItems {
		item.Name = FillTemplatePlaceholders(item.Name, month)
		item.Description = FillTemplatePlaceholders(item.Description, month)
		lineItems[i] = item
	}

	inv := Invoice{
		Number:         number,
		InvoiceMonth:   month,
		IssueDate:      MonthStart(now).AddDate(0, 0, now.Day()-1),
		DueDate:        MonthStart(now).AddDate(0, 0, now.Day()-1+tmpl.PaymentTermsDays),
		Status:         StatusDraft,
		CustomerID:     tmpl.CustomerID,
		TemplateID:     tmpl.ID,
		LineItems:      lineItems,
		EmailRecipient: tmpl.EmailRecipient,
	}

	created, err := s.repo.CreateInvoice(ctx, inv)
	if err != nil {
		return Invoice{}, fmt.Errorf("create invoice: %w", err)
	}

	return created, nil
}

func (s *Service) GenerateEmailDraft(ctx context.Context, invoiceID int64) (EmailDraft, error) {
	inv, err := s.repo.GetInvoice(ctx, invoiceID)
	if err != nil {
		return EmailDraft{}, fmt.Errorf("load invoice: %w", err)
	}

	tmpl, err := s.repo.GetTemplate(ctx, inv.TemplateID)
	if err != nil {
		return EmailDraft{}, fmt.Errorf("load template: %w", err)
	}

	subject := replaceEmailPlaceholders(tmpl.EmailSubjectTmpl, inv)
	body := replaceEmailPlaceholders(tmpl.EmailBodyTmpl, inv)
	if strings.TrimSpace(subject) == "" {
		subject = fmt.Sprintf("Invoice %s", inv.Number)
	}

	draft, err := s.repo.SaveEmailDraft(ctx, EmailDraft{
		InvoiceID:      inv.ID,
		Recipient:      tmpl.EmailRecipient,
		Subject:        subject,
		Body:           body,
		AttachmentPath: inv.PDFPath,
	})
	if err != nil {
		return EmailDraft{}, fmt.Errorf("save email draft: %w", err)
	}

	return draft, nil
}

func FormatMoneyEUR(cents int64) string {
	return fmt.Sprintf("%.2f EUR", float64(cents)/100)
}

func replaceEmailPlaceholders(input string, inv Invoice) string {
	replacer := strings.NewReplacer(
		"{{invoice_number}}", inv.Number,
		"{{month_name}}", inv.InvoiceMonth.Month().String(),
		"{{year}}", fmt.Sprintf("%d", inv.InvoiceMonth.Year()),
		"{{total_amount}}", FormatMoneyEUR(inv.TotalAmountCents),
	)
	return replacer.Replace(input)
}
