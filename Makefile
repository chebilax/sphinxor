.PHONY: build test vet fmt-check check

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

fmt-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed on:"; echo "$$unformatted"; exit 1; \
	fi

check: fmt-check vet build test
