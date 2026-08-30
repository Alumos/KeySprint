.PHONY: dev backend frontend build test

dev:
	@echo "Run 'make backend' and 'make frontend' in two terminals"

backend:
	cd backend && ADDR=:8080 PUBLIC_DIR=../frontend/dist go run .

frontend:
	cd frontend && npm run dev

build:
	cd frontend && npm run build
	cd backend && go build ./...

test:
	cd backend && go test ./...
	cd frontend && npm run build
