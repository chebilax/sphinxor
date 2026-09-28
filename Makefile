.PHONY: build test vet fmt-check hook-test check

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

hook-test:
	@sh scripts/test-commit-msg-hook.sh

check: fmt-check vet build test hook-test
