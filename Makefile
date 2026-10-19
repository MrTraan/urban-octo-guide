.PHONY: proto build run test test-unit lint up down demo load cli

proto: ## Regenerate Go code from proto/ (needs protoc, protoc-gen-go, protoc-gen-go-grpc)
	protoc -I proto \
		--go_out=gen --go_opt=paths=source_relative \
		--go-grpc_out=gen --go-grpc_opt=paths=source_relative \
		proto/player/v1/player.proto

build: ## Build server and CLI into ./bin
	go build -o bin/server ./cmd/server
	go build -o bin/playerctl ./cmd/playerctl

run: ## Run the server locally (expects Mongo at MONGO_URI, default localhost:27017)
	go run ./cmd/server

test: ## Run all tests (integration tests need Docker)
	go test ./... -count=1

test-unit: ## Run only tests that do not need Docker
	go test ./... -short -count=1

lint: ## gofmt + go vet (+ golangci-lint when installed)
	@test -z "$$(gofmt -l . | grep -v '^gen/')" || (gofmt -l . | grep -v '^gen/'; echo 'gofmt: files need formatting'; exit 1)
	go vet ./...
	@command -v golangci-lint >/dev/null && golangci-lint run ./... || echo "golangci-lint not installed, skipped"

up: ## Start Mongo + service + Prometheus + Grafana with docker compose
	docker compose up --build -d
	@echo "gRPC: localhost:8080  metrics: http://localhost:9100/metrics  Grafana: http://localhost:3000/d/player-service"

down: ## Stop docker compose services
	docker compose down -v

demo: ## Run the end-to-end demo scenario against a running server
	./scripts/demo.sh

load: ## Send traffic for 60s so the Grafana dashboard has data
	./scripts/load.sh 60

cli: ## Build the CLI only
	go build -o bin/playerctl ./cmd/playerctl
