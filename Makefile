.DEFAULT_GOAL := build
SHELL := /bin/bash

# Empty values use the installer defaults or .install-local.json.
CLIENT_INSTALL_DIR ?=
BROWSER ?=
INSTALL_ARGS ?=

.PHONY: build install help FORCE

# Recursive goals keep colon names compatible with macOS GNU Make 3.81.
build:
	$(MAKE) client:build browser:build

install:
	$(MAKE) client:install browser:install

client\:build: FORCE
	bash scripts/build-client.sh

client\:install: FORCE
	$(MAKE) client:build
	bash scripts/install-client.sh "$(CLIENT_INSTALL_DIR)"

browser\:deps: browser/node_modules/.package-lock.json FORCE

browser/node_modules/.package-lock.json: browser/package.json browser/package-lock.json
	npm --prefix browser ci

browser\:build: FORCE
	$(MAKE) browser:build:chrome browser:build:firefox

browser\:build\:chrome: browser/node_modules/.package-lock.json FORCE
	npm --prefix browser run build

browser\:build\:firefox: browser/node_modules/.package-lock.json FORCE
	npm --prefix browser run build:firefox

# Edge loads the same Chromium build as Chrome.
browser\:build\:edge: FORCE
	$(MAKE) browser:build:chrome

browser\:install: FORCE
	node scripts/install-local.mjs $(if $(BROWSER),--browser "$(BROWSER)") $(INSTALL_ARGS)

FORCE:

help:
	@printf '%s\n' \
	  'make / make build        Build the client and Chrome/Firefox extensions' \
	  'make install             Build and install the client and configured extension' \
	  'make client:build        Build the native client and Native Messaging host' \
	  'make client:install      Build, install and register the native client' \
	  'make browser:build       Build Chrome and Firefox extensions' \
	  'make browser:build:chrome / browser:build:firefox / browser:build:edge' \
	  'make browser:install     Build and install the configured browser extension' \
	  'Options: CLIENT_INSTALL_DIR="...", BROWSER=chrome|firefox|edge, INSTALL_ARGS="..."' \
	  'Extension installation: see docs/install-local.md'
