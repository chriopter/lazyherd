.PHONY: build test lint install

build:
	go build -o lazyherd .

test:
	go test ./...

lint:
	test -z "$$(gofmt -l .)" && go vet ./...

install:
	go install .
