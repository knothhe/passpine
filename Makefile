.DEFAULT_GOAL := build
SHELL := /bin/bash

# Empty values use the installer defaults or .install-local.json.
CLIENT_INSTALL_DIR ?=
BROWSER ?=
INSTALL_ARGS ?=
VERSION_BUMP ?= patch
VERSION_VALUE ?=
RELEASE_FLAGS ?=

.PHONY: build install help release FORCE

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

browser\:deps: browser/node_modules/.modules.yaml FORCE

browser/node_modules/.modules.yaml: browser/package.json browser/pnpm-lock.yaml
	pnpm --dir browser install --frozen-lockfile

browser\:build: FORCE
	$(MAKE) browser:build:chrome browser:build:firefox

browser\:build\:chrome: browser/node_modules/.modules.yaml FORCE
	pnpm --dir browser run build

browser\:build\:firefox: browser/node_modules/.modules.yaml FORCE
	pnpm --dir browser run build:firefox

# Edge loads the same Chromium build as Chrome.
browser\:build\:edge: FORCE
	$(MAKE) browser:build:chrome

browser\:install: FORCE
	node scripts/install-local.mjs $(if $(BROWSER),--browser "$(BROWSER)") $(INSTALL_ARGS)

version\:show: FORCE
	node scripts/version.mjs show

version\:check: FORCE
	node scripts/version.mjs check

version\:patch: FORCE
	node scripts/version.mjs patch

version\:minor: FORCE
	node scripts/version.mjs minor

version\:major: FORCE
	node scripts/version.mjs major

version\:set: FORCE
	node scripts/version.mjs set "$(VERSION_VALUE)"

release:
	node scripts/release.mjs $(VERSION_BUMP) $(RELEASE_FLAGS)

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
	  'make version:show / version:check / version:patch / version:minor / version:major' \
	  'make version:set VERSION_VALUE=0.2.0  Update desktop and browser version sources' \
	  'make release VERSION_BUMP=patch     Preview release; RELEASE_FLAGS=--apply executes' \
	  'Options: CLIENT_INSTALL_DIR="...", BROWSER=chrome|firefox|edge, INSTALL_ARGS="..."' \
	  'Extension installation: see docs/install-local.md'
