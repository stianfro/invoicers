package main

import (
	"flag"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Company Company `yaml:"company"`
}

// Company is the sender of an invoice.
type Company struct {
	Name string `yaml:"name"`

	// Address is the physical address of a company.
	Address string `yaml:"address"`
}

type Invoice struct {
	Name     string    `yaml:"name"`
	Customer Customer  `yaml:"customer"`
	Services []Service `yaml:"services"`
	Due      string    `yaml:"due"`
}

// Customer is the receiver of an invoice.
type Customer struct {
	Name  string `yaml:"name"`
	Email string `yaml:"email"`
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

	fmt.Println("company name:", config.Company.Name)
	fmt.Println("invoice name:", invoice.Name)
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
