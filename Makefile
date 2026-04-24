.PHONY: build run test clean

build:
	go build -o invoicers .

run:
	go run . serve

test:
	go test ./...

clean:
	rm -f invoicers
	rm -rf data
