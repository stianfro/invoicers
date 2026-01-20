# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Development Commands

```bash
go build              # Build the binary
go test ./...         # Run all tests
go test -run TestName # Run a specific test
go vet ./...          # Run static analysis
go mod tidy           # Fix module dependencies
```

## Running the Application

```bash
go run . -config path/to/config.yaml -invoice path/to/invoice.yaml > output.html
```

Both `-config` and `-invoice` flags are required.

## Architecture

This is a Go CLI tool for generating HTML invoices from YAML configuration files.

### Core Components

- **main.go** - CLI entry point, parses config/invoice YAML files, fetches exchange rates, calculates totals, and renders the HTML template
- **rate.go** - Fetches NOK/EUR exchange rates from Norges Bank API, finds the rate on the 15th of the invoice month (falls back to 14th/16th if unavailable)
- **invoice.templ** - Go text/template HTML template (embedded via `//go:embed`)

### Data Flow

1. Parse `config.yaml` (company/bank details) and `invoice.yaml` (services, customer, on-call amount)
2. Fetch 30-day exchange rate history from Norges Bank
3. Convert on-call compensation from NOK to EUR using mid-month rate
4. Calculate service totals and render HTML to stdout

### Key Types

- `Config` - Company and bank information
- `Invoice` - Customer, services list, dates, on-call amount in NOK
- `Service` - Individual billable item with quantity and price
