GO ?= go

SERVICES := api-gateway order-service tracking-service carrier-sync-service notification-service
SERVICE ?= api-gateway

.PHONY: run test lint help

run:
	@if [ ! -d services/$(SERVICE) ]; then \
		echo "Unknown service: $(SERVICE)"; \
		echo "Available services: $(SERVICES)"; \
		exit 1; \
	fi
	$(GO) run ./services/$(SERVICE)/cmd/server

test:
	@for svc in $(SERVICES); do \
		echo "== $$svc =="; \
		$(GO) test ./services/$$svc/...; \
	done

lint:
	@for svc in $(SERVICES); do \
		echo "== $$svc =="; \
		$(GO) vet ./services/$$svc/...; \
	done

help:
	@echo "Targets:"
	@echo "  make run SERVICE=<service>  Run one service (default: $(SERVICE))"
	@echo "  make test                   Run tests for all services"
	@echo "  make lint                   Run go vet for all services"
