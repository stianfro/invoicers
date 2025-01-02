package main

import (
	"embed"
	"flag"
	"fmt"
	"os"
	"strconv"
	"text/template"
	"time"

	"gopkg.in/yaml.v3"
)

//go:embed invoice.templ
var templateFS embed.FS

type Document struct {
	Config  Config
	Invoice Invoice
}

type Config struct {
	CompanyName    string   `yaml:"companyName"`
	CompanyAddress []string `yaml:"companyAddress"`
	BankName       string   `yaml:"bankName"`
	BankAddress    []string `yaml:"bankAddress"`
	AccountName    string   `yaml:"accountName"`
	IBAN           string   `yaml:"iban"`
	BIC            string   `yaml:"bic"`
}

type Invoice struct {
	Name         string    `yaml:"name"`
	CustomerName string    `yaml:"customerName"`
	Services     []Service `yaml:"services"`
	DueDate      string    `yaml:"dueDate"`
	IssueDate    string    `yaml:"issueDate"`
	OnCallNOK    int       `yaml:"onCallNOK"`
	TotalAmount  string
}

type Service struct {
	Name        string  `yaml:"name"`
	Description string  `yaml:"description"`
	Quantity    int     `yaml:"quantity"`
	Price       float64 `yaml:"price"`
	PriceTotal  float64
}

func main() {
	var configPath, invoicePath string

	flag.StringVar(&configPath, "config", "", "path to a config.yaml (required)")
	flag.StringVar(&invoicePath, "invoice", "", "path to an invoice.yaml (required)")
	flag.Parse()

	if configPath == "" || invoicePath == "" {
		fmt.Println("required flags missing, see usage with -h")
		os.Exit(1)
	}

	config := Config{}
	invoice := Invoice{}

	parseYAML(configPath, &config)
	parseYAML(invoicePath, &invoice)

	template, err := template.New("invoice.templ").ParseFS(templateFS, "invoice.templ")
	if err != nil {
		fmt.Println("error parsing template:", err.Error())
		os.Exit(1)
	}

	document := Document{
		Config:  config,
		Invoice: invoice,
	}

	t := time.Now()
	date := fmt.Sprintf("%d %s %d", t.Day(), t.Month().String(), t.Year())

	if document.Invoice.DueDate == "" {
		document.Invoice.DueDate = date
	}
	if document.Invoice.IssueDate == "" {
		document.Invoice.IssueDate = date
	}

	rates, err := GetDailyRates(30)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error getting exchange rates: %s", err.Error())
		os.Exit(1)
	}

	rate, err := FindRateOn15th(rates)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error getting rate on 15th: %s", err.Error())
		os.Exit(1)
	}

	rateFloat, err := strconv.ParseFloat(rate, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error converting rate to float64: %s", err.Error())
		os.Exit(1)
	}

	onCallEUR := float64(document.Invoice.OnCallNOK) / rateFloat
	onCallEURShort := fmt.Sprintf("%.2f", onCallEUR)

	onCallEurShortFloat, err := strconv.ParseFloat(onCallEURShort, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error converting to float64: %s", err.Error())
	}

	onCallService := Service{
		Name:        "On call",
		Description: fmt.Sprintf("NOK %d converted using NOK/EUR rate %s", document.Invoice.OnCallNOK, rate),
		Quantity:    1,
		Price:       onCallEurShortFloat,
	}

	document.Invoice.Services = append(document.Invoice.Services, onCallService)

	var totalRaw float64

	for i, item := range document.Invoice.Services {
		item.PriceTotal = item.Price * float64(item.Quantity)

		totalRaw += item.PriceTotal
		document.Invoice.Services[i].PriceTotal = item.PriceTotal
	}

	document.Invoice.TotalAmount = fmt.Sprintf("%.2f", totalRaw)

	err = template.Execute(os.Stdout, document)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error executing template: %s", err.Error())
		os.Exit(1)
	}
}

func parseYAML(path string, out interface{}) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("error reading file:", err.Error())
		os.Exit(1)
	}

	yamlerr := yaml.Unmarshal(data, out)
	if yamlerr != nil {
		fmt.Println("error parsing yaml:", err.Error())
		os.Exit(1)
	}
}
