package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/stianfro/invoicers/internal/invoice"
	"github.com/stianfro/invoicers/migrations"
	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func Open(ctx context.Context, dbPath string) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite database: %w", err)
	}

	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = ON;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) Migrate(ctx context.Context) error {
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}

		sqlBytes, err := migrations.FS.ReadFile(entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}

		if _, err := s.db.ExecContext(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("apply migration %s: %w", entry.Name(), err)
		}
	}

	return nil
}

func (s *SQLiteStore) UpsertCustomer(ctx context.Context, customer invoice.Customer) (int64, error) {
	if _, err := s.db.ExecContext(
		ctx,
		`INSERT INTO customers (name, email)
		 VALUES (?, ?)
		 ON CONFLICT(name) DO UPDATE SET email = excluded.email`,
		customer.Name,
		customer.Email,
	); err != nil {
		return 0, fmt.Errorf("upsert customer: %w", err)
	}

	var id int64
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT id FROM customers WHERE name = ?`,
		customer.Name,
	).Scan(&id); err != nil {
		return 0, fmt.Errorf("lookup customer id: %w", err)
	}

	return id, nil
}

func (s *SQLiteStore) UpsertTemplate(ctx context.Context, tmpl invoice.Template) (int64, error) {
	lineItemsJSON, err := json.Marshal(tmpl.DefaultLineItems)
	if err != nil {
		return 0, fmt.Errorf("marshal template line items: %w", err)
	}

	var id int64
	err = s.db.QueryRowContext(
		ctx,
		`SELECT id FROM templates WHERE name = ? AND customer_id = ? ORDER BY id DESC LIMIT 1`,
		tmpl.Name,
		tmpl.CustomerID,
	).Scan(&id)
	if err != nil && err != sql.ErrNoRows {
		return 0, fmt.Errorf("lookup template id: %w", err)
	}

	if id == 0 {
		result, err := s.db.ExecContext(
			ctx,
			`INSERT INTO templates
			 (name, customer_id, email_recipient, email_subject_tmpl, email_body_tmpl, payment_terms_days, default_line_items_json, on_call_enabled, default_currency, default_customer_po)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			tmpl.Name,
			tmpl.CustomerID,
			tmpl.EmailRecipient,
			tmpl.EmailSubjectTmpl,
			tmpl.EmailBodyTmpl,
			tmpl.PaymentTermsDays,
			string(lineItemsJSON),
			boolToInt(tmpl.OnCallEnabled),
			tmpl.DefaultCurrency,
			tmpl.DefaultCustomerPO,
		)
		if err != nil {
			return 0, fmt.Errorf("insert template: %w", err)
		}
		id, err = result.LastInsertId()
		if err != nil {
			return 0, fmt.Errorf("read inserted template id: %w", err)
		}
		return id, nil
	}

	if _, err := s.db.ExecContext(
		ctx,
		`UPDATE templates
		 SET email_recipient = ?,
		     email_subject_tmpl = ?,
		     email_body_tmpl = ?,
		     payment_terms_days = ?,
		     default_line_items_json = ?,
		     on_call_enabled = ?,
		     default_currency = ?,
		     default_customer_po = ?
		 WHERE id = ?`,
		tmpl.EmailRecipient,
		tmpl.EmailSubjectTmpl,
		tmpl.EmailBodyTmpl,
		tmpl.PaymentTermsDays,
		string(lineItemsJSON),
		boolToInt(tmpl.OnCallEnabled),
		tmpl.DefaultCurrency,
		tmpl.DefaultCustomerPO,
		id,
	); err != nil {
		return 0, fmt.Errorf("update template: %w", err)
	}

	return id, nil
}

func (s *SQLiteStore) CreateInvoice(ctx context.Context, inv invoice.Invoice) (invoice.Invoice, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return invoice.Invoice{}, fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	now := time.Now().UTC()
	total := invoice.ComputeInvoiceTotalCents(inv.LineItems, inv.FXSnapshot, inv.OnCallNOK)
	result, err := tx.ExecContext(
		ctx,
		`INSERT INTO invoices
		 (number, invoice_month, issue_date, due_date, status, customer_id, template_id, on_call_nok, fx_rate, fx_currency, fx_base, fx_observed_at, fx_source, html_path, pdf_path, email_recipient, email_subject, email_body, total_amount_cents, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		inv.Number,
		invoice.MonthStart(inv.InvoiceMonth).Format(time.DateOnly),
		inv.IssueDate.Format(time.DateOnly),
		inv.DueDate.Format(time.DateOnly),
		inv.Status,
		inv.CustomerID,
		inv.TemplateID,
		inv.OnCallNOK,
		nullableRate(inv.FXSnapshot),
		nullableString(fxCurrency(inv.FXSnapshot)),
		nullableString(fxBase(inv.FXSnapshot)),
		nullableTime(fxObservedAt(inv.FXSnapshot)),
		nullableString(fxSource(inv.FXSnapshot)),
		inv.HTMLPath,
		inv.PDFPath,
		inv.EmailRecipient,
		emailSubject(inv.EmailDraft),
		emailBody(inv.EmailDraft),
		total,
		now.Format(time.RFC3339),
		now.Format(time.RFC3339),
	)
	if err != nil {
		return invoice.Invoice{}, fmt.Errorf("insert invoice: %w", err)
	}

	invoiceID, err := result.LastInsertId()
	if err != nil {
		return invoice.Invoice{}, fmt.Errorf("read invoice id: %w", err)
	}

	for position, line := range invoice.ComputeLineTotals(inv.LineItems) {
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO invoice_lines (invoice_id, position, name, description, quantity, unit_price_cents, total_cents)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			invoiceID,
			position,
			line.Name,
			line.Description,
			line.Quantity,
			line.UnitPriceCents,
			line.TotalCents,
		); err != nil {
			return invoice.Invoice{}, fmt.Errorf("insert invoice line: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return invoice.Invoice{}, fmt.Errorf("commit invoice insert: %w", err)
	}

	inv.ID = invoiceID
	inv.TotalAmountCents = total
	inv.CreatedAt = now
	inv.UpdatedAt = now
	inv.LineItems = invoice.ComputeLineTotals(inv.LineItems)
	return inv, nil
}

func (s *SQLiteStore) ListInvoices(ctx context.Context) ([]invoice.Invoice, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, number, invoice_month, issue_date, due_date, status, customer_id, template_id, on_call_nok, total_amount_cents, created_at, updated_at
		 FROM invoices
		 ORDER BY invoice_month DESC, id DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("query invoices: %w", err)
	}
	defer rows.Close()

	var invoices []invoice.Invoice
	for rows.Next() {
		var inv invoice.Invoice
		var invoiceMonth string
		var issueDate string
		var dueDate string
		var createdAt string
		var updatedAt string
		if err := rows.Scan(
			&inv.ID,
			&inv.Number,
			&invoiceMonth,
			&issueDate,
			&dueDate,
			&inv.Status,
			&inv.CustomerID,
			&inv.TemplateID,
			&inv.OnCallNOK,
			&inv.TotalAmountCents,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan invoice: %w", err)
		}

		inv.InvoiceMonth, err = time.Parse(time.DateOnly, invoiceMonth)
		if err != nil {
			return nil, fmt.Errorf("parse invoice month: %w", err)
		}
		inv.IssueDate, err = time.Parse(time.DateOnly, issueDate)
		if err != nil {
			return nil, fmt.Errorf("parse issue date: %w", err)
		}
		inv.DueDate, err = time.Parse(time.DateOnly, dueDate)
		if err != nil {
			return nil, fmt.Errorf("parse due date: %w", err)
		}
		inv.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse created_at: %w", err)
		}
		inv.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
		if err != nil {
			return nil, fmt.Errorf("parse updated_at: %w", err)
		}

		invoices = append(invoices, inv)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate invoices: %w", err)
	}

	return invoices, nil
}

func (s *SQLiteStore) LatestInvoice(ctx context.Context) (invoice.Invoice, error) {
	rows, err := s.ListInvoices(ctx)
	if err != nil {
		return invoice.Invoice{}, err
	}
	if len(rows) == 0 {
		return invoice.Invoice{}, sql.ErrNoRows
	}
	return rows[0], nil
}

func (s *SQLiteStore) DefaultTemplate(ctx context.Context) (invoice.Template, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, name, customer_id, email_recipient, email_subject_tmpl, email_body_tmpl, payment_terms_days, default_line_items_json, on_call_enabled, default_currency, default_customer_po
		 FROM templates
		 ORDER BY id ASC
		 LIMIT 1`,
	)
	return scanTemplate(row)
}

func (s *SQLiteStore) GetTemplate(ctx context.Context, id int64) (invoice.Template, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, name, customer_id, email_recipient, email_subject_tmpl, email_body_tmpl, payment_terms_days, default_line_items_json, on_call_enabled, default_currency, default_customer_po
		 FROM templates
		 WHERE id = ?`,
		id,
	)
	return scanTemplate(row)
}

func (s *SQLiteStore) GetInvoice(ctx context.Context, id int64) (invoice.Invoice, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, number, invoice_month, issue_date, due_date, status, customer_id, template_id, on_call_nok, fx_rate, fx_currency, fx_base, fx_observed_at, fx_source, html_path, pdf_path, email_recipient, email_subject, email_body, total_amount_cents, created_at, updated_at
		 FROM invoices
		 WHERE id = ?`,
		id,
	)

	var inv invoice.Invoice
	var invoiceMonth string
	var issueDate string
	var dueDate string
	var fxRate sql.NullFloat64
	var fxCurrency sql.NullString
	var fxBase sql.NullString
	var fxObservedAt sql.NullString
	var fxSource sql.NullString
	var emailRecipient string
	var emailSubject string
	var emailBody string
	var createdAt string
	var updatedAt string
	if err := row.Scan(
		&inv.ID,
		&inv.Number,
		&invoiceMonth,
		&issueDate,
		&dueDate,
		&inv.Status,
		&inv.CustomerID,
		&inv.TemplateID,
		&inv.OnCallNOK,
		&fxRate,
		&fxCurrency,
		&fxBase,
		&fxObservedAt,
		&fxSource,
		&inv.HTMLPath,
		&inv.PDFPath,
		&emailRecipient,
		&emailSubject,
		&emailBody,
		&inv.TotalAmountCents,
		&createdAt,
		&updatedAt,
	); err != nil {
		return invoice.Invoice{}, fmt.Errorf("load invoice: %w", err)
	}

	var err error
	inv.InvoiceMonth, err = time.Parse(time.DateOnly, invoiceMonth)
	if err != nil {
		return invoice.Invoice{}, fmt.Errorf("parse invoice month: %w", err)
	}
	inv.IssueDate, err = time.Parse(time.DateOnly, issueDate)
	if err != nil {
		return invoice.Invoice{}, fmt.Errorf("parse issue date: %w", err)
	}
	inv.DueDate, err = time.Parse(time.DateOnly, dueDate)
	if err != nil {
		return invoice.Invoice{}, fmt.Errorf("parse due date: %w", err)
	}
	inv.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return invoice.Invoice{}, fmt.Errorf("parse created_at: %w", err)
	}
	inv.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return invoice.Invoice{}, fmt.Errorf("parse updated_at: %w", err)
	}

	if fxRate.Valid {
		fxSnapshot := &invoice.FXSnapshot{
			Rate:     fxRate.Float64,
			Currency: fxCurrency.String,
			Base:     fxBase.String,
			Source:   fxSource.String,
		}
		if fxObservedAt.Valid {
			fxSnapshot.ObservedAt, err = time.Parse(time.RFC3339, fxObservedAt.String)
			if err != nil {
				return invoice.Invoice{}, fmt.Errorf("parse fx observed at: %w", err)
			}
		}
		inv.FXSnapshot = fxSnapshot
	}

	inv.EmailRecipient = emailRecipient
	if emailRecipient != "" || emailSubject != "" || emailBody != "" {
		inv.EmailDraft = &invoice.EmailDraft{
			InvoiceID:      inv.ID,
			Recipient:      emailRecipient,
			Subject:        emailSubject,
			Body:           emailBody,
			AttachmentPath: inv.PDFPath,
		}
	}

	lines, err := s.listInvoiceLines(ctx, inv.ID)
	if err != nil {
		return invoice.Invoice{}, err
	}
	inv.LineItems = lines

	return inv, nil
}

func (s *SQLiteStore) SaveEmailDraft(ctx context.Context, draft invoice.EmailDraft) (invoice.EmailDraft, error) {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO email_drafts (invoice_id, recipient, subject, body, attachment_path, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(invoice_id) DO UPDATE SET
		   recipient = excluded.recipient,
		   subject = excluded.subject,
		   body = excluded.body,
		   attachment_path = excluded.attachment_path,
		   created_at = excluded.created_at`,
		draft.InvoiceID,
		draft.Recipient,
		draft.Subject,
		draft.Body,
		draft.AttachmentPath,
		now.Format(time.RFC3339),
	)
	if err != nil {
		return invoice.EmailDraft{}, fmt.Errorf("save email draft: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil || id == 0 {
		if err := s.db.QueryRowContext(ctx, `SELECT id FROM email_drafts WHERE invoice_id = ?`, draft.InvoiceID).Scan(&id); err != nil {
			return invoice.EmailDraft{}, fmt.Errorf("lookup email draft id: %w", err)
		}
	}

	draft.ID = id
	draft.CreatedAt = now
	if _, err := s.db.ExecContext(
		ctx,
		`UPDATE invoices SET email_recipient = ?, email_subject = ?, email_body = ?, updated_at = ? WHERE id = ?`,
		draft.Recipient,
		draft.Subject,
		draft.Body,
		now.Format(time.RFC3339),
		draft.InvoiceID,
	); err != nil {
		return invoice.EmailDraft{}, fmt.Errorf("update invoice email metadata: %w", err)
	}

	return draft, nil
}

func (s *SQLiteStore) UpsertProfile(ctx context.Context, profile invoice.Profile) error {
	companyAddressJSON, err := json.Marshal(profile.CompanyAddress)
	if err != nil {
		return fmt.Errorf("marshal company address: %w", err)
	}
	bankAddressJSON, err := json.Marshal(profile.BankAddress)
	if err != nil {
		return fmt.Errorf("marshal bank address: %w", err)
	}

	if _, err := s.db.ExecContext(
		ctx,
		`UPDATE profiles
		 SET company_name = ?,
		     company_address_json = ?,
		     account_name = ?,
		     iban = ?,
		     bic = ?,
		     bank_name = ?,
		     bank_address_json = ?,
		     default_currency = ?
		 WHERE id = 1`,
		profile.CompanyName,
		string(companyAddressJSON),
		profile.AccountName,
		profile.IBAN,
		profile.BIC,
		profile.BankName,
		string(bankAddressJSON),
		profile.DefaultCurrency,
	); err != nil {
		return fmt.Errorf("update profile: %w", err)
	}

	return nil
}

func (s *SQLiteStore) GetProfile(ctx context.Context) (invoice.Profile, error) {
	var profile invoice.Profile
	var companyAddressJSON string
	var bankAddressJSON string
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT id, company_name, company_address_json, account_name, iban, bic, bank_name, bank_address_json, default_currency
		 FROM profiles
		 WHERE id = 1`,
	).Scan(
		&profile.ID,
		&profile.CompanyName,
		&companyAddressJSON,
		&profile.AccountName,
		&profile.IBAN,
		&profile.BIC,
		&profile.BankName,
		&bankAddressJSON,
		&profile.DefaultCurrency,
	); err != nil {
		return invoice.Profile{}, fmt.Errorf("load profile: %w", err)
	}

	if err := json.Unmarshal([]byte(companyAddressJSON), &profile.CompanyAddress); err != nil {
		return invoice.Profile{}, fmt.Errorf("unmarshal company address: %w", err)
	}
	if err := json.Unmarshal([]byte(bankAddressJSON), &profile.BankAddress); err != nil {
		return invoice.Profile{}, fmt.Errorf("unmarshal bank address: %w", err)
	}

	return profile, nil
}

func (s *SQLiteStore) GetCustomer(ctx context.Context, id int64) (invoice.Customer, error) {
	var customer invoice.Customer
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT id, name, email FROM customers WHERE id = ?`,
		id,
	).Scan(&customer.ID, &customer.Name, &customer.Email); err != nil {
		return invoice.Customer{}, fmt.Errorf("load customer: %w", err)
	}
	return customer, nil
}

func (s *SQLiteStore) ListTemplates(ctx context.Context) ([]invoice.Template, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, name, customer_id, email_recipient, email_subject_tmpl, email_body_tmpl, payment_terms_days, default_line_items_json, on_call_enabled, default_currency, default_customer_po
		 FROM templates
		 ORDER BY id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("query templates: %w", err)
	}
	defer rows.Close()

	var templates []invoice.Template
	for rows.Next() {
		var tmpl invoice.Template
		var defaultLineItemsJSON string
		var onCallEnabled int
		if err := rows.Scan(
			&tmpl.ID,
			&tmpl.Name,
			&tmpl.CustomerID,
			&tmpl.EmailRecipient,
			&tmpl.EmailSubjectTmpl,
			&tmpl.EmailBodyTmpl,
			&tmpl.PaymentTermsDays,
			&defaultLineItemsJSON,
			&onCallEnabled,
			&tmpl.DefaultCurrency,
			&tmpl.DefaultCustomerPO,
		); err != nil {
			return nil, fmt.Errorf("scan template: %w", err)
		}
		if err := json.Unmarshal([]byte(defaultLineItemsJSON), &tmpl.DefaultLineItems); err != nil {
			return nil, fmt.Errorf("unmarshal template line items: %w", err)
		}
		tmpl.OnCallEnabled = onCallEnabled == 1
		templates = append(templates, tmpl)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate templates: %w", err)
	}

	return templates, nil
}

func (s *SQLiteStore) UpdateInvoice(ctx context.Context, inv invoice.Invoice) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin invoice update transaction: %w", err)
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	total := invoice.ComputeInvoiceTotalCents(inv.LineItems, inv.FXSnapshot, inv.OnCallNOK)
	if _, err := tx.ExecContext(
		ctx,
		`UPDATE invoices
		 SET issue_date = ?, due_date = ?, on_call_nok = ?, total_amount_cents = ?, updated_at = ?
		 WHERE id = ?`,
		inv.IssueDate.Format(time.DateOnly),
		inv.DueDate.Format(time.DateOnly),
		inv.OnCallNOK,
		total,
		time.Now().UTC().Format(time.RFC3339),
		inv.ID,
	); err != nil {
		return fmt.Errorf("update invoice row: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM invoice_lines WHERE invoice_id = ?`, inv.ID); err != nil {
		return fmt.Errorf("delete invoice lines: %w", err)
	}

	for position, line := range invoice.ComputeLineTotals(inv.LineItems) {
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO invoice_lines (invoice_id, position, name, description, quantity, unit_price_cents, total_cents)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			inv.ID,
			position,
			line.Name,
			line.Description,
			line.Quantity,
			line.UnitPriceCents,
			line.TotalCents,
		); err != nil {
			return fmt.Errorf("reinsert invoice line: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit invoice update: %w", err)
	}

	return nil
}

func (s *SQLiteStore) UpdateInvoiceArtifacts(ctx context.Context, invoiceID int64, htmlPath string, pdfPath string, fx *invoice.FXSnapshot) error {
	var rate any
	var currency any
	var base any
	var observedAt any
	var source any
	if fx != nil {
		rate = fx.Rate
		currency = fx.Currency
		base = fx.Base
		source = fx.Source
		if !fx.ObservedAt.IsZero() {
			observedAt = fx.ObservedAt.Format(time.RFC3339)
		}
	}

	if _, err := s.db.ExecContext(
		ctx,
		`UPDATE invoices
		 SET html_path = ?, pdf_path = ?, fx_rate = ?, fx_currency = ?, fx_base = ?, fx_observed_at = ?, fx_source = ?, updated_at = ?
		 WHERE id = ?`,
		htmlPath,
		pdfPath,
		rate,
		currency,
		base,
		observedAt,
		source,
		time.Now().UTC().Format(time.RFC3339),
		invoiceID,
	); err != nil {
		return fmt.Errorf("update invoice artifacts: %w", err)
	}

	return nil
}

func (s *SQLiteStore) UpdateInvoiceStatus(ctx context.Context, invoiceID int64, status invoice.Status) error {
	if _, err := s.db.ExecContext(
		ctx,
		`UPDATE invoices SET status = ?, updated_at = ? WHERE id = ?`,
		status,
		time.Now().UTC().Format(time.RFC3339),
		invoiceID,
	); err != nil {
		return fmt.Errorf("update invoice status: %w", err)
	}
	return nil
}

func scanTemplate(row *sql.Row) (invoice.Template, error) {
	var tmpl invoice.Template
	var defaultLineItemsJSON string
	var onCallEnabled int
	if err := row.Scan(
		&tmpl.ID,
		&tmpl.Name,
		&tmpl.CustomerID,
		&tmpl.EmailRecipient,
		&tmpl.EmailSubjectTmpl,
		&tmpl.EmailBodyTmpl,
		&tmpl.PaymentTermsDays,
		&defaultLineItemsJSON,
		&onCallEnabled,
		&tmpl.DefaultCurrency,
		&tmpl.DefaultCustomerPO,
	); err != nil {
		return invoice.Template{}, fmt.Errorf("load template: %w", err)
	}

	if err := json.Unmarshal([]byte(defaultLineItemsJSON), &tmpl.DefaultLineItems); err != nil {
		return invoice.Template{}, fmt.Errorf("unmarshal template line items: %w", err)
	}
	tmpl.OnCallEnabled = onCallEnabled == 1
	return tmpl, nil
}

func (s *SQLiteStore) listInvoiceLines(ctx context.Context, invoiceID int64) ([]invoice.LineItem, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, name, description, quantity, unit_price_cents, total_cents
		 FROM invoice_lines
		 WHERE invoice_id = ?
		 ORDER BY position ASC, id ASC`,
		invoiceID,
	)
	if err != nil {
		return nil, fmt.Errorf("query invoice lines: %w", err)
	}
	defer rows.Close()

	var lines []invoice.LineItem
	for rows.Next() {
		var line invoice.LineItem
		if err := rows.Scan(
			&line.ID,
			&line.Name,
			&line.Description,
			&line.Quantity,
			&line.UnitPriceCents,
			&line.TotalCents,
		); err != nil {
			return nil, fmt.Errorf("scan invoice line: %w", err)
		}
		lines = append(lines, line)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate invoice lines: %w", err)
	}

	return lines, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func nullableRate(fx *invoice.FXSnapshot) any {
	if fx == nil {
		return nil
	}
	return fx.Rate
}

func fxCurrency(fx *invoice.FXSnapshot) string {
	if fx == nil {
		return ""
	}
	return fx.Currency
}

func fxBase(fx *invoice.FXSnapshot) string {
	if fx == nil {
		return ""
	}
	return fx.Base
}

func fxObservedAt(fx *invoice.FXSnapshot) time.Time {
	if fx == nil {
		return time.Time{}
	}
	return fx.ObservedAt
}

func fxSource(fx *invoice.FXSnapshot) string {
	if fx == nil {
		return ""
	}
	return fx.Source
}

func nullableString(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func nullableTime(v time.Time) any {
	if v.IsZero() {
		return nil
	}
	return v.Format(time.RFC3339)
}

func emailSubject(draft *invoice.EmailDraft) string {
	if draft == nil {
		return ""
	}
	return draft.Subject
}

func emailBody(draft *invoice.EmailDraft) string {
	if draft == nil {
		return ""
	}
	return draft.Body
}
