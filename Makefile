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
	bash scripts/coverage-badge.sh coverage.out .github/badges/coverage.svg

build:
	go build ./...

install:
	go install .
