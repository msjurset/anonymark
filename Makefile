.PHONY: build test clean install deploy

BINARY_NAME = anonymark
BUILD_DIR = bin

build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/anonymark

test:
	go test -v ./...

clean:
	rm -rf $(BUILD_DIR)

install: build
	install -d ~/.local/bin
	install -m 755 $(BUILD_DIR)/$(BINARY_NAME) ~/.local/bin/$(BINARY_NAME)

deploy: build install
