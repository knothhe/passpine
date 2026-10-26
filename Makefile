.DEFAULT_GOAL := build
ifeq ($(OS),Windows_NT)
SHELL := cmd.exe
CLIENT_BUILD = powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/build-client.ps1
CLIENT_INSTALL = powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/install-client.ps1
CLIENT_UNINSTALL = powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/uninstall-client.ps1
else
SHELL := /bin/bash
CLIENT_BUILD = bash scripts/build-client.sh
CLIENT_INSTALL = bash scripts/install-client.sh
CLIENT_UNINSTALL = node scripts/client-installation.mjs uninstall
endif

# Desktop builds and installations are isolated development versions.
# GitHub Actions sets the desktop release channel after verifying its release tag.
# Browser extensions use one identity and output directory in every environment.
PASSPINE_BUILD_CHANNEL ?= dev
export PASSPINE_BUILD_CHANNEL

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

.PHONY: build install uninstall help release release-initial FORCE

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

client\:uninstall: FORCE
	$(CLIENT_UNINSTALL) "$(CLIENT_INSTALL_DIR)"

uninstall:
	$(MAKE) client:uninstall

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

browser\:zip: browser/node_modules/.modules.yaml FORCE
	pnpm --dir browser run zip

browser\:install: FORCE
	node scripts/install-local.mjs $(if $(BROWSER),--browser "$(BROWSER)") $(INSTALL_ARGS)

website\:deps: website/node_modules/.modules.yaml FORCE

website/node_modules/.modules.yaml: website/package.json website/pnpm-lock.yaml website/pnpm-workspace.yaml
	pnpm --dir website install --frozen-lockfile

website\:dev: website/node_modules/.modules.yaml FORCE
	pnpm --dir website run dev

website\:build: website/node_modules/.modules.yaml FORCE
	pnpm --dir website run build

website\:check: website/node_modules/.modules.yaml FORCE
	pnpm --dir website run typecheck
	pnpm --dir website run test
	pnpm --dir website run build

website\:preview: website/node_modules/.modules.yaml FORCE
	$(MAKE) website:build
	pnpm --dir website run preview

website\:sync: website/node_modules/.modules.yaml FORCE
	pnpm --dir website run sync:releases

homebrew\:sync: FORCE
	node scripts/sync-homebrew.mjs

website\:deploy: website/node_modules/.modules.yaml FORCE
	pnpm --dir website run deploy

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
