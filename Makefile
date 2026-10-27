PREFIX ?= $(HOME)/.local
BIN_DIR ?= $(PREFIX)/bin
INSTALL_DIR ?= $(PREFIX)/share/check-now

.PHONY: build test test-plugins install clean

build:
	mkdir -p dist
	go build -o dist/check-now ./cmd/check-now
	rm -rf dist/plugins
	cp -R plugins dist/plugins

test: test-plugins
	go test ./...

test-plugins:
	./plugins/test.sh

install: build
	mkdir -p "$(BIN_DIR)" "$(INSTALL_DIR)"
	cp dist/check-now "$(BIN_DIR)/check-now"
	rm -rf "$(INSTALL_DIR)/plugins"
	cp -R dist/plugins "$(INSTALL_DIR)/plugins"
	cp check-now.sh check-now.fish "$(INSTALL_DIR)/"
	printf 'Installed check-now to %s\n' "$(BIN_DIR)/check-now"
	printf 'Plugins installed to %s\n' "$(INSTALL_DIR)/plugins"

clean:
	rm -rf dist
