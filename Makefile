.PHONY: build run test migrate-up migrate-down migrate-create clean help sqlc

BINARY=letterbox
BUILD_DIR=bin
MIGRATIONS_DIR=migrations
DATABASE_URL ?= postgres://letterbox:letterbox@localhost:5434/letterbox?sslmode=disable

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
	migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" up

## migrate-down: Rollback last migration
migrate-down:
	migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" down 1

## migrate-create: Create new migration (usage: make migrate-create name=create_users)
migrate-create:
	@test -n "$(name)" || (echo "Usage: make migrate-create name=<migration_name>" && exit 1)
	migrate create -ext sql -dir $(MIGRATIONS_DIR) -seq $(name)

## sqlc: Generate sqlc code
sqlc:
	sqlc generate

## clean: Remove build artifacts
clean:
	rm -rf $(BUILD_DIR)

## docker-up: Start postgres + minio
docker-up:
	docker-compose up -d

## docker-down: Stop containers
docker-down:
	docker-compose down
