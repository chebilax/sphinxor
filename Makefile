.PHONY: build test vet fmt-check hook-test cerbos-notice check

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

# The real-engine Cerbos tests (ADR 0009 §5) skip when the cerbos CLI is not
# on PATH, and a skip is invisible in a green run. Say so, last, where it is
# read.
cerbos-notice:
	@command -v cerbos >/dev/null 2>&1 || { \
		echo ""; \
		echo "NOTICE: cerbos CLI not found on PATH -- the real-engine policy tests were SKIPPED."; \
		echo "        This check passed without validating exported policies against Cerbos (ADR 0009 §5)."; \
		echo "        Install it as CONTRIBUTING.md's Setup describes, then run make check again."; \
	}

check: fmt-check vet build test hook-test cerbos-notice
