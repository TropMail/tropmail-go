.PHONY: test lint fmt contract cover clean

test:
	go test ./...

lint:
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
	  echo "gofmt needed:" >&2; echo "$$unformatted" >&2; exit 1; \
	fi
	go vet ./...

fmt:
	gofmt -w .

contract:
	go test -run TestContract ./...

cover:
	go test -cover ./...

clean:
	go clean -testcache
