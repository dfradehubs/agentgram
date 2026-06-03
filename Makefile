.PHONY: help dev dev-docker api web install clean test lint \
       image-api image-web image-push-api image-push-web image-all \
       test-e2e test-e2e-staging swagger \
       bump-patch bump-minor bump-major version changelog

# =============================================================================
# Variables
# =============================================================================

REGISTRY  := ghcr.io/dfradehubs
COMMIT    := $(shell git rev-parse --short HEAD)
VERSION   := $(shell cat api/VERSION 2>/dev/null || echo "0.0.0")
API_IMAGE := $(REGISTRY)/agentgram-api
WEB_IMAGE := $(REGISTRY)/agentgram-web

# =============================================================================
# Development - Full Environment
# =============================================================================

dev: ## Start full dev environment (Docker api + Next.js web)
	@echo "Starting development environment..."
	@echo ""
	@echo "Step 1: Starting api services (Docker)..."
	cd api/docker && docker compose up -d api mock-agent
	@echo ""
	@echo "Services running:"
	@echo "  - API:        http://localhost:8080"
	@echo "  - Mock Agent: http://localhost:9000"
	@echo ""
	@echo "Step 2: Starting web (Next.js)..."
	@sleep 2
	cd web && npm run dev

dev-docker: ## Start full environment with Docker only (includes test-frontend)
	@echo "Starting Docker environment..."
	cd api/docker && docker compose up -d
	@echo ""
	@echo "Services running:"
	@echo "  - API:           http://localhost:8080"
	@echo "  - Mock Agent:    http://localhost:9000"
	@echo "  - Test Frontend: http://localhost:3001"

dev-api: ## Start api with Docker (background)
	@echo "Starting api services..."
	cd api/docker && docker compose up -d api mock-agent
	@echo "API ready at http://localhost:8080"

dev-web: ## Start web development server
	@echo "Starting web..."
	cd web && npm run dev

# =============================================================================
# Individual Services
# =============================================================================

api: ## Run api in development mode (requires Go)
	cd api && make dev

api-build: ## Build api binary
	cd api && go build -o bin/server ./cmd/server

web: ## Run web in development mode
	cd web && npm run dev

web-build: ## Build web for production
	cd web && npm run build

swagger: ## Generate Swagger/OpenAPI docs for the API
	cd api && swag init -g cmd/server/main.go -o docs/swagger --parseDependency --parseInternal

# =============================================================================
# Docker Commands
# =============================================================================

docker-up: ## Start all Docker services
	cd api/docker && docker compose up -d

docker-down: ## Stop all Docker services
	cd api/docker && docker compose down

docker-logs: ## Show Docker logs (follow)
	cd api/docker && docker compose logs -f

docker-restart: ## Restart Docker services
	cd api/docker && docker compose restart

docker-rebuild: ## Rebuild and restart Docker services
	cd api/docker && docker compose up -d --build

# =============================================================================
# Container Images (linux/amd64)
# =============================================================================

image-api: ## Build api image for amd64
	docker build --platform linux/amd64 \
		-t $(API_IMAGE):$(VERSION) \
		-t $(API_IMAGE):$(COMMIT) \
		-t $(API_IMAGE):latest \
		api/

image-web: ## Build web image for amd64
	docker build --platform linux/amd64 \
		-t $(WEB_IMAGE):$(VERSION) \
		-t $(WEB_IMAGE):$(COMMIT) \
		-t $(WEB_IMAGE):latest \
		web/

image-push-api: ## Push api image to registry
	docker push $(API_IMAGE):$(VERSION)
	docker push $(API_IMAGE):$(COMMIT)
	docker push $(API_IMAGE):latest

image-push-web: ## Push web image to registry
	docker push $(WEB_IMAGE):$(VERSION)
	docker push $(WEB_IMAGE):$(COMMIT)
	docker push $(WEB_IMAGE):latest

image-all: image-api image-web image-push-api image-push-web ## Build and push all images

# =============================================================================
# Setup & Installation
# =============================================================================

install: ## Install all dependencies
	@echo "Installing web dependencies..."
	cd web && npm install
	@echo ""
	@echo "Downloading Go modules..."
	cd api && go mod download
	@echo ""
	@echo "Done! Run 'make dev' to start development."

setup: install ## Alias for install

# =============================================================================
# Testing & Quality
# =============================================================================

test: ## Run all tests
	@echo "Running api tests..."
	cd api && go test ./...
	@echo ""
	@echo "Running web lint..."
	cd web && npm run lint

test-api: ## Run api tests only
	cd api && go test ./... -v

test-coverage: ## Run api tests with coverage
	cd api && go test ./... -coverprofile=coverage.out
	cd api && go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: api/coverage.html"

test-e2e: ## Run E2E tests against local docker environment
	cd web && npx playwright test

test-e2e-staging: ## Run E2E tests against staging (prompts for Keycloak credentials)
	@read -p "Keycloak user: " KC_USER && \
	read -sp "Keycloak password: " KC_PASS && echo && \
	cd web && STAGING=1 KC_USER="$$KC_USER" KC_PASS="$$KC_PASS" npx playwright test


lint: ## Run linters
	cd api && make lint || true
	cd web && npm run lint || true

lint-fix: ## Fix linting issues
	cd api && make lint-fix || true

# =============================================================================
# Cleanup
# =============================================================================

clean: ## Clean build artifacts and dependencies
	@echo "Cleaning up..."
	cd api && rm -rf bin/ coverage.out coverage.html
	cd web && rm -rf .next/ out/
	@echo "Done!"

clean-all: clean ## Clean everything including node_modules
	cd web && rm -rf node_modules/
	@echo "Run 'make install' to reinstall dependencies."

# =============================================================================
# Versioning & Changelog
# =============================================================================

version: ## Show current version
	@echo "$(VERSION)"

changelog: ## Generate changelog entry from commits since last tag
	@LAST_TAG=$$(git describe --tags --abbrev=0 2>/dev/null || echo ""); \
	if [ -z "$$LAST_TAG" ]; then \
		RANGE="HEAD"; \
	else \
		RANGE="$$LAST_TAG..HEAD"; \
	fi; \
	echo "## [$(VERSION)] - $$(date +%Y-%m-%d)"; \
	echo ""; \
	FEATS=$$(git log $$RANGE --oneline --no-merges --grep="^feat:" --format="- %s" | sed 's/^- feat: /- /'); \
	if [ -n "$$FEATS" ]; then \
		echo "### Features"; \
		echo ""; \
		echo "$$FEATS"; \
		echo ""; \
	fi; \
	FIXES=$$(git log $$RANGE --oneline --no-merges --grep="^fix:" --format="- %s" | sed 's/^- fix: /- /'); \
	if [ -n "$$FIXES" ]; then \
		echo "### Fixes"; \
		echo ""; \
		echo "$$FIXES"; \
		echo ""; \
	fi; \
	REFACTORS=$$(git log $$RANGE --oneline --no-merges --grep="^refactor:" --format="- %s" | sed 's/^- refactor: /- /'); \
	if [ -n "$$REFACTORS" ]; then \
		echo "### Refactors"; \
		echo ""; \
		echo "$$REFACTORS"; \
		echo ""; \
	fi; \
	PERFS=$$(git log $$RANGE --oneline --no-merges --grep="^perf:" --format="- %s" | sed 's/^- perf: /- /'); \
	if [ -n "$$PERFS" ]; then \
		echo "### Performance"; \
		echo ""; \
		echo "$$PERFS"; \
		echo ""; \
	fi; \
	OTHERS=$$(git log $$RANGE --oneline --no-merges --invert-grep --grep="^feat:" --grep="^fix:" --grep="^refactor:" --grep="^perf:" --grep="^bump:" --format="- %s"); \
	if [ -n "$$OTHERS" ]; then \
		echo "### Other"; \
		echo ""; \
		echo "$$OTHERS"; \
		echo ""; \
	fi

define bump_version
	@OLD_VERSION=$$(cat api/VERSION | tr -d '[:space:]'); \
	MAJOR=$$(echo $$OLD_VERSION | cut -d. -f1); \
	MINOR=$$(echo $$OLD_VERSION | cut -d. -f2); \
	PATCH=$$(echo $$OLD_VERSION | cut -d. -f3); \
	case "$(1)" in \
		major) MAJOR=$$((MAJOR + 1)); MINOR=0; PATCH=0 ;; \
		minor) MAJOR=$$MAJOR; MINOR=$$((MINOR + 1)); PATCH=0 ;; \
		patch) MAJOR=$$MAJOR; MINOR=$$MINOR; PATCH=$$((PATCH + 1)) ;; \
	esac; \
	NEW_VERSION="$$MAJOR.$$MINOR.$$PATCH"; \
	echo "Bumping version: $$OLD_VERSION -> $$NEW_VERSION"; \
	echo "$$NEW_VERSION" > api/VERSION; \
	cd web && npm version $$NEW_VERSION --no-git-tag-version --allow-same-version; \
	cd ..; \
	LAST_TAG=$$(git describe --tags --abbrev=0 2>/dev/null || echo ""); \
	if [ -z "$$LAST_TAG" ]; then \
		RANGE="HEAD"; \
	else \
		RANGE="$$LAST_TAG..HEAD"; \
	fi; \
	ENTRY="## [$$NEW_VERSION] - $$(date +%Y-%m-%d)"; \
	ENTRY="$$ENTRY\n"; \
	FEATS=$$(git log $$RANGE --oneline --no-merges --grep="^feat:" --format="- %s" | sed 's/^- feat: /- /'); \
	if [ -n "$$FEATS" ]; then \
		ENTRY="$$ENTRY\n### Features\n\n$$FEATS\n"; \
	fi; \
	FIXES=$$(git log $$RANGE --oneline --no-merges --grep="^fix:" --format="- %s" | sed 's/^- fix: /- /'); \
	if [ -n "$$FIXES" ]; then \
		ENTRY="$$ENTRY\n### Fixes\n\n$$FIXES\n"; \
	fi; \
	REFACTORS=$$(git log $$RANGE --oneline --no-merges --grep="^refactor:" --format="- %s" | sed 's/^- refactor: /- /'); \
	if [ -n "$$REFACTORS" ]; then \
		ENTRY="$$ENTRY\n### Refactors\n\n$$REFACTORS\n"; \
	fi; \
	PERFS=$$(git log $$RANGE --oneline --no-merges --grep="^perf:" --format="- %s" | sed 's/^- perf: /- /'); \
	if [ -n "$$PERFS" ]; then \
		ENTRY="$$ENTRY\n### Performance\n\n$$PERFS\n"; \
	fi; \
	OTHERS=$$(git log $$RANGE --oneline --no-merges --invert-grep --grep="^feat:" --grep="^fix:" --grep="^refactor:" --grep="^perf:" --grep="^bump:" --format="- %s"); \
	if [ -n "$$OTHERS" ]; then \
		ENTRY="$$ENTRY\n### Other\n\n$$OTHERS\n"; \
	fi; \
	HEADER=$$(head -6 CHANGELOG.md); \
	BODY=$$(tail -n +7 CHANGELOG.md); \
	printf "%s\n\n%b\n\n---\n\n%s\n" "$$HEADER" "$$ENTRY" "$$BODY" > CHANGELOG.md; \
	echo ""; \
	echo "Updated:"; \
	echo "  - api/VERSION          -> $$NEW_VERSION"; \
	echo "  - web/package.json     -> $$NEW_VERSION"; \
	echo "  - CHANGELOG.md         -> new entry added"; \
	echo ""; \
	echo "Next steps:"; \
	echo "  git add api/VERSION web/package.json CHANGELOG.md"; \
	echo "  git commit -m \"bump: v$$NEW_VERSION\""; \
	echo "  git tag $$NEW_VERSION"
endef

bump-patch: ## Bump patch version (1.0.0 -> 1.0.1), update changelog, print tag command
	$(call bump_version,patch)

bump-minor: ## Bump minor version (1.0.0 -> 1.1.0), update changelog, print tag command
	$(call bump_version,minor)

bump-major: ## Bump major version (1.0.0 -> 2.0.0), update changelog, print tag command
	$(call bump_version,major)

# =============================================================================
# Help
# =============================================================================

help: ## Show this help
	@echo "Agentgram - Development Commands"
	@echo ""
	@echo "Quick Start:"
	@echo "  make install    # Install dependencies"
	@echo "  make dev        # Start full development environment"
	@echo ""
	@echo "Available targets:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
