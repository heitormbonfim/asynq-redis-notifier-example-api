.PHONY: help setup up down clean api worker send schedule bulk logs-redis logs-mailhog

help:
	@echo "Setup:"
	@echo "  make setup       Install Go dependencies"
	@echo "  make up          Start Redis + MailHog"
	@echo "  make down        Stop containers"
	@echo "  make clean       Stop + remove volumes"
	@echo ""
	@echo "Run:"
	@echo "  make api         Start the API server (port 8080)"
	@echo "  make worker      Start the worker"
	@echo ""
	@echo "Test endpoints:"
	@echo "  make send        Send an email immediately"
	@echo "  make schedule    Schedule an email for a specific time"
	@echo "  make bulk        Send bulk emails"
	@echo ""
	@echo "Logs:"
	@echo "  make logs-redis    View Redis logs"
	@echo "  make logs-mailhog  View MailHog logs"

setup:
	go mod download
	go mod tidy

up:
	podman-compose up -d

down:
	podman-compose down

clean:
	podman-compose down -v

api:
	go run ./cmd/api

worker:
	go run ./cmd/worker

send:
	./scripts/send-email.sh

schedule:
	./scripts/schedule-email.sh

bulk:
	./scripts/send-bulk.sh

logs-redis:
	podman-compose logs -f redis

logs-mailhog:
	podman-compose logs -f mailhog
