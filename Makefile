APP_NAME := portscout
BIN_DIR := bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build clean run install uninstall

build:
	@mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(APP_NAME) .

run: build
	$(BIN_DIR)/$(APP_NAME)

install: build
	cp $(BIN_DIR)/$(APP_NAME) /usr/local/bin/$(APP_NAME)

uninstall:
	rm -f /usr/local/bin/$(APP_NAME)

clean:
	rm -rf $(BIN_DIR)
