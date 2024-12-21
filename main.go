package main

import (
	"flag"
	"fmt"
	"os"
	"text/template"

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
	Reference      string `yaml:"reference"`
	IBAN           string `yaml:"iban"`
	BIC            string `yaml:"bic"`
}

type Invoice struct {
	Name         string    `yaml:"name"`
	CustomerName string    `yaml:"customerName"`
	Services     []Service `yaml:"services"`
	DueDate      string    `yaml:"dueDate"`
	IssueDate    string    `yaml:"issueDate"`
}

type Service struct {
	Name        string  `yaml:"name"`
	Description string  `yaml:"description"`
	Quantity    int     `yaml:"quantity"`
	Price       float64 `yaml:"price"`
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

	fmt.Println("company name:", config.CompanyName)
	fmt.Println("invoice name:", invoice.Name)

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

	// TODO: automate invoice dates
	// document.Invoice.DueDate = time.Now().Format("DD/MM/YY")
	// document.Invoice.IssueDate = time.Now().Format("DD/MM/YY")

	// TODO: calculate total eur

	err = template.Execute(os.Stdout, document)
	if err != nil {
		fmt.Println("error executing template:", err.Error())
	}
}

func parseYAML(path string, out interface{}) {
	fmt.Println("path:", path)

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
