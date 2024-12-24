.PHONY: build run test clean

build:
	go build -o invoicers

run:
	go run . -config=./example/config.yaml -invoice=example/invoice.yaml > test.html
	open test.html

test:
	go test ./...

clean:
	rm -f invoicers
