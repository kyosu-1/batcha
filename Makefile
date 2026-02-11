.PHONY: build test lint clean install

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-s -w -X github.com/kyosu-1/batcha.Version=$(VERSION)"

build:
	go build $(LDFLAGS) -o batcha ./cmd/batcha

install:
	go install $(LDFLAGS) ./cmd/batcha

test:
	go test -v -race ./...

lint:
	golangci-lint run

clean:
	rm -f batcha
	rm -rf dist/
