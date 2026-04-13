BINARY := mailbridge-mcp
VERSION := 1.0.0

.PHONY: build install clean

build:
	go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BINARY) ./cmd/mailbridge-mcp/

install: build
	cp $(BINARY) /usr/local/bin/

clean:
	rm -f $(BINARY)
