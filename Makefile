BIN := itunes-detangler
MODULE := github.com/ianchesal/itunes-detangler
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: all build test lint clean install

all: build

build:
	go build $(LDFLAGS) -o $(BIN) .

test:
	go test ./...

test-verbose:
	go test -v ./...

test-race:
	go test -race ./...

lint:
	go vet ./...

clean:
	rm -f $(BIN)

install:
	go install $(LDFLAGS) .
