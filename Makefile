.PHONY: test lint vet generate breaking check clean

# What CI runs on every PR and push to main.
test:
	go test -race ./...

lint:
	buf lint

vet:
	go vet ./...

generate:
	buf generate

# The proto is a published contract; this is the guard that keeps it honest
# once anything depends on it.
breaking:
	buf breaking --against '.git#branch=main'

check: lint vet test

clean:
	rm -rf gen
