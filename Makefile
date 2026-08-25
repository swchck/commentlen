.PHONY: all test race bench cover fuzz lint build custom clean

all: test build

test:
	go test ./...

race:
	go test -race ./...

bench:
	go test ./analyzer/ -run XXX -bench . -benchmem

cover:
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out | tail -1

# the prefilter is the one place a bug means silently missing diagnostics
fuzz:
	go test ./analyzer/ -run FuzzPrefilter -fuzz FuzzPrefilter -fuzztime 60s

lint:
	go vet ./...

# standalone binary
build:
	go build -o bin/commentlen ./cmd/commentlen

# custom golangci-lint binary with the plugin compiled in
custom:
	golangci-lint custom

clean:
	rm -rf bin coverage.out
