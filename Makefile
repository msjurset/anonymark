.PHONY: build test clean install

BINARY_NAME = anonymark
BUILD_DIR = bin

build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/anonymark

test:
	go test -v ./...

clean:
	rm -rf $(BUILD_DIR)

install: build
	cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
