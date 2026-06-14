.PHONY: all fmt lint test build install

all: fmt lint test

fmt:
	golangci-lint fmt

lint:
	golangci-lint run

test:
	go test -race ./...

build:
	go build ./...

install:
	go install .
