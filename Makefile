.PHONY: setup up down demo test build integration
setup:
	python3 scripts/setup.py
up:
	docker compose up --build -d
down:
	docker compose down
demo:
	python3 scripts/demo.py
test:
	go test -race ./...
build:
	go build ./...
	cd web && npm ci && npm run build
integration:
	docker compose --profile test run --build --rm integration
