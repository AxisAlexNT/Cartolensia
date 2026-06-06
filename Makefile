.RECIPEPREFIX := >
SHELL := /bin/bash

APP_NAME := cartolensia
WEBUI_DIR := webui
GOCACHE ?= /tmp/cartolensia-go-build

.PHONY: help check-tools backend-run backend-test webui-install webui-build compose-up compose-down smoke dev-db reset-dev-db

help:
> @printf 'Targets:\n'
> @printf '  make check-tools    Check local development tools\n'
> @printf '  make backend-test   Run Go tests\n'
> @printf '  make backend-run    Run the Go backend scaffold\n'
> @printf '  make webui-install  Install WebUI dependencies with npm\n'
> @printf '  make webui-build    Build the WebUI\n'
> @printf '  make compose-up     Start development PostgreSQL\n'
> @printf '  make compose-down   Stop development PostgreSQL\n'
> @printf '  make smoke          Run available smoke checks\n'

check-tools:
> @bash scripts/check-tools.sh

backend-test:
> GOCACHE=$(GOCACHE) go test ./...

backend-run:
> GOCACHE=$(GOCACHE) go run ./cmd/$(APP_NAME)

webui-install:
> npm --prefix $(WEBUI_DIR) install

webui-build:
> npm --prefix $(WEBUI_DIR) run build

compose-up:
> docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d

compose-down:
> docker compose -f docker-compose.yml -f docker-compose.dev.yml down

dev-db:
> bash scripts/dev-db.sh

reset-dev-db:
> bash scripts/reset-dev-db.sh

smoke:
> @bash scripts/smoke-test.sh
