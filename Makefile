APP      := coffeesos-api
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
DEPLOY_HOST ?= hector_vps
DEPLOY_DIR  ?= /root/coffeesos

.PHONY: run build test vet sqlc migrate migrate-down seed tidy deploy logs

run:            ## run the API locally (reads .env)
	go run ./cmd/api serve

build:          ## build a static binary into bin/
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o bin/api ./cmd/api

test:
	go test ./...

vet:
	go vet ./...

sqlc:           ## regenerate internal/db from SQL (needs sqlc)
	sqlc generate

migrate:
	go run ./cmd/api migrate

migrate-down:
	go run ./cmd/api migrate down

seed:
	go run ./cmd/api seed

tidy:
	go mod tidy

deploy:         ## rsync + docker compose up on the VPS
	DEPLOY_HOST=$(DEPLOY_HOST) DEPLOY_DIR=$(DEPLOY_DIR) VERSION=$(VERSION) ./scripts/deploy.sh

logs:           ## tail API logs on the VPS
	ssh $(DEPLOY_HOST) 'cd $(DEPLOY_DIR) && docker compose logs -f --tail=100 api'
