.PHONY: test lint nocgo generate breaking check clean

# What CI runs on every PR and push to main.
test:
	go test -race ./...

lint:
	golangci-lint run
	buf lint

# Enforces ADR-0017: no cgo without an ADR. A cgo dependency usually arrives
# transitively and unnoticed; this build is what makes it fail loudly instead.
nocgo:
	CGO_ENABLED=0 go build ./...

generate:
	buf generate

# The proto is a published contract; this is the guard that keeps it honest
# once anything depends on it.
breaking:
	buf breaking --against '.git#branch=main'

check: lint nocgo test

clean:
	rm -rf gen
