BIN := bin/claude-search
TAGS := sqlite_fts5

.PHONY: build install test clean

build:
	CGO_ENABLED=1 go build -tags "$(TAGS)" -o $(BIN) ./cmd/claude-search

install: build
	cp $(BIN) $(HOME)/.local/bin/claude-search

test:
	CGO_ENABLED=1 go test -tags "$(TAGS)" ./...

clean:
	rm -f $(BIN)
