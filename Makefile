SHELL := /bin/bash
GO ?= go
CONFIG ?= config.json
NPM ?= npm
export npm_config_cache := $(CURDIR)/.cache/npm

# Cache hanya di proyek; tidak memasang binary atau mengubah konfigurasi global.
export GOCACHE := $(CURDIR)/.cache/build
export GOMODCACHE := $(CURDIR)/.cache/mod
export GOPATH := $(CURDIR)/.cache/go
export GOTMPDIR := $(CURDIR)/.cache/tmp

.PHONY: help cache build build-go frontend-install frontend-build frontend-check frontend-test frontend-dev serve test race vet check recovery-test recovery-smoke

help:
	@echo 'make build | build-go | frontend-dev | serve | test | race | vet | check'
	@echo 'Go 1.26.8; build UI memerlukan Node 22.18+ (disarankan Node 24 LTS) dan npm'
	@echo 'serve menggunakan CONFIG=config.json; build-go memakai aset dist yang tersedia'
	@echo 'recovery-test | recovery-smoke (binary harus sudah dibuild; fixture sintetis dipertahankan)'

cache:
	@mkdir -p "$(GOCACHE)" "$(GOMODCACHE)" "$(GOPATH)" "$(GOTMPDIR)"

frontend-install: cache
	cd frontend && $(NPM) ci --ignore-scripts --no-fund --no-audit

frontend-build: frontend-install
	cd frontend && $(NPM) run build

frontend-check: frontend-install
	cd frontend && $(NPM) run check

frontend-test: frontend-install
	cd frontend && $(NPM) test

frontend-dev:
	cd frontend && $(NPM) run dev

build: frontend-build
	$(MAKE) build-go

build-go: cache
	@mkdir -p bin
	$(GO) build -o bin/memona ./cmd/memona

serve: cache
	$(GO) run ./cmd/memona serve -config "$(CONFIG)"

test: cache
	$(GO) test ./...

race: cache
	$(GO) test -race ./...

vet: cache
	$(GO) vet ./...

recovery-test: cache
	python3 -B -m unittest discover -s scripts -p test_recovery.py -v

recovery-smoke: cache
	python3 -B scripts/recovery_smoke.py

check: test race vet frontend-check frontend-test recovery-test
