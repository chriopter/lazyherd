.PHONY: build test lint install

build:
	go build -o lazyherd .

test:
	go test ./...
	go test -race ./...

lint:
	test -z "$$(gofmt -l .)" && go vet ./...

install:
	go install .
