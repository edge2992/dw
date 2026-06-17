.PHONY: all fmt lint test cover build install

all: fmt lint test

fmt:
	golangci-lint fmt

lint:
	golangci-lint run

test:
	go test -race ./...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

build:
	go build ./...

install:
	go install .
