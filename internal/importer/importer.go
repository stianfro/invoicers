package importer

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/stianfro/invoicers/internal/invoice"
	"gopkg.in/yaml.v3"
)

type Store interface {
	UpsertProfile(ctx context.Context, profile invoice.Profile) error
	UpsertCustomer(ctx context.Context, customer invoice.Customer) (int64, error)
	UpsertTemplate(ctx context.Context, tmpl invoice.Template) (int64, error)
	CreateInvoice(ctx context.Context, inv invoice.Invoice) (invoice.Invoice, error)
}

type FXProvider interface {
	RateForMonth(ctx context.Context, month time.Time) (*invoice.FXSnapshot, error)
}

type Importer struct {
	store    Store
	dataDir  string
	fxClient FXProvider
}

type Result struct {
	ImportedInvoices int
}

func New(store Store, dataDir string, fxClient FXProvider) *Importer {
	return &Importer{
		store:    store,
		dataDir:  dataDir,
		fxClient: fxClient,
	}
}

func (i *Importer) Import(ctx context.Context, sourceDir string) (Result, error) {
	configPath := filepath.Join(sourceDir, "config.yaml")
	var cfg legacyConfig
	if err := readYAML(configPath, &cfg); err != nil {
		return Result{}, fmt.Errorf("read config yaml: %w", err)
	}

	if err := i.store.UpsertProfile(ctx, invoice.Profile{
		CompanyName:     cfg.CompanyName,
		CompanyAddress:  cfg.CompanyAddress,
		AccountName:     cfg.AccountName,
		IBAN:            cfg.IBAN,
		BIC:             cfg.BIC,
		BankName:        cfg.BankName,
		BankAddress:     cfg.BankAddress,
		DefaultCurrency: "EUR",
	}); err != nil {
		return Result{}, fmt.Errorf("upsert profile: %w", err)
	}

	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return Result{}, fmt.Errorf("read source dir: %w", err)
	}

	sort.Slice(entries, func(a, b int) bool {
		return entries[a].Name() < entries[b].Name()
	})

	result := Result{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		month, ok := parseMonthDir(entry.Name())
		if !ok {
			continue
		}

		monthDir := filepath.Join(sourceDir, entry.Name())
		yamlFiles, err := filepath.Glob(filepath.Join(monthDir, "INV-*.yaml"))
		if err != nil {
			return Result{}, fmt.Errorf("glob yaml files: %w", err)
		}
		sort.Strings(yamlFiles)

		for _, yamlPath := range yamlFiles {
			var invFile legacyInvoice
			if err := readYAML(yamlPath, &invFile); err != nil {
				return Result{}, fmt.Errorf("read invoice yaml %s: %w", yamlPath, err)
			}

			customerID, err := i.store.UpsertCustomer(ctx, invoice.Customer{
				Name: invFile.CustomerName,
			})
			if err != nil {
				return Result{}, fmt.Errorf("upsert customer: %w", err)
			}

			defaultLines := make([]invoice.LineItem, 0, len(invFile.Services))
			importLines := make([]invoice.LineItem, 0, len(invFile.Services))
			for _, svc := range invFile.Services {
				line := invoice.LineItem{
					Name:           svc.Name,
					Description:    svc.Description,
					Quantity:       svc.Quantity,
					UnitPriceCents: moneyToCents(svc.Price),
				}
				defaultLines = append(defaultLines, line)
				importLines = append(importLines, line)
			}

			templateID, err := i.store.UpsertTemplate(ctx, invoice.Template{
				Name:             "Imported - " + invFile.CustomerName,
				CustomerID:       customerID,
				EmailRecipient:   "",
				EmailSubjectTmpl: "Invoice {{invoice_number}}",
				EmailBodyTmpl:    "Attached is invoice {{invoice_number}}.",
				PaymentTermsDays: 14,
				DefaultLineItems: defaultLines,
				OnCallEnabled:    invFile.OnCallNOK > 0,
				DefaultCurrency:  "EUR",
			})
			if err != nil {
				return Result{}, fmt.Errorf("upsert template: %w", err)
			}

			issueDate := month
			if parsed, err := parseLooseDate(invFile.IssueDate); err == nil {
				issueDate = parsed
			}
			dueDate := month.AddDate(0, 0, 14)
			if parsed, err := parseLooseDate(invFile.DueDate); err == nil {
				dueDate = parsed
			}

			var snapshot *invoice.FXSnapshot
			if i.fxClient != nil && invFile.OnCallNOK > 0 {
				snapshot, _ = i.fxClient.RateForMonth(ctx, month)
			}

			htmlPath, pdfPath, err := i.copyArtifacts(monthDir, invFile.Name)
			if err != nil {
				return Result{}, err
			}

			if _, err := i.store.CreateInvoice(ctx, invoice.Invoice{
				Number:         invFile.Name,
				InvoiceMonth:   month,
				IssueDate:      issueDate,
				DueDate:        dueDate,
				Status:         invoice.StatusSent,
				CustomerID:     customerID,
				TemplateID:     templateID,
				LineItems:      importLines,
				OnCallNOK:      invFile.OnCallNOK,
				FXSnapshot:     snapshot,
				HTMLPath:       htmlPath,
				PDFPath:        pdfPath,
				EmailRecipient: "",
			}); err != nil {
				return Result{}, fmt.Errorf("create imported invoice: %w", err)
			}

			result.ImportedInvoices++
		}
	}

	return result, nil
}

type legacyConfig struct {
	CompanyName    string   `yaml:"companyName"`
	CompanyAddress []string `yaml:"companyAddress"`
	IBAN           string   `yaml:"iban"`
	BIC            string   `yaml:"bic"`
	AccountName    string   `yaml:"accountName"`
	BankName       string   `yaml:"bankName"`
	BankAddress    []string `yaml:"bankAddress"`
}

type legacyInvoice struct {
	Name         string          `yaml:"name"`
	CustomerName string          `yaml:"customerName"`
	Services     []legacyService `yaml:"services"`
	DueDate      string          `yaml:"dueDate"`
	IssueDate    string          `yaml:"issueDate"`
	OnCallNOK    int             `yaml:"onCallNOK"`
	Month        string          `yaml:"invoiceMonth"`
}

type legacyService struct {
	Name        string  `yaml:"name"`
	Description string  `yaml:"description"`
	Quantity    int     `yaml:"quantity"`
	Price       float64 `yaml:"price"`
}

func readYAML(path string, out any) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(content, out)
}

var monthDirPattern = regexp.MustCompile(`^(\d{2})(\d{2})_[A-Za-z]+$`)

func parseMonthDir(name string) (time.Time, bool) {
	matches := monthDirPattern.FindStringSubmatch(name)
	if len(matches) != 3 {
		return time.Time{}, false
	}

	year := 2000 + mustAtoi(matches[1])
	month := mustAtoi(matches[2])
	return time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC), true
}

func mustAtoi(raw string) int {
	value, _ := strconv.Atoi(raw)
	return value
}

func moneyToCents(amount float64) int64 {
	return int64(math.Round(amount * 100))
}

func parseLooseDate(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("empty date")
	}

	layouts := []string{
		time.DateOnly,
		"2 January 2006",
		"02 January 2006",
		"2 Jan 2006",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed, nil
		}
	}

	return time.Time{}, fmt.Errorf("unsupported date format %q", raw)
}

func (i *Importer) copyArtifacts(monthDir string, invoiceNumber string) (string, string, error) {
	artifactDir := filepath.Join(i.dataDir, "artifacts", "imported")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return "", "", fmt.Errorf("create import artifact dir: %w", err)
	}

	var htmlPath string
	var pdfPath string
	for _, ext := range []string{".html", ".pdf"} {
		source := filepath.Join(monthDir, invoiceNumber+ext)
		if _, err := os.Stat(source); err != nil {
			continue
		}
		target := filepath.Join(artifactDir, invoiceNumber+ext)
		if err := copyFile(source, target); err != nil {
			return "", "", fmt.Errorf("copy artifact %s: %w", source, err)
		}
		if ext == ".html" {
			htmlPath = "/artifacts/imported/" + invoiceNumber + ext
		} else {
			pdfPath = "/artifacts/imported/" + invoiceNumber + ext
		}
	}

	return htmlPath, pdfPath, nil
}

func copyFile(source string, target string) error {
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(target)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}

	return nil
}
