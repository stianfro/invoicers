package cli

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/stianfro/invoicers/internal/fx"
	"github.com/stianfro/invoicers/internal/httpapp"
	"github.com/stianfro/invoicers/internal/importer"
	"github.com/stianfro/invoicers/internal/invoice"
	"github.com/stianfro/invoicers/internal/render"
	"github.com/stianfro/invoicers/internal/store"
)

type Config struct {
	Addr    string
	DataDir string
	DBPath  string
}

func Run(args []string) error {
	command := "serve"
	if len(args) > 0 {
		command = args[0]
		args = args[1:]
	}

	switch command {
	case "serve":
		return runServe(args)
	case "import-history":
		return runImportHistory(args)
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

func runServe(args []string) error {
	cfg := defaultConfig()

	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.StringVar(&cfg.Addr, "addr", cfg.Addr, "listen address")
	fs.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "directory for database and generated artifacts")
	fs.StringVar(&cfg.DBPath, "db-path", cfg.DBPath, "path to sqlite database file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx := context.Background()
	db, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		return err
	}
	if err := seedDefaults(ctx, db); err != nil {
		return err
	}

	server := httpapp.NewServer(httpapp.Options{
		Store:          db,
		InvoiceService: invoice.NewService(db, nil, nil),
		Renderer:       render.NewRenderer(),
		FXClient:       fx.NewClient(http.DefaultClient, ""),
		DataDir:        cfg.DataDir,
	})

	log.Printf("listening on %s", cfg.Addr)
	return http.ListenAndServe(cfg.Addr, server)
}

func runImportHistory(args []string) error {
	cfg := defaultConfig()
	sourcePath := ""

	fs := flag.NewFlagSet("import-history", flag.ContinueOnError)
	fs.StringVar(&sourcePath, "source", "", "path to historical invoice directory")
	fs.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "directory for database and generated artifacts")
	fs.StringVar(&cfg.DBPath, "db-path", cfg.DBPath, "path to sqlite database file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if sourcePath == "" {
		return fmt.Errorf("-source is required")
	}

	ctx := context.Background()
	db, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		return err
	}

	result, err := importer.New(db, cfg.DataDir, fx.NewClient(http.DefaultClient, "")).Import(ctx, sourcePath)
	if err != nil {
		return err
	}

	log.Printf("imported %d invoices from %s", result.ImportedInvoices, sourcePath)
	return nil
}

func defaultConfig() Config {
	dataDir := os.Getenv("INVOICERS_DATA_DIR")
	if dataDir == "" {
		dataDir = "data"
	}

	dbPath := os.Getenv("INVOICERS_DB_PATH")
	if dbPath == "" {
		dbPath = filepath.Join(dataDir, "invoicers.db")
	}

	addr := os.Getenv("INVOICERS_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	return Config{
		Addr:    addr,
		DataDir: dataDir,
		DBPath:  dbPath,
	}
}

func seedDefaults(ctx context.Context, db *store.SQLiteStore) error {
	if _, err := db.DefaultTemplate(ctx); err == nil {
		return nil
	}

	customerID, err := db.UpsertCustomer(ctx, invoice.Customer{
		Name:  "Intility AS",
		Email: "accounts@intility.no",
	})
	if err != nil {
		return fmt.Errorf("seed customer id: %w", err)
	}

	if _, err := db.UpsertTemplate(ctx, invoice.Template{
		Name:             "Default",
		CustomerID:       customerID,
		EmailRecipient:   "accounts@intility.no",
		EmailSubjectTmpl: "Invoice {{invoice_number}} for {{month_name}} {{year}}",
		EmailBodyTmpl:    "Hi,\n\nAttached is invoice {{invoice_number}} totaling {{total_amount}}.\n",
		PaymentTermsDays: 14,
		DefaultLineItems: []invoice.LineItem{{
			Name:           "Consultancy work",
			Description:    "{{month_name}}",
			Quantity:       1,
			UnitPriceCents: 740546,
		}},
		OnCallEnabled:   true,
		DefaultCurrency: "EUR",
	}); err != nil {
		return fmt.Errorf("seed template: %w", err)
	}

	return nil
}
