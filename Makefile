.PHONY: build run test migrate-up migrate-down migrate-create clean help

BINARY=letterbox
BUILD_DIR=bin

## help: Show this help
help:
	@echo "letterbox - IMAP-to-REST facade"
	@echo ""
	@echo "Usage:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'

## build: Build the binary
build:
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/letterbox

## run: Run the server
run: build
	./$(BUILD_DIR)/$(BINARY)

## test: Run tests
test:
	go test -v ./...

## migrate-up: Run all migrations
migrate-up:
	@echo "Migrations not yet configured"

## migrate-down: Rollback last migration
migrate-down:
	@echo "Migrations not yet configured"

## migrate-create: Create new migration (usage: make migrate-create name=create_users)
migrate-create:
	@echo "Migrations not yet configured"

## clean: Remove build artifacts
clean:
	rm -rf $(BUILD_DIR)

## docker-up: Start postgres + minio
docker-up:
	docker-compose up -d

## docker-down: Stop containers
docker-down:
	docker-compose down
