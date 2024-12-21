package main

import (
	"flag"
	"fmt"
	"os"
	"text/template"
	"time"

	"gopkg.in/yaml.v3"
)

type Document struct {
	Config  Config
	Invoice Invoice
}

type Config struct {
	CompanyName    string `yaml:"companyName"`
	CompanyAddress string `yaml:"companyAddress"`
	BankName       string `yaml:"bankName"`
	BankAddress    string `yaml:"bankAddress"`
	AccountName    string `yaml:"accountName"`
	IBAN           string `yaml:"iban"`
	BIC            string `yaml:"bic"`
}

type Invoice struct {
	Name         string    `yaml:"name"`
	CustomerName string    `yaml:"customerName"`
	Services     []Service `yaml:"services"`
	DueDate      string    `yaml:"dueDate"`
	IssueDate    string    `yaml:"issueDate"`
	TotalAmount  float64
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

	templateFile := "invoice.templ"
	template, err := template.New(templateFile).ParseFiles(templateFile)
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

	// TODO: calculate total eur

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
