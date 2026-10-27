.PHONY: generate migrate migrate-down migrate-reset run test test-integration lint docker-build

# Подхватываем безопасные значения, затем локальный .env от tripgoctl.
ENV_LOAD = set -a; [ -f .env.example ] && . ./.env.example; [ -f .env ] && . ./.env; set +a;

generate:
	go generate ./...

migrate:
	$(ENV_LOAD) go tool goose -dir migrations postgres "$$DATABASE_URL" up

migrate-down:
	$(ENV_LOAD) go tool goose -dir migrations postgres "$$DATABASE_URL" down

migrate-reset:
	$(ENV_LOAD) go tool goose -dir migrations postgres "$$DATABASE_URL" down-to 0

run:
	go run ./cmd/service

test:
	go test -race -count=1 -coverprofile=coverage.out -coverpkg=./internal/domain/...,./internal/usecase/... ./...
	go tool cover -func=coverage.out

test-integration:
	go test -race -count=1 -tags=integration ./internal/repository/...

lint:
	golangci-lint run ./...

docker-build:
	docker build -f deploy/Dockerfile -t trip-service:lab1 .
