package main

import (
	"log"
	"os"

	"github.com/stianfro/invoicers/internal/cli"
)

func main() {
	if err := cli.Run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}
