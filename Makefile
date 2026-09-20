.PHONY: build test lint link install

build:
	go build -o bin/lazyherd .

test:
	go test ./...
	go test -race ./...

lint:
	test -z "$$(gofmt -l .)" && go vet ./...

link: build
	herdr plugin link "$(CURDIR)"

install:
	go install .
