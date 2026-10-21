.DEFAULT_GOAL := build
ifeq ($(OS),Windows_NT)
SHELL := cmd.exe
CLIENT_BUILD = powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/build-client.ps1
CLIENT_INSTALL = powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/install-client.ps1
else
SHELL := /bin/bash
CLIENT_BUILD = bash scripts/build-client.sh
CLIENT_INSTALL = bash scripts/install-client.sh
endif

# Empty values use the installer defaults or .install-local.json.
CLIENT_INSTALL_DIR ?=
# Desktop sessions use BROWSER for a launcher command, not an extension target.
ifeq ($(origin BROWSER),environment)
BROWSER :=
endif
BROWSER ?=
INSTALL_ARGS ?=
VERSION_BUMP ?= patch
VERSION_VALUE ?=
RELEASE_FLAGS ?=

.PHONY: build install help release release-initial FORCE

# Recursive goals keep colon names compatible with macOS GNU Make 3.81.
build:
	$(MAKE) client:build browser:build

install:
	$(MAKE) client:install browser:install

client\:build: FORCE
	$(CLIENT_BUILD)

client\:install: FORCE
	$(MAKE) client:build
	$(CLIENT_INSTALL) "$(CLIENT_INSTALL_DIR)"

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

release-initial:
	node scripts/release.mjs initial $(RELEASE_FLAGS)

FORCE:

help:
	@node scripts/help.mjs
