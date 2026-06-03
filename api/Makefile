.PHONY: dev test lint build docker-up docker-down clean help

# Variables
BINARY_NAME=server
BUILD_DIR=bin
MAIN_PATH=cmd/server/main.go

# Desarrollo
dev: ## Ejecutar en modo desarrollo
	go run $(MAIN_PATH)

dev-watch: ## Ejecutar con hot reload (requiere air)
	air -c .air.toml

# Tests
test: ## Ejecutar tests unitarios
	go test ./... -v

test-coverage: ## Ejecutar tests con cobertura
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

test-integration: ## Ejecutar tests de integración
	go test ./test/integration/... -v -tags=integration

# Linting
lint: ## Ejecutar linter
	golangci-lint run

lint-fix: ## Ejecutar linter con fix automático
	golangci-lint run --fix

# Build
build: ## Compilar binario
	go build -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)

build-linux: ## Compilar para Linux
	GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PATH)

# Docker
docker-build: ## Construir imagen Docker
	docker build -t agentgram-api -f docker/Dockerfile .

docker-up: ## Levantar entorno Docker
	docker-compose -f docker/docker-compose.yml up -d

docker-down: ## Detener entorno Docker
	docker-compose -f docker/docker-compose.yml down

docker-logs: ## Ver logs de Docker
	docker-compose -f docker/docker-compose.yml logs -f

# Dependencias
deps: ## Descargar dependencias
	go mod download

deps-tidy: ## Limpiar dependencias
	go mod tidy

deps-update: ## Actualizar dependencias
	go get -u ./...
	go mod tidy

# Generación
generate: ## Generar código (mocks, etc.)
	go generate ./...

generate-mocks: ## Generar mocks
	./scripts/generate-mocks.sh

# Limpieza
clean: ## Limpiar artefactos de build
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

# Ayuda
help: ## Mostrar ayuda
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
