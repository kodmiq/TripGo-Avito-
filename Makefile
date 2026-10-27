.PHONY: generate migrate migrate-down migrate-reset run test docker-build

# Подхватываем безопасные значения, затем локальный .env от tripgoctl.
ENV_LOAD = set -a; [ -f .env.example ] && . ./.env.example; [ -f .env ] && . ./.env; set +a;

generate:
	go tool oapi-codegen \
		-generate types,chi-server \
		-package api \
		-include-operation-ids createTrip,getTrip,finishTrip,health,ready \
		-o internal/generated/api.gen.go \
		contracts/openapi/trip-service.openapi.yaml

migrate:
	$(ENV_LOAD) go tool goose -dir migrations postgres "$$DATABASE_URL" up

migrate-down:
	$(ENV_LOAD) go tool goose -dir migrations postgres "$$DATABASE_URL" down

migrate-reset:
	$(ENV_LOAD) go tool goose -dir migrations postgres "$$DATABASE_URL" down-to 0

run:
	go run ./cmd/service

test:
	go test -race -count=1 ./...

docker-build:
	docker build -f deploy/Dockerfile -t trip-service:lab1 .
